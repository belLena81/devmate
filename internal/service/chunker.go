package service

import (
	"fmt"
	"sort"
	"strings"
)

// Chunk is one piece of a diff together with the file names it covers.
// File names are used only for logging — the content is what matters to the LLM.
type Chunk struct {
	Content string
	Files   []string
}

// ChunkDiff splits a unified diff into chunks whose size does not exceed
// maxSize bytes. It splits on file boundaries ("diff --git …") wherever
// possible; if a single file's diff exceeds maxSize it falls back to a hard
// byte split.
func ChunkDiff(diff string, maxSize int) []string {
	fileDiffs := splitOnFileBoundary(diff)

	var chunks []string
	var current strings.Builder

	for _, fileDiff := range fileDiffs {
		if len(fileDiff) > maxSize {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			chunks = append(chunks, hardSplit(fileDiff, maxSize)...)
			continue
		}
		if current.Len()+len(fileDiff) > maxSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteString(fileDiff)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

// ChunkDiffWithMeta is like ChunkDiff but returns Chunk values that also carry
// the file names covered by each chunk. Used by mapReduce for structured logs.
func ChunkDiffWithMeta(diff string, maxSize int) []Chunk {
	fileDiffs := splitOnFileBoundary(diff)

	var chunks []Chunk
	var currentContent strings.Builder
	var currentFiles []string

	flush := func() {
		if currentContent.Len() > 0 {
			chunks = append(chunks, Chunk{
				Content: currentContent.String(),
				Files:   currentFiles,
			})
			currentContent.Reset()
			currentFiles = nil
		}
	}

	for _, fileDiff := range fileDiffs {
		fileName := extractFileName(fileDiff)

		if len(fileDiff) > maxSize {
			flush()
			for i, part := range hardSplit(fileDiff, maxSize) {
				chunks = append(chunks, Chunk{
					Content: part,
					Files:   []string{fmt.Sprintf("%s (part %d)", fileName, i+1)},
				})
			}
			continue
		}
		if currentContent.Len()+len(fileDiff) > maxSize {
			flush()
		}
		currentContent.WriteString(fileDiff)
		currentFiles = append(currentFiles, fileName)
	}

	flush()
	return chunks
}

// splitOnFileBoundary splits a unified diff into per-file sections. Each
// section starts with its own "diff --git …" header line.
func splitOnFileBoundary(diff string) []string {
	var files []string
	var current strings.Builder

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") && current.Len() > 0 {
			files = append(files, current.String())
			current.Reset()
		}
		current.WriteString(line + "\n")
	}
	if current.Len() > 0 {
		files = append(files, current.String())
	}
	return files
}

// extractFileName pulls the file path from the first line of a file diff.
// "diff --git a/foo.go b/foo.go" → "foo.go"
func extractFileName(fileDiff string) string {
	for _, line := range strings.SplitN(fileDiff, "\n", 2) {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return strings.TrimPrefix(parts[2], "a/")
			}
		}
	}
	return "unknown"
}

// hardSplit breaks a string into parts of at most maxSize bytes regardless of
// content. Used as a last resort when a single file diff exceeds the limit.
func hardSplit(content string, maxSize int) []string {
	if maxSize <= 0 {
		return []string{content}
	}
	var parts []string
	for len(content) > maxSize {
		parts = append(parts, content[:maxSize])
		content = content[maxSize:]
	}
	if len(content) > 0 {
		parts = append(parts, content)
	}
	return parts
}

// PackChunks splits a unified diff into Chunks that fit within maxSize bytes,
// packing multiple small files together to minimise LLM round-trips while
// keeping large files in their own chunks.
//
// Algorithm:
//  1. Split the diff on file boundaries into individual file diffs.
//  2. Sort file diffs by size descending so the largest files are placed first.
//     This front-loads the expensive solo chunks and lets small files fill the
//     tail slots compactly.
//  3. Greedy bin-pack: accumulate files into the current chunk until the next
//     file would push it over maxSize, then flush and start a new chunk.
//  4. Files that alone exceed maxSize are hard-split into byte-level parts,
//     each part becoming its own Chunk.
func PackChunks(diff string, maxSize int) []Chunk {
	if strings.TrimSpace(diff) == "" {
		return nil
	}

	fileDiffs := splitOnFileBoundary(diff)
	if maxSize <= 0 {
		return []Chunk{{Content: diff, Files: []string{"unknown"}}}
	}

	// Step 1: parse each file diff into a named record.
	type namedDiff struct {
		name    string
		content string
	}
	items := make([]namedDiff, 0, len(fileDiffs))
	for _, fd := range fileDiffs {
		items = append(items, namedDiff{
			name:    extractFileName(fd),
			content: fd,
		})
	}

	// Step 2: sort descending by size so large files come first.
	sort.Slice(items, func(i, j int) bool {
		return len(items[i].content) > len(items[j].content)
	})

	// Step 3 & 4: greedy bin-pack with hard-split fallback.
	var chunks []Chunk
	var currentContent strings.Builder
	var currentFiles []string

	flush := func() {
		if currentContent.Len() > 0 {
			chunks = append(chunks, Chunk{
				Content: currentContent.String(),
				Files:   currentFiles,
			})
			currentContent.Reset()
			currentFiles = nil
		}
	}

	for _, item := range items {
		if len(item.content) > maxSize {
			// File is larger than the limit even alone — flush pending work
			// and hard-split this file into byte-level parts.
			flush()
			for i, part := range hardSplit(item.content, maxSize) {
				chunks = append(chunks, Chunk{
					Content: part,
					Files:   []string{fmt.Sprintf("%s (part %d)", item.name, i+1)},
				})
			}
			continue
		}

		// File fits within the limit — check whether it still fits alongside
		// whatever is already in the current accumulator.
		if currentContent.Len()+len(item.content) > maxSize {
			flush()
		}
		currentContent.WriteString(item.content)
		currentFiles = append(currentFiles, item.name)
	}

	flush()
	return chunks
}
