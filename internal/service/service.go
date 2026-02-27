package service

import (
	"context"
	"devmate/internal/domain"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// Service orchestrates git data retrieval, caching, and LLM generation.
type Service struct {
	Git            domain.GitClient
	LLM            domain.LLM
	Cache          Cache           // optional — nil is safe, treated as NoopCache
	Progress       domain.Progress // optional — nil is safe, treated as NoopProgress
	Model          string          // included in cache keys to isolate responses per model
	Log            *slog.Logger
	ChunkThreshold int
	MaxConcurrency int    // max parallel LLM calls; 0 or negative means runtime.NumCPU()
	BinaryHash     string // included in cache keys so a new binary never serves stale entries
	MaxRetries     int    // service-level retry attempts for transient LLM errors; 0 means no retry
}

func New(git domain.GitClient, llm domain.LLM, cache Cache, model string, log *slog.Logger) *Service {
	return &Service{
		Git:            git,
		LLM:            llm,
		Cache:          cache,
		Model:          model,
		Log:            log.With("component", "service"),
		ChunkThreshold: DefaultChunkThreshold,
		MaxConcurrency: DefaultServiceMaxConcurrency,
		BinaryHash:     BinaryHash(),
	}
}

// concurrency returns the effective concurrency limit.
func (s *Service) concurrency() int {
	if s.MaxConcurrency > 0 {
		return s.MaxConcurrency
	}
	return runtime.NumCPU()
}

// progress returns the configured Progress, or NoopProgress when nil.
func (s *Service) progress() domain.Progress {
	if s.Progress == nil {
		return domain.NoopProgress{}
	}
	return s.Progress
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

// generateWithRetry calls s.LLM.Generate and retries up to s.MaxRetries times
// on failure. Retry 0 means a single attempt with no retries.
// All calls share the same prompt — this is a pure service-level safety net
// on top of any HTTP-level retry already present in the LLM client.
func (s *Service) generateWithRetry(prompt string) (string, error) {
	var lastErr error
	attempts := s.MaxRetries + 1
	for i := 0; i < attempts; i++ {
		if i > 0 {
			s.log().Debug("retrying LLM generate",
				"attempt", i+1,
				"of", attempts,
				"last_error", lastErr,
			)
		}
		result, err := s.LLM.Generate(prompt)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("LLM generate failed after %d attempt(s): %w", attempts, lastErr)
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

func (s *Service) mapReducePr(commits []string, options PrOptions) (string, error) {
	joined := strings.Join(commits, "\n")
	chunks := PackChunks(joined, s.ChunkThreshold)

	s.log().Debug("starting PR map-reduce",
		"total_bytes", len(joined),
		"chunks", len(chunks),
		"threshold", s.ChunkThreshold,
	)

	// Map phase: summarize each chunk of commits in parallel.
	summaries, err := s.summarizeChunksParallel(chunks)
	if err != nil {
		return "", err
	}

	// Hierarchically reduce summaries until they fit within the threshold.
	summaries, err = s.reduceSummaries(summaries)
	if err != nil {
		return "", err
	}

	s.log().Debug("synthesizing PR description", "summaries", summaries)
	s.progress().Status("Synthesizing PR description...")

	prompt := BuildPrSynthesisPrompt(summaries, options)
	result, err := s.generateWithRetry(prompt)
	if err != nil {
		return "", err
	}

	s.log().Debug("PR synthesis complete", "result", result)
	return sanitize(result), nil
}

func (s *Service) mapReduce(diff string, options CommitOptions) (string, error) {
	chunks := PackChunks(diff, s.ChunkThreshold)

	s.log().Debug("starting map-reduce",
		"total_bytes", len(diff),
		"chunks", len(chunks),
		"threshold", s.ChunkThreshold,
	)

	// Map phase: summarize all chunks in parallel.
	summaries, err := s.summarizeChunksParallel(chunks)
	if err != nil {
		return "", err
	}

	// Hierarchically reduce summaries until they fit within the threshold.
	summaries, err = s.reduceSummaries(summaries)
	if err != nil {
		return "", err
	}

	s.log().Debug("synthesizing", "summaries", summaries)
	s.progress().Status("Synthesizing commit message...")

	prompt := BuildSynthesisPrompt(summaries, options.Type, options.Mode, options.Explain)
	result, err := s.generateWithRetry(prompt)
	if err != nil {
		return "", err
	}

	s.log().Debug("synthesis complete", "result", result)
	return result, nil
}

// summarizeChunksParallel sends all chunk prompts to the LLM concurrently
// (up to s.concurrency() at a time) and returns the summaries in their
// original order. It returns the first error encountered (if any).
func (s *Service) summarizeChunksParallel(chunks []Chunk) ([]string, error) {
	n := len(chunks)
	summaries := make([]string, n)
	sem := make(chan struct{}, s.concurrency())
	var completed atomic.Int64

	// ctx is cancelled as soon as any chunk fails so remaining goroutines
	// waiting on the semaphore abort instead of sending more requests to Ollama.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	s.progress().Status(fmt.Sprintf("Summarizing chunk 0/%d...", n))

	wg.Add(n)
	for i, chunk := range chunks {
		go func(idx int, ch Chunk) {
			defer wg.Done()

			// Abort before acquiring a slot if a sibling already failed.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			s.log().Debug("summarizing chunk",
				"chunk", idx+1,
				"of", n,
				"files", ch.Files,
				"bytes", len(ch.Content),
			)

			prompt := BuildChunkPrompt(ch.Content, idx+1, n)
			summary, err := s.generateWithRetry(prompt)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("summarizing chunk %d: %w", idx+1, err)
					cancel() // signal all waiting goroutines to stop
				}
				mu.Unlock()
				return
			}

			done := completed.Add(1)
			s.progress().Status(fmt.Sprintf("Summarizing chunk %d/%d...", done, n))

			s.log().Debug("chunk summary received",
				"chunk", idx+1,
				"files", ch.Files,
				"summary", summary,
			)

			summaries[idx] = summary
		}(i, chunk)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return summaries, nil
}

// reduceSummaries iteratively condenses summaries in groups until their
// combined size fits within ChunkThreshold. Each iteration packs summaries
// into groups that fit the threshold, sends each group to the LLM in parallel
// for condensation, and replaces the list with the condensed results.
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
		reduced := make([]string, len(groups))
		sem := make(chan struct{}, s.concurrency())
		var reduceCompleted atomic.Int64

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		s.progress().Status(fmt.Sprintf("Reducing summaries: group 0/%d...", len(groups)))

		var (
			wg       sync.WaitGroup
			mu       sync.Mutex
			firstErr error
		)

		for i, group := range groups {
			// Single-item groups don't need an LLM call — pass through.
			if len(group) == 1 {
				reduced[i] = group[0]
				continue
			}

			wg.Add(1)
			go func(idx int, grp []string) {
				defer wg.Done()

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()

				s.log().Debug("reducing summary group",
					"group", idx+1,
					"of", len(groups),
					"items", len(grp),
				)

				prompt := BuildReducePrompt(grp)
				condensed, err := s.generateWithRetry(prompt)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("reducing summary group %d: %w", idx+1, err)
						cancel()
					}
					mu.Unlock()
					return
				}
				done := reduceCompleted.Add(1)
				s.progress().Status(fmt.Sprintf("Reducing summaries: group %d/%d...", done, len(groups)))
				reduced[idx] = condensed
			}(i, group)
		}

		wg.Wait()

		if firstErr != nil {
			return nil, firstErr
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

	key := commitCacheKey(s.Model, s.BinaryHash, diff, typeStr, modeStr, o.Explain)
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
		s.progress().Status("Generating commit message...")
		prompt := BuildCommitPrompt(diff, o)
		result, err = s.generateWithRetry(prompt)
	}
	if err != nil {
		s.progress().Done("")
		s.log().Error("LLM generation failed", "error", err)
		return "", err
	}

	result = sanitize(result)
	if err := s.cache().Set(key, result); err != nil {
		s.log().Debug("failed to write cache entry", "error", err)
	}

	s.progress().Done("")
	s.log().Debug("message drafted successfully")
	return result, nil
}

func (s *Service) DraftBranchName(o BranchOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.log().Debug("drafting branch name", "task", o.Task, "type", typeStr, "mode", modeStr)

	key := branchCacheKey(s.Model, s.BinaryHash, o.Task, typeStr, modeStr, o.Explain)
	if hit, ok := s.cache().Get(key); ok {
		s.log().Debug("branch name cache hit")
		return hit, nil
	}

	s.progress().Status("Generating branch name...")
	prompt := BuildBranchPrompt(o)
	result, err := s.generateWithRetry(prompt)
	if err != nil {
		s.progress().Done("")
		s.log().Error("LLM generation failed", "error", err)
		return "", err
	}

	result = extractBranchName(result)
	if err := s.cache().Set(key, result); err != nil {
		s.log().Debug("failed to write cache entry", "error", err)
	}

	s.progress().Done("")
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

	key := prCacheKey(s.Model, s.BinaryHash, commits, typeStr, modeStr, o.Explain)
	if hit, ok := s.cache().Get(key); ok {
		s.log().Debug("pr description cache hit")
		return hit, nil
	}

	var result string
	joinedCommits := strings.Join(commits, "\n")
	if s.ChunkThreshold > 0 && len(joinedCommits) > s.ChunkThreshold {
		s.log().Debug("commits exceeds threshold, using map-reduce",
			"commits_bytes", len(joinedCommits),
			"threshold", s.ChunkThreshold,
		)
		result, err = s.mapReducePr(commits, o)
	} else {
		s.progress().Status("Generating PR description...")
		prompt := BuildPrPrompt(commits, o)
		result, err = s.generateWithRetry(prompt)
		if err == nil {
			result = sanitize(result)
		}
	}

	if err != nil {
		s.progress().Done("")
		s.log().Error("LLM generation failed", "error", err)
		return "", err
	}

	if err := s.cache().Set(key, result); err != nil {
		s.log().Debug("failed to write cache entry", "error", err)
	}

	s.progress().Done("")
	s.log().Debug("pr description drafted successfully")
	return result, nil
}

// NoopCache is a Cache that never stores anything.
// Used when caching is disabled and as the zero-value fallback via cache().
type NoopCache struct{}

func (NoopCache) Get(string) (string, bool) { return "", false }
func (NoopCache) Set(string, string) error  { return nil }
func (NoopCache) Clear() error              { return nil }
