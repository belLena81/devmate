package git_test

import (
	"devmate/internal/infra/git"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestRepo creates a real temporary git repository and returns its path.
func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	return dir
}

// defaultBranch returns the branch name git init created (main, master, etc.)
// Must be called after at least one commit exists.
func defaultBranch(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("could not get default branch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// stageFile writes content to a file inside the repo and stages it.
func stageFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd := exec.Command("git", "add", name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
}

// commitAll commits everything currently staged.
func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// checkoutBranch creates and switches to a new branch.
func checkoutBranch(t *testing.T, dir, name string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "-b", name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b %s: %v\n%s", name, err, out)
	}
}

// mergeBranch merges the given branch into the current branch.
func mergeBranch(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "merge", branch, "--no-edit")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge %s: %v\n%s", branch, err, out)
	}
}

// --- DiffCached --------------------------------------------------------------

func TestDiffCached_ReturnsStagedChanges(t *testing.T) {
	dir := newTestRepo(t)
	stageFile(t, dir, "hello.go", "package main\n")

	runner := git.New(dir, noopLogger())
	diff, err := runner.DiffCached()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff for staged file")
	}
	if !strings.Contains(diff, "hello.go") {
		t.Errorf("expected diff to mention hello.go, got:\n%s", diff)
	}
}

func TestDiffCached_EmptyWhenNothingStaged(t *testing.T) {
	dir := newTestRepo(t)
	os.WriteFile(filepath.Join(dir, "unstaged.go"), []byte("package main\n"), 0644)

	runner := git.New(dir, noopLogger())
	diff, err := runner.DiffCached()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff when nothing staged, got:\n%s", diff)
	}
}

func TestDiffCached_ErrorOutsideGitRepo(t *testing.T) {
	runner := git.New(t.TempDir(), noopLogger())
	_, err := runner.DiffCached()
	if err == nil {
		t.Fatal("expected error when run outside a git repo")
	}
}

// --- LogBetween --------------------------------------------------------------

func TestLogBetween_ReturnsOnlyFeatureBranchCommits(t *testing.T) {
	dir := newTestRepo(t)

	stageFile(t, dir, "main.go", "package main\n")
	commitAll(t, dir, "initial commit")
	base := defaultBranch(t, dir)

	checkoutBranch(t, dir, "feature/foo")
	stageFile(t, dir, "a.go", "// a\n")
	commitAll(t, dir, "commit 1")
	stageFile(t, dir, "b.go", "// b\n")
	commitAll(t, dir, "commit 2")
	stageFile(t, dir, "c.go", "// c\n")
	commitAll(t, dir, "commit 3")

	runner := git.New(dir, noopLogger())
	msgs, err := runner.LogBetween(base, "feature/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 commit messages, got %d: %v", len(msgs), msgs)
	}
	// --reverse means oldest first
	expected := []string{"commit 1", "commit 2", "commit 3"}
	for i, want := range expected {
		if msgs[i] != want {
			t.Errorf("msgs[%d]: expected %q, got %q", i, want, msgs[i])
		}
	}
}

func TestLogBetween_ExcludesMergeCommits(t *testing.T) {
	dir := newTestRepo(t)

	stageFile(t, dir, "main.go", "package main\n")
	commitAll(t, dir, "initial commit")
	base := defaultBranch(t, dir)

	// add a commit on base that will be merged back into feature
	stageFile(t, dir, "base_update.go", "// base update\n")
	commitAll(t, dir, "commit 4")

	checkoutBranch(t, dir, "feature/foo")
	stageFile(t, dir, "a.go", "// a\n")
	commitAll(t, dir, "commit 1")
	stageFile(t, dir, "b.go", "// b\n")
	commitAll(t, dir, "commit 2")

	// merge base back into feature (simulating keeping feature up to date)
	mergeBranch(t, dir, base)

	stageFile(t, dir, "c.go", "// c\n")
	commitAll(t, dir, "commit 3")

	runner := git.New(dir, noopLogger())
	msgs, err := runner.LogBetween(base, "feature/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, msg := range msgs {
		if strings.HasPrefix(msg, "Merge") {
			t.Errorf("merge commit should be excluded, got: %q", msg)
		}
		if msg == "commit 4" {
			t.Errorf("base branch commit should be excluded, got: %q", msg)
		}
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 feature commits, got %d: %v", len(msgs), msgs)
	}
}

func TestLogBetween_EmptyWhenNoUniqueCommits(t *testing.T) {
	dir := newTestRepo(t)
	stageFile(t, dir, "main.go", "package main\n")
	commitAll(t, dir, "initial commit")
	base := defaultBranch(t, dir)

	// feature branch with no additional commits
	checkoutBranch(t, dir, "feature/empty")

	runner := git.New(dir, noopLogger())
	msgs, err := runner.LogBetween(base, "feature/empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice, got: %v", msgs)
	}
}

func TestLogBetween_ErrorOnUnknownBranch(t *testing.T) {
	dir := newTestRepo(t)
	stageFile(t, dir, "main.go", "package main\n")
	commitAll(t, dir, "initial commit")

	runner := git.New(dir, noopLogger())
	_, err := runner.LogBetween("HEAD", "nonexistent-branch")
	if err == nil {
		t.Fatal("expected error for unknown branch")
	}
}

func TestLogBetween_ErrorOutsideGitRepo(t *testing.T) {
	runner := git.New(t.TempDir(), noopLogger())
	_, err := runner.LogBetween("main", "feature")
	if err == nil {
		t.Fatal("expected error when run outside a git repo")
	}
}

func TestDiffCached_WhitespaceOnlyChanges_ExcludedFromDiff(t *testing.T) {
	// Stage a file, commit it, then re-stage the same content with only
	// indentation changes. DiffCached must return empty because -w ignores
	// all whitespace differences.
	dir := newTestRepo(t)

	// Commit the original file.
	original := "package main\n\nfunc hello() {\n\treturn\n}\n"
	stageFile(t, dir, "hello.go", original)
	commitAll(t, dir, "initial")

	// Re-write with spaces instead of tabs — whitespace-only change.
	whitespaceOnly := "package main\n\nfunc hello() {\n    return\n}\n"
	stageFile(t, dir, "hello.go", whitespaceOnly)

	runner := git.New(dir, noopLogger())
	diff, err := runner.DiffCached()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for whitespace-only change, got:\n%s", diff)
	}
}

// --- validRef (option injection guard) --------------------------------------

func TestLogBetween_ErrorOnRefStartingWithDash(t *testing.T) {
	dir := newTestRepo(t)
	stageFile(t, dir, "main.go", "package main\n")
	commitAll(t, dir, "initial commit")

	runner := git.New(dir, noopLogger())

	_, err := runner.LogBetween("-p", "HEAD")
	if err == nil {
		t.Error("expected error when base ref starts with '-'")
	}

	_, err = runner.LogBetween("HEAD", "--all")
	if err == nil {
		t.Error("expected error when head ref starts with '-'")
	}
}

func TestLogBetween_ErrorOnEmptyRef(t *testing.T) {
	dir := newTestRepo(t)
	stageFile(t, dir, "main.go", "package main\n")
	commitAll(t, dir, "initial commit")

	runner := git.New(dir, noopLogger())

	_, err := runner.LogBetween("", "HEAD")
	if err == nil {
		t.Error("expected error for empty base ref")
	}

	_, err = runner.LogBetween("HEAD", "")
	if err == nil {
		t.Error("expected error for empty head ref")
	}
}

// --- GIT_DIR isolation -------------------------------------------------------

func TestRunner_IsolatesGitEnv(t *testing.T) {
	// Create two separate repos.
	repoA := newTestRepo(t)
	stageFile(t, repoA, "main.go", "package main\n")
	commitAll(t, repoA, "commit in repo A")
	branchA := defaultBranch(t, repoA)

	checkoutBranch(t, repoA, "feature/a")
	stageFile(t, repoA, "a.go", "// a\n")
	commitAll(t, repoA, "feature commit in A")

	repoB := newTestRepo(t)
	stageFile(t, repoB, "main.go", "package main\n")
	commitAll(t, repoB, "commit in repo B")

	// Leak repoB's GIT_DIR into the environment — simulating an IDE runner.
	t.Setenv("GIT_DIR", repoB+"/.git")

	// Runner for repoA must still see repoA's commits, not repoB's.
	runner := git.New(repoA, noopLogger())
	msgs, err := runner.LogBetween(branchA, "feature/a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 || msgs[0] != "feature commit in A" {
		t.Errorf("expected repoA's commit, got: %v", msgs)
	}
}
