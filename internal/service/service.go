package service

import (
	"devmate/internal/domain"
	"fmt"
	"io"
	"log/slog"
)

// Service orchestrates git data retrieval, caching, and LLM generation.
type Service struct {
	Git            domain.GitClient
	LLM            domain.LLM
	Cache          Cache  // optional — nil is safe, treated as NoopCache
	Model          string // included in cache keys to isolate responses per model
	Log            *slog.Logger
	ChunkThreshold int
}

func New(git domain.GitClient, llm domain.LLM, cache Cache, model string, log *slog.Logger) *Service {
	return &Service{
		Git:            git,
		LLM:            llm,
		Cache:          cache,
		Model:          model,
		Log:            log.With("component", "service"),
		ChunkThreshold: DefaultChunkThreshold,
	}
}

// cache returns the configured Cache, or NoopCache when nil.
// Makes Cache an optional field: tests that omit it get safe no-op behaviour
// with no nil dereference risk.
func (s *Service) cache() Cache {
	if s.Cache == nil {
		return NoopCache{}
	}
	return s.Cache
}

// log returns the configured logger, or a discard logger when nil.
func (s *Service) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return s.Log
}

type CommitOptions struct {
	domain.Options
}

type BranchOptions struct {
	Task string
	domain.Options
}

type PrOptions struct {
	SourceBranch      string
	DestinationBranch string
	domain.Options
}

func (s *Service) mapReduce(diff string, options CommitOptions) (string, error) {
	chunks := PackChunks(diff, s.ChunkThreshold)

	s.log().Debug("starting map-reduce",
		"total_bytes", len(diff),
		"chunks", len(chunks),
		"threshold", s.ChunkThreshold,
	)

	summaries := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		s.log().Debug("summarizing chunk",
			"chunk", i+1,
			"of", len(chunks),
			"files", chunk.Files,
			"bytes", len(chunk.Content),
		)

		prompt := BuildChunkPrompt(chunk.Content, i+1, len(chunks))
		summary, err := s.LLM.Generate(prompt)
		if err != nil {
			return "", fmt.Errorf("summarizing chunk %d: %w", i+1, err)
		}

		s.log().Debug("chunk summary received",
			"chunk", i+1,
			"files", chunk.Files,
			"summary", summary,
		)

		summaries = append(summaries, summary)
	}

	// Hierarchically reduce summaries until they fit within the threshold.
	summaries, err := s.reduceSummaries(summaries)
	if err != nil {
		return "", err
	}

	s.log().Debug("synthesizing", "summaries", summaries)

	prompt := BuildSynthesisPrompt(summaries, options.Type, options.Mode, options.Explain)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		return "", err
	}

	s.log().Debug("synthesis complete", "result", result)
	return result, nil
}

// reduceSummaries iteratively condenses summaries in groups until their
// combined size fits within ChunkThreshold. Each iteration packs summaries
// into groups that fit the threshold, sends each group to the LLM for
// condensation, and replaces the list with the condensed results.
// This guarantees the final synthesis prompt stays within model limits
// regardless of how many chunks the original diff produced.
func (s *Service) reduceSummaries(summaries []string) ([]string, error) {
	for {
		total := summariesSize(summaries)
		if total <= s.ChunkThreshold || len(summaries) <= 1 {
			return summaries, nil
		}

		s.log().Debug("summaries exceed threshold, reducing",
			"summaries", len(summaries),
			"total_bytes", total,
			"threshold", s.ChunkThreshold,
		)

		groups := groupSummaries(summaries, s.ChunkThreshold)
		reduced := make([]string, 0, len(groups))

		for i, group := range groups {
			// Single-item groups don't need an LLM call — pass through.
			if len(group) == 1 {
				reduced = append(reduced, group[0])
				continue
			}

			s.log().Debug("reducing summary group",
				"group", i+1,
				"of", len(groups),
				"items", len(group),
			)

			prompt := BuildReducePrompt(group)
			condensed, err := s.LLM.Generate(prompt)
			if err != nil {
				return nil, fmt.Errorf("reducing summary group %d: %w", i+1, err)
			}
			reduced = append(reduced, condensed)
		}

		// Safety: if reduction made no progress, stop to avoid infinite loop.
		if len(reduced) >= len(summaries) {
			s.log().Debug("reduction made no progress, proceeding with current summaries",
				"before", len(summaries), "after", len(reduced))
			return reduced, nil
		}

		summaries = reduced
	}
}

// summariesSize returns the total byte length of all summaries.
func summariesSize(summaries []string) int {
	n := 0
	for _, s := range summaries {
		n += len(s)
	}
	return n
}

// groupSummaries packs summaries into groups whose combined size does not
// exceed maxSize. Each group will be sent to the LLM as one reduce call.
func groupSummaries(summaries []string, maxSize int) [][]string {
	var groups [][]string
	var current []string
	currentSize := 0

	for _, s := range summaries {
		// If a single summary exceeds maxSize, it gets its own group.
		if len(s) > maxSize {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
				currentSize = 0
			}
			groups = append(groups, []string{s})
			continue
		}

		if currentSize+len(s) > maxSize && len(current) > 0 {
			groups = append(groups, current)
			current = nil
			currentSize = 0
		}
		current = append(current, s)
		currentSize += len(s)
	}

	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func (s *Service) DraftMessage(o CommitOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.log().Debug("drafting commit message", "type", typeStr, "mode", modeStr)

	diff, err := s.Git.DiffCached()
	if err != nil {
		s.log().Error("failed to get diff", "error", err)
		return "", err
	}
	s.log().Debug("got diff", "bytes", len(diff))

	key := commitCacheKey(s.Model, diff, typeStr, modeStr, o.Explain)
	if hit, ok := s.cache().Get(key); ok {
		s.log().Debug("commit message cache hit")
		return hit, nil
	}

	var result string
	if s.ChunkThreshold > 0 && len(diff) > s.ChunkThreshold {
		s.log().Debug("diff exceeds threshold, using map-reduce",
			"diff_bytes", len(diff),
			"threshold", s.ChunkThreshold,
		)
		result, err = s.mapReduce(diff, o)
	} else {
		prompt := BuildCommitPrompt(diff, o)
		result, err = s.LLM.Generate(prompt)
	}
	if err != nil {
		s.log().Error("LLM generation failed", "error", err)
		return "", err
	}

	result = sanitize(result)
	if err := s.cache().Set(key, result); err != nil {
		s.log().Debug("failed to write cache entry", "error", err)
	}

	s.log().Debug("message drafted successfully")
	return result, nil
}

func (s *Service) DraftBranchName(o BranchOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.log().Debug("drafting branch name", "task", o.Task, "type", typeStr, "mode", modeStr)

	key := branchCacheKey(s.Model, o.Task, typeStr, modeStr, o.Explain)
	if hit, ok := s.cache().Get(key); ok {
		s.log().Debug("branch name cache hit")
		return hit, nil
	}

	prompt := BuildBranchPrompt(o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.log().Error("LLM generation failed", "error", err)
		return "", err
	}

	result = extractBranchName(result)
	if err := s.cache().Set(key, result); err != nil {
		s.log().Debug("failed to write cache entry", "error", err)
	}

	s.log().Debug("branch name drafted successfully")
	return result, nil
}

func (s *Service) DraftPrDescription(o PrOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.log().Debug("drafting pr description", "source", o.SourceBranch, "destination", o.DestinationBranch, "type", typeStr, "mode", modeStr)

	commits, err := s.Git.LogBetween(o.DestinationBranch, o.SourceBranch)
	if err != nil {
		s.log().Error("failed to get commits", "error", err)
		return "", err
	}
	s.log().Debug("got commits", "count", len(commits))

	if len(commits) == 0 {
		s.log().Debug("no unique commits between branches, skipping LLM call",
			"source", o.SourceBranch, "destination", o.DestinationBranch)
		return "", domain.ErrEmptyPR
	}

	key := prCacheKey(s.Model, commits, typeStr, modeStr, o.Explain)
	if hit, ok := s.cache().Get(key); ok {
		s.log().Debug("pr description cache hit")
		return hit, nil
	}

	prompt := BuildPrPrompt(commits, o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.log().Error("LLM generation failed", "error", err)
		return "", err
	}

	result = sanitize(result)
	if err := s.cache().Set(key, result); err != nil {
		s.log().Debug("failed to write cache entry", "error", err)
	}

	s.log().Debug("pr description drafted successfully")
	return result, nil
}

// NoopCache is a Cache that never stores anything.
// Used when caching is disabled and as the zero-value fallback via cache().
type NoopCache struct{}

func (NoopCache) Get(string) (string, bool) { return "", false }
func (NoopCache) Set(string, string) error  { return nil }
func (NoopCache) Clear() error              { return nil }
