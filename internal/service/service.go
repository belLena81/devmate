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
	"time"
)

// parallelResult carries the ordered output of one parallel task.
type parallelResult[T any] struct {
	idx int
	val T
	err error
}

// parallelDo runs fn on each element of tasks concurrently with at most
// maxWorkers goroutines alive at once (worker-pool pattern). Results are
// returned in the original order. On the first error, remaining queued tasks
// are abandoned via context cancellation.
//
// This replaces the previous fan-out pattern that spawned len(tasks) goroutines
// up-front and used a semaphore to gate execution. With n=100 and concurrency=2,
// the old pattern kept 98 blocked goroutines in memory; this keeps exactly 2.
func parallelDo[In, Out any](
	ctx context.Context,
	maxWorkers int,
	tasks []In,
	fn func(ctx context.Context, idx int, in In) (Out, error),
) ([]Out, error) {
	n := len(tasks)
	if n == 0 {
		return nil, nil
	}

	workers := min(n, maxWorkers)

	// Pre-fill a buffered work channel so workers never block on send.
	work := make(chan int, n)
	for i := range tasks {
		work <- i
	}
	close(work)

	results := make(chan parallelResult[Out], n)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for idx := range work {
				if cancelCtx.Err() != nil {
					return
				}
				out, err := fn(cancelCtx, idx, tasks[idx])
				results <- parallelResult[Out]{idx: idx, val: out, err: err}
				if err != nil {
					cancel() // signal remaining workers to stop
					return
				}
			}
		}()
	}

	// Close results as soon as all workers finish so the collector loop below exits.
	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]Out, n)
	for r := range results {
		if r.err != nil {
			// Drain remaining in-flight results so blocked workers can send
			// and exit; we discard them since we're returning an error.
			go func() {
				for range results {
				}
			}()
			return nil, r.err
		}
		out[r.idx] = r.val
	}
	return out, nil
}

type Service struct {
	git            domain.GitClient
	llm            domain.LLM
	cache          Cache           // optional — nil is safe, treated as NoopCache
	progress       domain.Progress // optional — nil is safe, treated as NoopProgress
	model          string          // included in cache keys to isolate responses per model
	log            *slog.Logger
	chunkThreshold int
	maxConcurrency int           // max parallel LLM calls; 0 or negative means runtime.NumCPU()
	binaryHash     string        // included in cache keys so a new binary never serves stale entries
	maxRetries     int           // retry attempts for transient LLM errors; 0 means a single attempt
	retryBaseDelay time.Duration // initial back-off between retries; 0 uses defaultRetryBaseDelay
}

func NewService(git domain.GitClient, llm domain.LLM, cache Cache, logger *slog.Logger, threshold int) *Service {
	return &Service{
		git:            git,
		llm:            llm,
		cache:          cache,
		model:          "",
		log:            logger,
		chunkThreshold: threshold,
		maxConcurrency: DefaultServiceMaxConcurrency,
		binaryHash:     BinaryHash(),
	}
}

const defaultRetryBaseDelay = 2 * time.Second

// Settings is a functional settings for configuring a Service at
// construction time. Using settings instead of post-construction field mutation
// keeps the Service immutable after New() returns and makes the call site in
// main.go self-documenting.
type Settings func(*Service)

// WithProgress attaches a progress reporter to the service.
// When not provided, a no-op reporter is used.
func WithProgress(p domain.Progress) Settings {
	return func(s *Service) { s.progress = p }
}

// WithChunkThreshold overrides the diff-size threshold above which map-reduce
// chunking is used. Must be a positive value; zero is ignored (keeps default).
func WithChunkThreshold(n int) Settings {
	return func(s *Service) {
		if n > 0 {
			s.chunkThreshold = n
		}
	}
}

// WithMaxConcurrency overrides the maximum number of parallel llm calls.
// Zero or negative is ignored (keeps default; runtime.NumCPU() is used at
// call time).
func WithMaxConcurrency(n int) Settings {
	return func(s *Service) {
		if n > 0 {
			s.maxConcurrency = n
		}
	}
}

// WithMaxRetries sets the number of retry attempts for transient llm errors.
// Zero means a single attempt (no retries), which is the default.
func WithMaxRetries(n int) Settings {
	return func(s *Service) { s.maxRetries = n }
}

// WithRetryBaseDelay sets the initial exponential back-off delay between
// retries. Zero is ignored (keeps the package default of 2 s).
func WithRetryBaseDelay(d time.Duration) Settings {
	return func(s *Service) {
		if d > 0 {
			s.retryBaseDelay = d
		}
	}
}

// New constructs a Service with sensible defaults. Pass Settings values
// to override any field — this is the only supported way to configure the
// service after the constructor returns.
func New(git domain.GitClient, llm domain.LLM, cache Cache, model string, log *slog.Logger, opts ...Settings) *Service {
	svc := &Service{
		git:            git,
		llm:            llm,
		cache:          cache,
		model:          model,
		log:            log.With("component", "service"),
		chunkThreshold: DefaultChunkThreshold,
		maxConcurrency: DefaultServiceMaxConcurrency,
		binaryHash:     BinaryHash(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// concurrency returns the effective concurrency limit.
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
// Makes cache an optional field: tests that omit it get safe no-op behaviour
// with no nil dereference risk.
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

// RetryBaseDelay returns the configured base delay, falling back to the
// package default when the field is zero (i.e. not explicitly set).
func (s *Service) RetryBaseDelay() time.Duration {
	if s.retryBaseDelay > 0 {
		return s.retryBaseDelay
	}
	return defaultRetryBaseDelay
}

func (s *Service) Git() domain.GitClient {
	return s.git
}

// GenerateWithRetry calls s.llm.Generate and retries up to s.MaxRetries times
// on failure with exponential back-off. MaxRetries=0 means a single attempt.
//
// This is the single retry site in the application. The LLM client (OllamaClient)
// makes exactly one HTTP attempt per call; all retry logic lives here so the
// total number of attempts is always MaxRetries+1, never multiplied.
//
// ctx is forwarded to every LLM call. A cancelled or expired context
// short-circuits immediately without further retries.
func (s *Service) GenerateWithRetry(ctx context.Context, prompt string) (string, error) {
	if s.llm == nil {
		return "", fmt.Errorf("service: llm is not configured (nil)")
	}
	attempts := s.maxRetries + 1
	delay := s.RetryBaseDelay()

	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if i > 0 {
			s.Log().Debug("retrying llm generate",
				"attempt", i+1,
				"of", attempts,
				"after", delay,
			)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			delay *= 2
		}

		result, err := s.llm.Generate(ctx, prompt)
		if err == nil {
			return result, nil
		}

		// Don't retry on context cancellation — the caller gave up.
		if ctx.Err() != nil {
			return "", err
		}

		s.Log().Debug("llm generate failed", "attempt", i+1, "of", attempts, "error", err)
		if i == attempts-1 {
			return "", fmt.Errorf("llm generate failed after %d attempt(s): %w", attempts, err)
		}
	}

	// Unreachable, but satisfies the compiler.
	return "", domain.ErrLLMNoAttempts
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

	// Map phase: summarize each chunk of commits in parallel.
	summaries, err := s.summarizeChunksParallel(ctx, chunks)
	if err != nil {
		return "", err
	}

	// Hierarchically reduce summaries until they fit within the threshold.
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
		"threshold", s.ChunkThreshold,
	)

	// Map phase: summarize all chunks in parallel.
	summaries, err := s.summarizeChunksParallel(ctx, chunks)
	if err != nil {
		return "", err
	}

	// Hierarchically reduce summaries until they fit within the threshold.
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

// summarizeChunksParallel sends all chunk prompts to the LLM concurrently
// (up to s.concurrency() at a time) and returns the summaries in their
// original order. It returns the first error encountered (if any).
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

// reduceSummaries iteratively condenses summaries in groups until their
// combined size fits within ChunkThreshold. Each iteration packs summaries
// into groups that fit the threshold, sends each group to the LLM in parallel
// for condensation, and replaces the list with the condensed results.
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

// summariesSize returns the total byte length of all summaries.
func summariesSize(summaries []string) int {
	n := 0
	for _, s := range summaries {
		n += len(s)
	}
	return n
}

// groupSummaries packs summaries into groups whose combined size does not
// exceed maxSize. Each group will be sent to the llm as one reduce call.
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

func (s *Service) DraftMessage(ctx context.Context, o CommitOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log().Debug("drafting commit message", "type", typeStr, "mode", modeStr)

	diff, err := s.git.DiffCached()
	if err != nil {
		s.Log().Error("failed to get diff", "error", err)
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
		s.Log().Error("llm generation failed", "error", err)
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
		s.Log().Error("llm generation failed", "error", err)
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
		s.Log().Error("failed to get commits", "error", err)
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
		s.Log().Error("llm generation failed", "error", err)
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
// Used when caching is disabled and as the zero-value fallback via cache().
type NoopCache struct{}

func (NoopCache) Get(string) (string, bool) { return "", false }
func (NoopCache) Set(string, string) error  { return nil }
func (NoopCache) Clear() error              { return nil }
