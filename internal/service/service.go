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

	s.log().Debug("synthesizing", "summaries", summaries)

	prompt := BuildSynthesisPrompt(summaries, options.Type, options.Mode, options.Explain)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		return "", err
	}

	s.log().Debug("synthesis complete", "result", result)
	return result, nil
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
