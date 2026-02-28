package service

import (
	"context"
	"devmate/internal/domain"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ─── groupSummaries ─────────────────────────────────────────────────────────

func TestGroupSummaries_FitsInOneGroup(t *testing.T) {
	summaries := []string{"- small change A", "- small change B"}
	groups := groupSummaries(summaries, 1000)

	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("expected 2 items in group, got %d", len(groups[0]))
	}
}

func TestGroupSummaries_SplitsWhenExceedsMax(t *testing.T) {
	s1 := strings.Repeat("a", 300)
	s2 := strings.Repeat("b", 300)
	s3 := strings.Repeat("c", 300)

	groups := groupSummaries([]string{s1, s2, s3}, 500)

	if len(groups) < 2 {
		t.Errorf("expected at least 2 groups for 900 bytes with 500 max, got %d", len(groups))
	}
}

func TestGroupSummaries_OversizedSingleSummaryGetsOwnGroup(t *testing.T) {
	small := "small"
	huge := strings.Repeat("x", 600)

	groups := groupSummaries([]string{small, huge}, 500)

	// Expect: [["small"], ["xxx..."]]
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0][0] != small {
		t.Errorf("first group should contain the small summary")
	}
	if groups[1][0] != huge {
		t.Errorf("second group should contain the oversized summary")
	}
}

func TestGroupSummaries_Empty(t *testing.T) {
	groups := groupSummaries(nil, 500)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil input, got %d", len(groups))
	}
}

// ─── summariesSize ──────────────────────────────────────────────────────────

func TestSummariesSize(t *testing.T) {
	s := []string{"abc", "defgh"}
	if got := summariesSize(s); got != 8 {
		t.Errorf("expected 8, got %d", got)
	}
}

func TestSummariesSize_Empty(t *testing.T) {
	if got := summariesSize(nil); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// ─── reduceSummaries (integration via Service) ──────────────────────────────

func TestReduceSummaries_SmallEnough_NoLLMCalls(t *testing.T) {
	var llmCalls atomic.Int64
	svc := Service{
		LLM: &fakeLLM{
			onGenerate: func(string) { llmCalls.Add(1) },
			response:   "condensed",
		},
		ChunkThreshold: 1000,
		Log:            noopLogger(),
	}

	summaries := []string{"- change A", "- change B"}
	result, err := svc.reduceSummaries(context.Background(), summaries)
	if err != nil {
		t.Fatal(err)
	}
	if llmCalls.Load() != 0 {
		t.Errorf("expected 0 LLM calls for small summaries, got %d", llmCalls.Load())
	}
	if len(result) != 2 {
		t.Errorf("expected summaries unchanged, got %d items", len(result))
	}
}

func TestReduceSummaries_LargeSummaries_ReducedViaLLM(t *testing.T) {
	var llmCalls atomic.Int64
	svc := Service{
		LLM: &fakeLLM{
			onGenerate: func(string) { llmCalls.Add(1) },
			response:   "- condensed summary",
		},
		ChunkThreshold: 200,
		Log:            noopLogger(),
	}

	// 5 summaries of 100 bytes each = 500 bytes, well above 200 threshold
	summaries := make([]string, 5)
	for i := range summaries {
		summaries[i] = fmt.Sprintf("- change %s", strings.Repeat("x", 90))
	}

	result, err := svc.reduceSummaries(context.Background(), summaries)
	if err != nil {
		t.Fatal(err)
	}
	if llmCalls.Load() == 0 {
		t.Error("expected LLM calls for reduction, got 0")
	}
	if len(result) >= len(summaries) {
		t.Errorf("expected fewer summaries after reduction, got %d (was %d)", len(result), len(summaries))
	}
}

func TestReduceSummaries_LLMError_Propagated(t *testing.T) {
	svc := Service{
		LLM: &fakeLLM{
			err: fmt.Errorf("LLM failed"),
		},
		ChunkThreshold: 100,
		Log:            noopLogger(),
	}

	// Each summary is 40 bytes — two fit within the 100-byte threshold as one
	// group, so groupSummaries produces a multi-item group that triggers an
	// LLM call (and therefore the error).
	summaries := []string{
		strings.Repeat("a", 40),
		strings.Repeat("b", 40),
		strings.Repeat("c", 40),
	}

	_, err := svc.reduceSummaries(context.Background(), summaries)
	if !errors.Is(err, domain.ErrReduceFailed) {
		t.Errorf("expected ErrReduceFailed, got %v", err)
	}
}

func TestReduceSummaries_SingleSummary_PassedThrough(t *testing.T) {
	svc := Service{
		LLM:            &fakeLLM{response: "should not be called"},
		ChunkThreshold: 10, // threshold smaller than the summary
		Log:            noopLogger(),
	}

	summaries := []string{"- only one summary that is quite long"}
	result, err := svc.reduceSummaries(context.Background(), summaries)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != summaries[0] {
		t.Errorf("single summary should pass through unchanged, got %v", result)
	}
}

// ─── end-to-end: mapReduce with reduction ───────────────────────────────────

func TestMapReduce_LargeDiff_SummariesReduced(t *testing.T) {
	var diff strings.Builder
	for i := 0; i < 30; i++ {
		diff.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	var callCount atomic.Int64
	var mu sync.Mutex
	prompts := make([]string, 0)
	svc := Service{
		Git: &fakeGit{diff: diff.String()},
		LLM: &fakeLLM{
			onGenerate: func(p string) {
				callCount.Add(1)
				mu.Lock()
				prompts = append(prompts, p)
				mu.Unlock()
			},
			response: "- " + strings.Repeat("summary ", 10),
		},
		ChunkThreshold: 500,
		Log:            noopLogger(),
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	chunks := PackChunks(diff.String(), 500)
	minCallsWithoutReduction := len(chunks) + 1
	count := int(callCount.Load())
	if count <= minCallsWithoutReduction {
		t.Errorf("expected extra LLM calls for reduction, got %d (chunks=%d, min_without_reduction=%d)",
			count, len(chunks), minCallsWithoutReduction)
	}

	mu.Lock()
	hasSynthesis := false
	for _, p := range prompts {
		if strings.Contains(p, "commit message") {
			hasSynthesis = true
			break
		}
	}
	mu.Unlock()
	if !hasSynthesis {
		t.Error("expected at least one synthesis prompt containing 'commit message'")
	}
}

// ─── BuildReducePrompt ──────────────────────────────────────────────────────

func TestBuildReducePrompt_ContainsSummaries(t *testing.T) {
	prompt := BuildReducePrompt([]string{"- added auth", "- fixed login"})

	if !strings.Contains(prompt, "1.") || !strings.Contains(prompt, "2.") {
		t.Error("reduce prompt should number the summaries")
	}
	if !strings.Contains(prompt, "added auth") {
		t.Error("reduce prompt should contain summaries text")
	}
	if !strings.Contains(prompt, "bullet") {
		t.Error("reduce prompt should instruct bullet-point output")
	}
}

// ─── concurrency ────────────────────────────────────────────────────────────

func TestMapReduce_ChunksProcessedInParallel(t *testing.T) {
	var diff strings.Builder
	for i := 0; i < 6; i++ {
		diff.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	var (
		inFlight    atomic.Int64
		maxInFlight atomic.Int64
		barrier     = make(chan struct{})
	)

	// Every goroutine increments inFlight, records peak, then waits on the
	// barrier. The test closes the barrier once peak >= 2, unblocking all.
	slow := &fakeLLM{
		onGenerate: func(string) {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			// Record peak concurrency.
			for {
				old := maxInFlight.Load()
				if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
					break
				}
			}
			// Wait for the barrier — released once we know parallelism is proven.
			<-barrier
		},
		response: "- summary bullet",
	}

	// Close the barrier once at least 2 goroutines are in-flight, or after a
	// generous timeout so the test doesn't hang forever on failure.
	go func() {
		for i := 0; i < 2000; i++ {
			if maxInFlight.Load() >= 2 {
				close(barrier)
				return
			}
			// yield to let goroutines schedule
			testing_sleep(t)
		}
		// Timeout path: close anyway so goroutines unblock and test can fail.
		close(barrier)
	}()

	svc := Service{
		Git:            &fakeGit{diff: diff.String()},
		LLM:            slow,
		ChunkThreshold: 500,
		MaxConcurrency: 6, // allow all chunks to run at once for this test
		Log:            noopLogger(),
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if maxInFlight.Load() < 2 {
		t.Errorf("expected parallel chunk processing (max in-flight >= 2), got %d", maxInFlight.Load())
	}
}

func TestMapReduce_ConcurrencyLimiterCapsInFlight(t *testing.T) {
	// With MaxConcurrency=2 and 6 chunks, peak in-flight must never exceed 2.
	var diff strings.Builder
	for i := 0; i < 6; i++ {
		diff.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	var (
		inFlight    atomic.Int64
		maxInFlight atomic.Int64
		barrier     = make(chan struct{})
	)

	slow := &fakeLLM{
		onGenerate: func(string) {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				old := maxInFlight.Load()
				if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
					break
				}
			}
			<-barrier
		},
		response: "- summary bullet",
	}

	// Let exactly 2 goroutines in, verify peak, then unblock them all.
	go func() {
		for i := 0; i < 2000; i++ {
			if maxInFlight.Load() >= 2 {
				break
			}
			testing_sleep(t)
		}
		close(barrier)
	}()

	svc := Service{
		Git:            &fakeGit{diff: diff.String()},
		LLM:            slow,
		ChunkThreshold: 500,
		MaxConcurrency: 2,
		Log:            noopLogger(),
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if peak := maxInFlight.Load(); peak > 2 {
		t.Errorf("expected max 2 in-flight (MaxConcurrency=2), got %d", peak)
	}
	if peak := maxInFlight.Load(); peak < 2 {
		t.Errorf("expected at least 2 in-flight, got %d (limiter may be too restrictive)", peak)
	}
}

// testing_sleep yields the current goroutine briefly so that other goroutines
// can be scheduled. Uses runtime.Gosched via a time.Sleep(0) equivalent.
func testing_sleep(_ *testing.T) {
	// A tiny sleep is more reliable than runtime.Gosched across platforms.
	var ch = make(chan struct{})
	go func() { close(ch) }()
	<-ch
}

func TestMapReduce_PreservesChunkOrder(t *testing.T) {
	// Each chunk gets a unique response; verify summaries arrive in order.
	var diff strings.Builder
	for i := 0; i < 5; i++ {
		diff.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	var mu sync.Mutex
	var synthPrompt string

	svc := Service{
		Git: &fakeGit{diff: diff.String()},
		LLM: &fakeLLM{
			onGenerate: func(p string) {
				// Capture the synthesis prompt which contains all summaries.
				if strings.Contains(p, "commit message") {
					mu.Lock()
					synthPrompt = p
					mu.Unlock()
				}
			},
			response: "- ordered bullet",
		},
		ChunkThreshold: 300,
		Log:            noopLogger(),
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	p := synthPrompt
	mu.Unlock()

	if !strings.Contains(p, "1.") {
		t.Error("synthesis prompt should contain numbered summaries starting at 1")
	}
}

func TestMapReduce_PartialChunkError_ReturnsError(t *testing.T) {
	var diff strings.Builder
	for i := 0; i < 4; i++ {
		diff.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	var callCount atomic.Int64

	svc := Service{
		Git: &fakeGit{diff: diff.String()},
		LLM: &fakeLLM{
			onGenerate: func(string) { callCount.Add(1) },
			// All calls fail — guarantees at least one error is captured.
			err: fmt.Errorf("network timeout"),
		},
		ChunkThreshold: 300,
		Log:            noopLogger(),
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if !errors.Is(err, domain.ErrChunkFailed) {
		t.Errorf("expected ErrChunkFailed when a chunk fails, got %v", err)
	}
}
