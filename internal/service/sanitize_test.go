package service

import "testing"

func TestSanitize_TrimsLeadingAndTrailingBlankLines(t *testing.T) {
	got := sanitize("\n\nfeat: title\n\n")
	want := "feat: title"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestSanitize_CollapsesMultipleBlankLines(t *testing.T) {
	got := sanitize("feat: title\n\n\n\nbody here")
	want := "feat: title\n\nbody here"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestSanitize_CollapsesBlanksWithSpaces(t *testing.T) {
	// LLM sometimes emits " \n \n" — lines containing only spaces.
	got := sanitize("feat: title\n \n \nbody")
	want := "feat: title\n\nbody"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestSanitize_RemovesTrailingSpacesPerLine(t *testing.T) {
	got := sanitize("feat: title   \n\nbody    ")
	want := "feat: title\n\nbody"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestSanitize_NormalizesWindowsLineEndings(t *testing.T) {
	got := sanitize("feat: title\r\n\r\nbody\r\n")
	want := "feat: title\n\nbody"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestSanitize_AlreadyCleanInputUnchanged(t *testing.T) {
	input := "## Summary\n\nSome text\n\n## Changes"
	if got := sanitize(input); got != input {
		t.Errorf("clean input should be unchanged, got %q", got)
	}
}

func TestSanitize_PreservesBulletList(t *testing.T) {
	input := "feat: title\n\n• point one\n• point two"
	if got := sanitize(input); got != input {
		t.Errorf("bullet list should be unchanged, got %q", got)
	}
}

func TestSanitize_EmptyInputReturnsEmpty(t *testing.T) {
	if got := sanitize(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestSanitize_WhitespaceOnlyReturnsEmpty(t *testing.T) {
	if got := sanitize("   \n\n\n  "); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ─── extractBranchName ────────────────────────────────────────────────────────

func TestExtractBranchName_CleanInput_ReturnedAsIs(t *testing.T) {
	got := extractBranchName("feat/add-auth")
	if got != "feat/add-auth" {
		t.Errorf("got %q", got)
	}
}

func TestExtractBranchName_PreambleLine_Stripped(t *testing.T) {
	input := "Based on the task description, I would use:\nfeat/redact-limit"
	got := extractBranchName(input)
	if got != "feat/redact-limit" {
		t.Errorf("got %q", got)
	}
}

func TestExtractBranchName_HereIsPrefix_Stripped(t *testing.T) {
	input := "Here is the generated branch name:\nfeat/redact-limit"
	got := extractBranchName(input)
	if got != "feat/redact-limit" {
		t.Errorf("got %q", got)
	}
}

func TestExtractBranchName_BranchNameOnFirstLine_Returned(t *testing.T) {
	input := "feat/redact-limit\n\nReasoning:\nChose feat because..."
	got := extractBranchName(input)
	if got != "feat/redact-limit" {
		t.Errorf("got %q", got)
	}
}

func TestExtractBranchName_MultiWordPreamble_BranchFound(t *testing.T) {
	input := "Based on the task description, I would determine the branch type as \"feat\".\nHere is the generated concise and brief branch name:\nfeat/redact-limit"
	got := extractBranchName(input)
	if got != "feat/redact-limit" {
		t.Errorf("got %q", got)
	}
}

func TestExtractBranchName_NoBranchFound_FallsBackToSanitized(t *testing.T) {
	// If no line matches type/slug, return sanitized content rather than empty.
	input := "I cannot determine the branch name from this task."
	got := extractBranchName(input)
	if got == "" {
		t.Error("fallback should return sanitized input, not empty string")
	}
}

func TestExtractBranchName_AllValidTypes_Recognised(t *testing.T) {
	for _, typ := range []string{"feat", "fix", "chore", "docs", "refactor"} {
		name := typ + "/some-slug"
		got := extractBranchName("preamble\n" + name)
		if got != name {
			t.Errorf("type %q: got %q", typ, got)
		}
	}
}
