package service

import (
	"fmt"
	"strings"
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
	llmCalls := 0
	svc := Service{
		LLM: &fakeLLM{
			onGenerate: func(string) { llmCalls++ },
			response:   "condensed",
		},
		ChunkThreshold: 1000,
		Log:            noopLogger(),
	}

	summaries := []string{"- change A", "- change B"}
	result, err := svc.reduceSummaries(summaries)
	if err != nil {
		t.Fatal(err)
	}
	if llmCalls != 0 {
		t.Errorf("expected 0 LLM calls for small summaries, got %d", llmCalls)
	}
	if len(result) != 2 {
		t.Errorf("expected summaries unchanged, got %d items", len(result))
	}
}

func TestReduceSummaries_LargeSummaries_ReducedViaLLM(t *testing.T) {
	llmCalls := 0
	svc := Service{
		LLM: &fakeLLM{
			onGenerate: func(string) { llmCalls++ },
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

	result, err := svc.reduceSummaries(summaries)
	if err != nil {
		t.Fatal(err)
	}
	if llmCalls == 0 {
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

	_, err := svc.reduceSummaries(summaries)
	if err == nil {
		t.Fatal("expected error to propagate from LLM")
	}
	if !strings.Contains(err.Error(), "reducing summary group") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReduceSummaries_SingleSummary_PassedThrough(t *testing.T) {
	svc := Service{
		LLM:            &fakeLLM{response: "should not be called"},
		ChunkThreshold: 10, // threshold smaller than the summary
		Log:            noopLogger(),
	}

	summaries := []string{"- only one summary that is quite long"}
	result, err := svc.reduceSummaries(summaries)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0] != summaries[0] {
		t.Errorf("single summary should pass through unchanged, got %v", result)
	}
}

// ─── end-to-end: mapReduce with reduction ───────────────────────────────────

func TestMapReduce_LargeDiff_SummariesReduced(t *testing.T) {
	// Generate a diff large enough to produce many chunks, where each chunk
	// summary is also large enough that the combined summaries exceed threshold.
	var diff strings.Builder
	for i := 0; i < 30; i++ {
		diff.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	callCount := 0
	prompts := make([]string, 0)
	svc := Service{
		Git: &fakeGit{diff: diff.String()},
		LLM: &fakeLLM{
			onGenerate: func(p string) {
				callCount++
				prompts = append(prompts, p)
			},
			// Each chunk summary is ~100 bytes — with 30 chunks that's ~3000 bytes
			// which exceeds a 500-byte threshold, triggering reduction.
			response: "- " + strings.Repeat("summary ", 10),
		},
		ChunkThreshold: 500,
		Log:            noopLogger(),
	}

	_, err := svc.DraftMessage(CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Expect: chunk summarization calls + reduction calls + 1 synthesis call
	// The exact count depends on packing, but it must be more than just chunks + 1
	// (which was the old behavior without reduction).
	chunks := PackChunks(diff.String(), 500)
	minCallsWithoutReduction := len(chunks) + 1
	if callCount <= minCallsWithoutReduction {
		t.Errorf("expected extra LLM calls for reduction, got %d (chunks=%d, min_without_reduction=%d)",
			callCount, len(chunks), minCallsWithoutReduction)
	}

	// The last prompt should be a synthesis prompt (contains "commit message")
	lastPrompt := prompts[len(prompts)-1]
	if !strings.Contains(lastPrompt, "commit message") {
		t.Error("last LLM call should be the synthesis prompt")
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
