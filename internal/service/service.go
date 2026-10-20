package service

import (
	"context"
	"devmate/internal/config"
	"devmate/internal/domain"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/sync/errgroup"
)

// parallelDo runs fn on each task with at most maxWorkers goroutines and
// returns results in input order. The first error cancels the remaining tasks.
func parallelDo[In, Out any](
	ctx context.Context,
	maxWorkers int,
	tasks []In,
	fn func(ctx context.Context, idx int, in In) (Out, error),
) ([]Out, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(max(maxWorkers, 1))

	out := make([]Out, len(tasks))
	for i, task := range tasks {
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			res, err := fn(ctx, i, task)
			if err != nil {
				return err
			}
			out[i] = res
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

type Service struct {
	git            domain.GitClient
	llm            domain.LLM
	cache          Cache           // optional, nil means NoopCache
	progress       domain.Progress // optional, nil means NoopProgress
	model          string
	log            *slog.Logger
	chunkThreshold int
	maxConcurrency int // 0 means runtime.NumCPU()
	binaryHash     string
	maxRetries     int           // 0 means a single attempt
	retryBaseDelay time.Duration // 0 means config.DefaultRetryBaseDelay
}

// Settings configures a Service at construction time.
type Settings func(*Service)

// WithProgress attaches a progress reporter to the service.
func WithProgress(p domain.Progress) Settings {
	return func(s *Service) { s.progress = p }
}

// WithChunkThreshold overrides the diff size above which map-reduce chunking is used.
func WithChunkThreshold(n int) Settings {
	return func(s *Service) {
		if n > 0 {
			s.chunkThreshold = n
		}
	}
}

// WithMaxConcurrency overrides the maximum number of parallel LLM calls.
func WithMaxConcurrency(n int) Settings {
	return func(s *Service) {
		if n > 0 {
			s.maxConcurrency = n
		}
	}
}

// WithMaxRetries sets the number of retries after a failed LLM call. Negative values mean zero.
func WithMaxRetries(n int) Settings {
	return func(s *Service) { s.maxRetries = max(n, 0) }
}

// WithRetryBaseDelay sets the initial exponential back-off delay between retries.
func WithRetryBaseDelay(d time.Duration) Settings {
	return func(s *Service) {
		if d > 0 {
			s.retryBaseDelay = d
		}
	}
}

// New constructs a Service with defaults, overridden by opts.
func New(git domain.GitClient, llm domain.LLM, cache Cache, model string, log *slog.Logger, opts ...Settings) *Service {
	svc := &Service{
		git:            git,
		llm:            llm,
		cache:          cache,
		model:          model,
		log:            log.With("component", "service"),
		chunkThreshold: config.DefaultServiceChunkThreshold,
		maxConcurrency: config.DefaultServiceMaxConcurrency,
		binaryHash:     BinaryHash(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func (s *Service) concurrency() int {
	if s.maxConcurrency > 0 {
		return s.maxConcurrency
	}
	return runtime.NumCPU()
}

// Progress returns the configured progress, or NoopProgress when nil.
func (s *Service) Progress() domain.Progress {
	if s.progress == nil {
		return domain.NoopProgress{}
	}
	return s.progress
}

func (s *Service) ChunkThreshold() int {
	return s.chunkThreshold
}

func (s *Service) MaxRetries() int {
	return s.maxRetries
}

// Cache returns the configured cache, or NoopCache when nil.
func (s *Service) Cache() Cache {
	if s.cache == nil {
		return NoopCache{}
	}
	return s.cache
}

// Log returns the configured logger, or a discard logger when nil.
func (s *Service) Log() *slog.Logger {
	if s.log == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return s.log
}

// RetryBaseDelay returns the configured base delay, or config.DefaultRetryBaseDelay when unset.
func (s *Service) RetryBaseDelay() time.Duration {
	if s.retryBaseDelay > 0 {
		return s.retryBaseDelay
	}
	return config.DefaultRetryBaseDelay
}

func (s *Service) Git() domain.GitClient {
	return s.git
}

// GenerateWithRetry calls the LLM with exponential back-off, making at most
// MaxRetries+1 attempts. On exhaustion the returned error wraps both
// domain.ErrLLMNoAttemptsSucceed and the last underlying error; non-retryable
// errors are returned immediately as-is.
func (s *Service) GenerateWithRetry(ctx context.Context, prompt string) (string, error) {
	if s.llm == nil {
		return "", domain.ErrLLMNoConfigured
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	attempts := s.maxRetries + 1
	policy := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(s.RetryBaseDelay()),
		backoff.WithMultiplier(2),
		backoff.WithRandomizationFactor(0),
		backoff.WithMaxElapsedTime(0),
	)
	attempt := 0

	result, err := backoff.RetryNotifyWithData(
		func() (string, error) {
			attempt++
			res, err := s.llm.Generate(ctx, prompt)
			if err == nil {
				return res, nil
			}
			if ctx.Err() != nil || !retryable(err) {
				return "", backoff.Permanent(err)
			}
			s.Log().Debug("llm generate failed", "attempt", attempt, "of", attempts, "error", err)
			return "", err
		},
		backoff.WithContext(backoff.WithMaxRetries(policy, uint64(s.maxRetries)), ctx),
		func(_ error, next time.Duration) {
			s.Log().Debug("retrying llm generate", "attempt", attempt+1, "of", attempts, "after", next)
		},
	)
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil || !retryable(err) {
		return "", err
	}
	return "", fmt.Errorf("%w: %w", domain.ErrLLMNoAttemptsSucceed, err)
}

// retryable reports whether err is worth another attempt: network failures and
// transient server statuses are; malformed requests and client-side statuses are not.
func retryable(err error) bool {
	var status *domain.LLMStatusError
	if errors.As(err, &status) {
		return status.Retryable()
	}
	return !errors.Is(err, domain.ErrLLMMarshalRequestFailed) &&
		!errors.Is(err, domain.ErrLLMBuildRequestFailed)
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

func (s *Service) mapReducePr(ctx context.Context, commits []string, options PrOptions) (string, error) {
	joined := strings.Join(commits, "\n")
	chunks := PackChunks(joined, s.chunkThreshold)

	s.Log().Debug("starting PR map-reduce",
		"total_bytes", len(joined),
		"chunks", len(chunks),
		"threshold", s.ChunkThreshold(),
	)

	summaries, err := s.summarizeChunksParallel(ctx, chunks)
	if err != nil {
		return "", err
	}

	summaries, err = s.reduceSummaries(ctx, summaries)
	if err != nil {
		return "", err
	}

	s.Log().Debug("synthesizing PR description", "summaries", summaries)
	s.Progress().Status("Synthesizing PR description...")

	prompt := BuildPrSynthesisPrompt(summaries, options)
	result, err := s.GenerateWithRetry(ctx, prompt)
	if err != nil {
		return "", err
	}

	s.Log().Debug("PR synthesis complete", "result", result)
	return sanitize(result), nil
}

func (s *Service) mapReduce(ctx context.Context, diff string, options CommitOptions) (string, error) {
	chunks := PackChunks(diff, s.ChunkThreshold())

	s.Log().Debug("starting map-reduce",
		"total_bytes", len(diff),
		"chunks", len(chunks),
		"threshold", s.ChunkThreshold(),
	)

	summaries, err := s.summarizeChunksParallel(ctx, chunks)
	if err != nil {
		return "", err
	}

	summaries, err = s.reduceSummaries(ctx, summaries)
	if err != nil {
		return "", err
	}

	s.Log().Debug("synthesizing", "summaries", summaries)
	s.Progress().Status("Synthesizing commit message...")

	prompt := BuildSynthesisPrompt(summaries, options.Type, options.Mode, options.Explain)
	result, err := s.GenerateWithRetry(ctx, prompt)
	if err != nil {
		return "", err
	}

	s.Log().Debug("synthesis complete", "result", result)
	return result, nil
}

func (s *Service) summarizeChunksParallel(ctx context.Context, chunks []Chunk) ([]string, error) {
	n := len(chunks)
	var completed atomic.Int64
	s.Progress().Status(fmt.Sprintf("Summarizing chunk 0/%d...", n))

	return parallelDo(ctx, s.concurrency(), chunks,
		func(ctx context.Context, idx int, ch Chunk) (string, error) {
			s.Log().Debug("summarizing chunk",
				"chunk", idx+1, "of", n,
				"files", ch.Files, "bytes", len(ch.Content),
			)
			prompt := BuildChunkPrompt(ch.Content, idx+1, n)
			summary, err := s.GenerateWithRetry(ctx, prompt)
			if err != nil {
				return "", fmt.Errorf("%w: chunk %d: %w", domain.ErrChunkFailed, idx+1, err)
			}
			done := completed.Add(1)
			s.Progress().Status(fmt.Sprintf("Summarizing chunk %d/%d...", done, n))
			s.Log().Debug("chunk summary received",
				"chunk", idx+1, "files", ch.Files, "summary", summary,
			)
			return summary, nil
		},
	)
}

// reduceSummaries condenses summaries in parallel groups until they fit within ChunkThreshold.
func (s *Service) reduceSummaries(ctx context.Context, summaries []string) ([]string, error) {
	for {
		total := summariesSize(summaries)
		if total <= s.ChunkThreshold() || len(summaries) <= 1 {
			return summaries, nil
		}

		s.Log().Debug("summaries exceed threshold, reducing",
			"summaries", len(summaries),
			"total_bytes", total,
			"threshold", s.ChunkThreshold(),
		)

		groups := groupSummaries(summaries, s.ChunkThreshold())
		var reduceCompleted atomic.Int64
		s.Progress().Status(fmt.Sprintf("Reducing summaries: group 0/%d...", len(groups)))

		reduced, err := parallelDo(ctx, s.concurrency(), groups,
			func(ctx context.Context, idx int, grp []string) (string, error) {
				if len(grp) == 1 {
					return grp[0], nil
				}
				s.Log().Debug("reducing summary group",
					"group", idx+1, "of", len(groups), "items", len(grp),
				)
				prompt := BuildReducePrompt(grp)
				condensed, err := s.GenerateWithRetry(ctx, prompt)
				if err != nil {
					return "", fmt.Errorf("%w: group %d: %w", domain.ErrReduceFailed, idx+1, err)
				}
				done := reduceCompleted.Add(1)
				s.Progress().Status(fmt.Sprintf("Reducing summaries: group %d/%d...", done, len(groups)))
				return condensed, nil
			},
		)
		if err != nil {
			return nil, err
		}

		if len(reduced) >= len(summaries) {
			s.Log().Debug("reduction made no progress, proceeding with current summaries",
				"before", len(summaries), "after", len(reduced))
			return reduced, nil
		}

		summaries = reduced
	}
}

func summariesSize(summaries []string) int {
	n := 0
	for _, s := range summaries {
		n += len(s)
	}
	return n
}

// groupSummaries packs summaries into groups of at most maxSize bytes.
func groupSummaries(summaries []string, maxSize int) [][]string {
	var groups [][]string
	var current []string
	currentSize := 0

	for _, s := range summaries {
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

func (s *Service) DraftMessage(ctx context.Context, o CommitOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log().Debug("drafting commit message", "type", typeStr, "mode", modeStr)

	diff, err := s.git.DiffCached()
	if err != nil {
		s.Log().Debug("failed to get diff", "error", err)
		return "", err
	}
	s.Log().Debug("got diff", "bytes", len(diff))

	key := commitCacheKey(s.model, s.binaryHash, diff, typeStr, modeStr, o.Explain)
	if !o.NoCache {
		if hit, ok := s.Cache().Get(key); ok {
			s.Log().Debug("commit message cache hit")
			return hit, nil
		}
	}

	var result string
	if s.ChunkThreshold() > 0 && len(diff) > s.ChunkThreshold() {
		s.Log().Debug("diff exceeds threshold, using map-reduce",
			"diff_bytes", len(diff),
			"threshold", s.ChunkThreshold(),
		)
		result, err = s.mapReduce(ctx, diff, o)
	} else {
		s.Progress().Status("Generating commit message...")
		prompt := BuildCommitPrompt(diff, o)
		result, err = s.GenerateWithRetry(ctx, prompt)
	}
	if err != nil {
		s.Progress().Done("")
		s.Log().Debug("llm generation failed", "error", err)
		return "", err
	}

	result = sanitize(result)
	if err := s.Cache().Set(key, result); err != nil {
		s.Log().Debug("failed to write cache entry", "error", err)
	}

	s.Progress().Done("")
	s.Log().Debug("message drafted successfully")
	return result, nil
}

func (s *Service) DraftBranchName(ctx context.Context, o BranchOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log().Debug("drafting branch name", "task", o.Task, "type", typeStr, "mode", modeStr)

	key := branchCacheKey(s.model, s.binaryHash, o.Task, typeStr, modeStr, o.Explain)
	if !o.NoCache {
		if hit, ok := s.Cache().Get(key); ok {
			s.Log().Debug("branch name cache hit")
			return hit, nil
		}
	}

	s.Progress().Status("Generating branch name...")
	prompt := BuildBranchPrompt(o)
	result, err := s.GenerateWithRetry(ctx, prompt)
	if err != nil {
		s.Progress().Done("")
		s.Log().Debug("llm generation failed", "error", err)
		return "", err
	}

	result = extractBranchName(result)
	if err := s.Cache().Set(key, result); err != nil {
		s.Log().Debug("failed to write cache entry", "error", err)
	}

	s.Progress().Done("")
	s.Log().Debug("branch name drafted successfully")
	return result, nil
}

func (s *Service) DraftPrDescription(ctx context.Context, o PrOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log().Debug("drafting pr description", "source", o.SourceBranch, "destination", o.DestinationBranch, "type", typeStr, "mode", modeStr)

	commits, err := s.git.LogBetween(o.DestinationBranch, o.SourceBranch)
	if err != nil {
		s.Log().Debug("failed to get commits", "error", err)
		return "", err
	}
	s.Log().Debug("got commits", "count", len(commits))

	if len(commits) == 0 {
		s.Log().Debug("no unique commits between branches, skipping llm call",
			"source", o.SourceBranch, "destination", o.DestinationBranch)
		return "", domain.ErrEmptyPR
	}

	key := prCacheKey(s.model, s.binaryHash, commits, typeStr, modeStr, o.Explain)
	if !o.NoCache {
		if hit, ok := s.Cache().Get(key); ok {
			s.Log().Debug("pr description cache hit")
			return hit, nil
		}
	}

	var result string
	joinedCommits := strings.Join(commits, "\n")
	if s.ChunkThreshold() > 0 && len(joinedCommits) > s.ChunkThreshold() {
		s.Log().Debug("commits exceeds threshold, using map-reduce",
			"commits_bytes", len(joinedCommits),
			"threshold", s.ChunkThreshold(),
		)
		result, err = s.mapReducePr(ctx, commits, o)
	} else {
		s.Progress().Status("Generating PR description...")
		prompt := BuildPrPrompt(commits, o)
		result, err = s.GenerateWithRetry(ctx, prompt)
		if err == nil {
			result = sanitize(result)
		}
	}

	if err != nil {
		s.Progress().Done("")
		s.Log().Debug("llm generation failed", "error", err)
		return "", err
	}

	if err := s.Cache().Set(key, result); err != nil {
		s.Log().Debug("failed to write cache entry", "error", err)
	}

	s.Progress().Done("")
	s.Log().Debug("pr description drafted successfully")
	return result, nil
}

// NoopCache is a Cache that never stores anything.
type NoopCache struct{}

func (NoopCache) Get(string) (string, bool) { return "", false }
func (NoopCache) Set(string, string) error  { return nil }
func (NoopCache) Clear() error              { return nil }
