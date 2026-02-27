package service

import (
	"devmate/internal/domain"
	"strings"
	"testing"
)

// ─── Template smoke tests ─────────────────────────────────────────────────────
// These verify that all three embedded templates parse and render without
// panicking. A corrupt or missing template file would fail at compile time
// (embed) or here at test time (mustRender).

func TestTemplates_RenderWithoutPanic(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("commit template panicked: %v", r)
			}
		}()
		BuildCommitPrompt("some diff", CommitOptions{})
	})

	t.Run("branch", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("branch template panicked: %v", r)
			}
		}()
		BuildBranchPrompt(BranchOptions{Task: "some task"})
	})

	t.Run("pr", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("pr template panicked: %v", r)
			}
		}()
		BuildPrPrompt([]string{"feat: something"}, PrOptions{
			SourceBranch: "feature/x", DestinationBranch: "main",
		})
	})
}

func TestBuildCommitPrompt_ContainsDiff(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n+func hello() {}"
	prompt := BuildCommitPrompt(diff, CommitOptions{})
	if !strings.Contains(prompt, diff) {
		t.Errorf("expected prompt to contain diff, got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_ContainsConventionalCommitInstruction(t *testing.T) {
	prompt := BuildCommitPrompt("some diff", CommitOptions{})
	if !strings.Contains(prompt, "Conventional Commit") {
		t.Errorf("expected prompt to mention Conventional Commit format, got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_ShortMode_RequestsSingleLine(t *testing.T) {
	opts := CommitOptions{domain.Options{Mode: domain.Short}}
	prompt := BuildCommitPrompt("diff", opts)
	if !strings.Contains(prompt, "single") && !strings.Contains(prompt, "one line") && !strings.Contains(prompt, "concise") {
		t.Errorf("expected short mode prompt to request a single-line message, got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_DetailedMode_RequestsBody(t *testing.T) {
	opts := CommitOptions{domain.Options{Mode: domain.Detailed}}
	prompt := BuildCommitPrompt("diff", opts)
	if !strings.Contains(prompt, "body") && !strings.Contains(prompt, "detailed") {
		t.Errorf("expected detailed mode prompt to request a body, got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_WithTypeOverride_IncludesType(t *testing.T) {
	opts := CommitOptions{domain.Options{Type: domain.Fix}}
	prompt := BuildCommitPrompt("diff", opts)
	if !strings.Contains(prompt, "fix") {
		t.Errorf("expected prompt to include forced type 'fix', got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_WithoutTypeOverride_AsksToDetect(t *testing.T) {
	opts := CommitOptions{domain.Options{Type: domain.Undefined}}
	p := strings.ToLower(BuildCommitPrompt("diff", opts))
	if !strings.Contains(p, "detect") && !strings.Contains(p, "infer") && !strings.Contains(p, "determine") {
		t.Errorf("expected prompt to ask LLM to detect type, got:\n%s", p)
	}
}

func TestBuildCommitPrompt_ExplainMode_AsksForReasoning(t *testing.T) {
	opts := CommitOptions{domain.Options{Explain: true}}
	prompt := BuildCommitPrompt("diff", opts)
	if !strings.Contains(prompt, "explain") && !strings.Contains(prompt, "reasoning") && !strings.Contains(prompt, "reason") {
		t.Errorf("expected explain mode prompt to request reasoning, got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_NoExplain_DoesNotAskForReasoning(t *testing.T) {
	opts := CommitOptions{domain.Options{Explain: false}}
	prompt := BuildCommitPrompt("diff", opts)
	// should not ask for explanation when not requested
	if strings.Contains(prompt, "explain why") || strings.Contains(prompt, "reasoning") {
		t.Errorf("expected no reasoning request when explain=false, got:\n%s", prompt)
	}
}

func TestBuildCommitPrompt_OutputOnlyCommitMessage(t *testing.T) {
	prompt := BuildCommitPrompt("diff", CommitOptions{})
	if !strings.Contains(prompt, "only") && !strings.Contains(prompt, "nothing else") {
		t.Errorf("expected prompt to instruct output of commit message only, got:\n%s", prompt)
	}
}

// ─── BuildBranchPrompt ───────────────────────────────────────────────────────

func TestBuildBranchPrompt_ContainsTask(t *testing.T) {
	opts := BranchOptions{Task: "add user authentication"}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "add user authentication") {
		t.Errorf("expected prompt to contain task, got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_ContainsSlugFormatInstruction(t *testing.T) {
	opts := BranchOptions{Task: "some task"}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "kebab") && !strings.Contains(prompt, "lowercase") && !strings.Contains(prompt, "slug") {
		t.Errorf("expected prompt to mention slug/kebab-case format, got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_ContainsTypeSlashNameFormat(t *testing.T) {
	opts := BranchOptions{Task: "some task"}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "type/") && !strings.Contains(prompt, "<type>") {
		t.Errorf("expected prompt to describe type/name format, got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_WithTypeOverride_IncludesType(t *testing.T) {
	opts := BranchOptions{Task: "fix null pointer", Options: domain.Options{Type: domain.Fix}}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "fix") {
		t.Errorf("expected prompt to include forced type 'fix', got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_WithoutTypeOverride_AsksToDetect(t *testing.T) {
	opts := BranchOptions{Task: "some task", Options: domain.Options{Type: domain.Undefined}}
	p := strings.ToLower(BuildBranchPrompt(opts))
	if !strings.Contains(p, "detect") && !strings.Contains(p, "infer") && !strings.Contains(p, "determine") && !strings.Contains(p, "choose") {
		t.Errorf("expected prompt to ask LLM to choose/detect type, got:\n%s", p)
	}
}

func TestBuildBranchPrompt_ShortMode_RequestsConciseName(t *testing.T) {
	opts := BranchOptions{Task: "task", Options: domain.Options{Mode: domain.Short}}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "concise") && !strings.Contains(prompt, "short") && !strings.Contains(prompt, "brief") {
		t.Errorf("expected short mode to request concise name, got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_DetailedMode_AllowsLongerName(t *testing.T) {
	opts := BranchOptions{Task: "task", Options: domain.Options{Mode: domain.Detailed}}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "descriptive") && !strings.Contains(prompt, "detailed") && !strings.Contains(prompt, "verbose") {
		t.Errorf("expected detailed mode to allow longer name, got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_ExplainMode_AsksForReasoning(t *testing.T) {
	opts := BranchOptions{Task: "task", Options: domain.Options{Explain: true}}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "explain") && !strings.Contains(prompt, "reasoning") && !strings.Contains(prompt, "reason") {
		t.Errorf("expected explain mode prompt to request reasoning, got:\n%s", prompt)
	}
}

func TestBuildBranchPrompt_OutputOnlyBranchName(t *testing.T) {
	opts := BranchOptions{Task: "task"}
	prompt := BuildBranchPrompt(opts)
	if !strings.Contains(prompt, "only") && !strings.Contains(prompt, "nothing else") {
		t.Errorf("expected prompt to instruct output of branch name only, got:\n%s", prompt)
	}
}

// ─── BuildPrPrompt ───────────────────────────────────────────────────────────

func TestBuildPrPrompt_ContainsCommitMessages(t *testing.T) {
	commits := []string{"feat: add login", "fix: null check", "chore: update deps"}
	opts := PrOptions{SourceBranch: "feature/foo", DestinationBranch: "main"}
	prompt := BuildPrPrompt(commits, opts)
	for _, c := range commits {
		if !strings.Contains(prompt, c) {
			t.Errorf("expected prompt to contain commit %q, got:\n%s", c, prompt)
		}
	}
}

func TestBuildPrPrompt_ContainsBranchNames(t *testing.T) {
	opts := PrOptions{SourceBranch: "feature/auth", DestinationBranch: "main"}
	prompt := BuildPrPrompt([]string{"feat: add auth"}, opts)
	if !strings.Contains(prompt, "feature/auth") {
		t.Errorf("expected prompt to contain source branch, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "main") {
		t.Errorf("expected prompt to contain destination branch, got:\n%s", prompt)
	}
}

func TestBuildPrPrompt_ContainsPRTitleAndDescriptionInstruction(t *testing.T) {
	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	prompt := BuildPrPrompt([]string{"feat: something"}, opts)
	if !strings.Contains(prompt, "title") {
		t.Errorf("expected prompt to request a PR title, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "description") && !strings.Contains(prompt, "body") {
		t.Errorf("expected prompt to request a PR description, got:\n%s", prompt)
	}
}

func TestBuildPrPrompt_ShortMode_RequestsConciseSummary(t *testing.T) {
	opts := PrOptions{
		SourceBranch:      "feature/x",
		DestinationBranch: "main",
		Options:           domain.Options{Mode: domain.Short},
	}
	prompt := BuildPrPrompt([]string{"feat: something"}, opts)
	if !strings.Contains(prompt, "concise") && !strings.Contains(prompt, "brief") && !strings.Contains(prompt, "short") {
		t.Errorf("expected short mode to request concise summary, got:\n%s", prompt)
	}
}

func TestBuildPrPrompt_DetailedMode_RequestsFullDescription(t *testing.T) {
	opts := PrOptions{
		SourceBranch:      "feature/x",
		DestinationBranch: "main",
		Options:           domain.Options{Mode: domain.Detailed},
	}
	prompt := BuildPrPrompt([]string{"feat: something"}, opts)
	if !strings.Contains(prompt, "detailed") && !strings.Contains(prompt, "comprehensive") && !strings.Contains(prompt, "full") {
		t.Errorf("expected detailed mode to request full description, got:\n%s", prompt)
	}
}

func TestBuildPrPrompt_WithTypeOverride_IncludesType(t *testing.T) {
	opts := PrOptions{
		SourceBranch:      "feature/x",
		DestinationBranch: "main",
		Options:           domain.Options{Type: domain.Feat},
	}
	prompt := BuildPrPrompt([]string{"add something"}, opts)
	if !strings.Contains(prompt, "feat") {
		t.Errorf("expected prompt to include forced type 'feat', got:\n%s", prompt)
	}
}

func TestBuildPrPrompt_WithoutTypeOverride_AsksToDetect(t *testing.T) {
	opts := PrOptions{
		SourceBranch:      "feature/x",
		DestinationBranch: "main",
		Options:           domain.Options{Type: domain.Undefined},
	}
	p := strings.ToLower(BuildPrPrompt([]string{"add something"}, opts))
	if !strings.Contains(p, "detect") && !strings.Contains(p, "infer") && !strings.Contains(p, "determine") {
		t.Errorf("expected prompt to ask LLM to detect type, got:\n%s", p)
	}
}

func TestBuildPrPrompt_ExplainMode_AsksForReasoning(t *testing.T) {
	opts := PrOptions{
		SourceBranch:      "feature/x",
		DestinationBranch: "main",
		Options:           domain.Options{Explain: true},
	}
	prompt := BuildPrPrompt([]string{"feat: something"}, opts)
	if !strings.Contains(prompt, "explain") && !strings.Contains(prompt, "reasoning") && !strings.Contains(prompt, "reason") {
		t.Errorf("expected explain mode to request reasoning, got:\n%s", prompt)
	}
}

func TestBuildPrPrompt_OutputOnlyPRContent(t *testing.T) {
	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	prompt := BuildPrPrompt([]string{"feat: something"}, opts)
	if !strings.Contains(prompt, "only") && !strings.Contains(prompt, "nothing else") {
		t.Errorf("expected prompt to instruct output of PR content only, got:\n%s", prompt)
	}
}
