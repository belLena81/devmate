package service

import (
	"regexp"
	"strings"
)

// blankLine matches lines that are empty or contain only whitespace.
var blankLine = regexp.MustCompile(`(?m)^[[:blank:]]*$`)

// multipleBlankLines matches three or more consecutive newlines.
var multipleBlankLines = regexp.MustCompile(`\n{3,}`)

// branchName matches a valid conventional branch name: type/slug.
var branchNamePattern = regexp.MustCompile(`^[a-z]+/[a-z0-9][a-z0-9\-]*$`)

// sanitize cleans raw LLM output for display:
//   - normalises Windows line endings (CRLF → LF)
//   - strips trailing whitespace from every line
//   - collapses whitespace-only lines to truly empty lines
//   - collapses three or more consecutive newlines to exactly two
//   - trims leading and trailing blank lines
func sanitize(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	s = strings.ReplaceAll(s, "\r\n", "\n")

	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	s = strings.Join(lines, "\n")

	s = blankLine.ReplaceAllString(s, "")
	s = multipleBlankLines.ReplaceAllString(s, "\n\n")

	return strings.Trim(s, "\n")
}

// extractBranchName scans a raw LLM response line by line and returns the
// first line that looks like a valid branch name (type/slug).
// This handles small models that prefix their answer with preamble like
// "Based on the task, I would use:" or "Here is the branch name:".
// If no valid branch name is found, the sanitized full response is returned
// as a fallback so we never silently discard output.
func extractBranchName(s string) string {
	s = sanitize(s)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if branchNamePattern.MatchString(line) {
			return line
		}
	}
	return s // fallback: return whatever sanitize produced
}
