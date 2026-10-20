package service

import (
	"regexp"
	"strings"
)

var blankLine = regexp.MustCompile(`(?m)^[[:blank:]]*$`)

var multipleBlankLines = regexp.MustCompile(`\n{3,}`)

var branchNamePattern = regexp.MustCompile(`^[a-z]+/[a-z0-9][a-z0-9\-]*$`)

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

// extractBranchName skips any preamble small models emit and returns the first type/slug line, or the whole sanitized response.
func extractBranchName(s string) string {
	s = sanitize(s)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if branchNamePattern.MatchString(line) {
			return line
		}
	}
	return s
}
