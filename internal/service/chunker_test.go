package service

import (
	"devmate/internal/domain"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestChunkDiff_SplitsOnFileBoundary(t *testing.T) {
	fileFoo := "diff --git a/foo.go b/foo.go\n+change1\n"
	fileBar := "diff --git a/bar.go b/bar.go\n+change2\n"
	diff := fileFoo + fileBar

	// set limit smaller than combined but larger than each file
	// forces them into separate chunks
	chunks := ChunkDiff(diff, len(fileFoo)+1)

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %#v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0], "foo.go") {
		t.Error("first chunk should contain foo.go")
	}
	if !strings.Contains(chunks[1], "bar.go") {
		t.Error("second chunk should contain bar.go")
	}
}

func TestChunkDiff_RespectsMaxSize(t *testing.T) {
	// build a diff larger than the limit
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		b.WriteString("+lots of changes here\n")
	}

	chunks := ChunkDiff(b.String(), 200)

	for i, chunk := range chunks {
		if len(chunk) > 300 { // some tolerance for boundary splits
			t.Errorf("chunk %d exceeds max size: %d", i, len(chunk))
		}
	}
}

func TestChunkDiff_SingleFileUnderLimit_NotSplit(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n+small change\n"

	chunks := ChunkDiff(diff, 1000)

	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestBuildChunkPrompt_IndicatesPosition(t *testing.T) {
	prompt := BuildChunkPrompt("diff content", 2, 5)
	if !strings.Contains(prompt, "2") || !strings.Contains(prompt, "5") {
		t.Error("chunk prompt must indicate chunk position")
	}
}

func TestDraftMessage_LargeDiff_UsesMapReduce(t *testing.T) {
	var callCount atomic.Int64
	fake := &fakeLLM{
		onGenerate: func(prompt string) {
			callCount.Add(1)
		},
		response: "feat: some change",
	}

	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString(fmt.Sprintf("diff --git a/file%d.go b/file%d.go\n", i, i))
		// pad each file diff to ensure total exceeds threshold
		b.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	svc := Service{
		Git:            &fakeGit{diff: b.String()},
		LLM:            fake,
		ChunkThreshold: 500, // inject threshold so test controls it
	}

	_, err := svc.DraftMessage(CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if callCount.Load() < 2 {
		t.Errorf("expected multiple LLM calls for large diff, got %d", callCount.Load())
	}
}

func TestBuildChunkPrompt_ResponseIsCleanBullets(t *testing.T) {
	prompt := BuildChunkPrompt("some diff", 1, 3)

	if !strings.Contains(prompt, "ONLY bullet points") {
		t.Error("chunk prompt must instruct model to respond with only bullet points")
	}
	if !strings.Contains(prompt, "Do not write") {
		t.Error("chunk prompt must forbid prose wrapping")
	}
	if !strings.Contains(prompt, "Start each line with -") {
		t.Error("chunk prompt must specify bullet format")
	}
}

func TestBuildSynthesisPrompt_ReceivesCleanInput(t *testing.T) {
	// synthesis prompt should present summaries as numbered list
	summaries := []string{"- added login flag", "- updated root cmd"}
	prompt := BuildSynthesisPrompt(summaries, domain.Undefined, domain.Short, false)

	if !strings.Contains(prompt, "1.") {
		t.Error("synthesis prompt should number the summaries")
	}
}

// ─── PackChunks ───────────────────────────────────────────────────────────────

func TestPackChunks_SmallFilesPacked(t *testing.T) {
	// Three tiny files that individually fit easily — all should land in one chunk.
	var b strings.Builder
	for i := 0; i < 3; i++ {
		b.WriteString(fmt.Sprintf("diff --git a/tiny%d.go b/tiny%d.go\n+x\n", i, i))
	}
	chunks := PackChunks(b.String(), 1000)
	if len(chunks) != 1 {
		t.Errorf("expected 1 packed chunk for tiny files, got %d", len(chunks))
	}
	if len(chunks[0].Files) != 3 {
		t.Errorf("packed chunk should list 3 files, got %v", chunks[0].Files)
	}
}

func TestPackChunks_LargeFileGetsOwnChunk(t *testing.T) {
	// big.go is 490 bytes — it consumes 98% of the 500-byte threshold.
	// small.go is ~30 bytes. Together they exceed 500, so the packer must
	// flush big.go first and put small.go in a separate chunk.
	bigContent := "diff --git a/big.go b/big.go\n+" + strings.Repeat("x", 490) + "\n"
	smallContent := "diff --git a/small.go b/small.go\n+y\n"
	diff := bigContent + smallContent

	chunks := PackChunks(diff, 500)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	// big.go should not share a chunk with small.go
	for _, chunk := range chunks {
		if len(chunk.Files) > 1 {
			for _, f := range chunk.Files {
				if strings.Contains(f, "big.go") {
					t.Errorf("big.go should not share a chunk: %v", chunk.Files)
				}
			}
		}
	}
}

func TestPackChunks_OversizedFileSplitIntoParts(t *testing.T) {
	// A file that alone exceeds maxSize must be hard-split into numbered parts.
	big := "diff --git a/huge.go b/huge.go\n+" + strings.Repeat("z", 1200) + "\n"
	chunks := PackChunks(big, 500)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple parts for oversized file, got %d chunk(s)", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk.Content) > 600 { // 500 + small tolerance
			t.Errorf("chunk exceeds maxSize: %d bytes, files: %v", len(chunk.Content), chunk.Files)
		}
		for _, f := range chunk.Files {
			if !strings.Contains(f, "huge.go") {
				t.Errorf("part should reference original file, got %q", f)
			}
			if !strings.Contains(f, "part") {
				t.Errorf("part name should say 'part', got %q", f)
			}
		}
	}
}

func TestPackChunks_MixedSizes_PackedEfficiently(t *testing.T) {
	// 1 large file (450 bytes of content) + 4 small files (30 bytes each).
	// threshold = 500. Large file must be alone; small files should all pack together.
	var b strings.Builder
	b.WriteString("diff --git a/large.go b/large.go\n+" + strings.Repeat("L", 450) + "\n")
	for i := 0; i < 4; i++ {
		b.WriteString(fmt.Sprintf("diff --git a/s%d.go b/s%d.go\n+s\n", i, i))
	}

	chunks := PackChunks(b.String(), 500)

	// We expect exactly 2 chunks: one for large.go (alone) and one for all small files.
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks for mixed sizes, got %d", len(chunks))
		for i, c := range chunks {
			t.Logf("  chunk %d files: %v (%d bytes)", i, c.Files, len(c.Content))
		}
	}
}

func TestPackChunks_SortedLargestFirst(t *testing.T) {
	// large.go is listed last in the diff but should end up first in chunks
	// because PackChunks sorts by descending size before packing.
	small := "diff --git a/small.go b/small.go\n+s\n"
	large := "diff --git a/large.go b/large.go\n+" + strings.Repeat("L", 400) + "\n"
	diff := small + large // small appears first in the raw diff

	chunks := PackChunks(diff, 500)

	// large.go must be in its own chunk and that chunk must come first
	// (because we sort descending and large.go exceeds half the threshold)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	firstFiles := chunks[0].Files
	hasLarge := false
	for _, f := range firstFiles {
		if strings.Contains(f, "large.go") {
			hasLarge = true
		}
	}
	if !hasLarge {
		t.Errorf("expected large.go to be in first chunk (sorted by size), got: %v", firstFiles)
	}
}

func TestPackChunks_AllFilesPresent(t *testing.T) {
	// Regardless of packing, every file must appear in exactly one chunk.
	fileNames := []string{"alpha.go", "beta.go", "gamma.go", "delta.go", "epsilon.go"}
	var b strings.Builder
	for i, name := range fileNames {
		b.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n+%s\n", name, name, strings.Repeat("x", (i+1)*80)))
	}

	chunks := PackChunks(b.String(), 400)

	// Track which original files have been seen. Parts of the same file
	// (e.g. "epsilon.go (part 1)") share the same base name, so we strip
	// the " (part N)" suffix before recording.
	seen := make(map[string]bool)
	for _, chunk := range chunks {
		for _, f := range chunk.Files {
			// Strip optional " (part N)" suffix to get the base file name.
			base := f
			if idx := strings.Index(f, " (part "); idx != -1 {
				base = f[:idx]
			}
			seen[base] = true
		}
	}
	for _, name := range fileNames {
		if !seen[name] {
			t.Errorf("file %q missing from all chunks", name)
		}
	}
}

func TestPackChunks_EmptyDiff_ReturnsNil(t *testing.T) {
	if chunks := PackChunks("", 1000); chunks != nil {
		t.Errorf("expected nil for empty diff, got %v", chunks)
	}
}

func TestPackChunks_SingleFile_OneChunk(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n+change\n"
	chunks := PackChunks(diff, 1000)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for single file, got %d", len(chunks))
	}
	if chunks[0].Files[0] != "foo.go" {
		t.Errorf("expected file name foo.go, got %q", chunks[0].Files[0])
	}
}
