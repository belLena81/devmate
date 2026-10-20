package service

import (
	"fmt"
	"slices"
	"strings"
)

// Chunk is one piece of a diff together with the file names it covers.
type Chunk struct {
	Content string
	Files   []string
}

// ChunkDiff splits a unified diff into chunks of at most maxSize bytes, on file boundaries where possible.
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

// PackChunks bin-packs a unified diff into Chunks of at most maxSize bytes, largest files first.
func PackChunks(diff string, maxSize int) []Chunk {
	if strings.TrimSpace(diff) == "" {
		return nil
	}

	fileDiffs := splitOnFileBoundary(diff)
	if maxSize <= 0 {
		return []Chunk{{Content: diff, Files: []string{"unknown"}}}
	}

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

	slices.SortStableFunc(items, func(a, b namedDiff) int {
		return len(b.content) - len(a.content)
	})

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
			flush()
			for i, part := range hardSplit(item.content, maxSize) {
				chunks = append(chunks, Chunk{
					Content: part,
					Files:   []string{fmt.Sprintf("%s (part %d)", item.name, i+1)},
				})
			}
			continue
		}

		if currentContent.Len()+len(item.content) > maxSize {
			flush()
		}
		currentContent.WriteString(item.content)
		currentFiles = append(currentFiles, item.name)
	}

	flush()
	return chunks
}
