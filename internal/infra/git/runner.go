package git

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// Runner executes git commands in a specific working directory.
// It implements domain.GitClient.
type Runner struct {
	dir string
	log *slog.Logger
}

// New returns a Runner rooted at dir. Pass an empty string to use the
// current working directory.
func New(dir string, log *slog.Logger) *Runner {
	return &Runner{
		dir: dir,
		log: log.With("component", "git"),
	}
}

func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// DiffCached returns the output of `git diff --cached` — the staged diff.
// Returns an empty string (and no error) when nothing is staged.
func (r *Runner) DiffCached() (string, error) {
	r.log.Debug("running diff --cached")
	out, err := r.run("diff", "--cached")
	if err != nil {
		r.log.Error("diff --cached failed", "error", err)
		return "", err
	}
	r.log.Debug("diff --cached completed", "bytes", len(out))
	return out, nil
}

// LogBetween returns the commit subject lines that exist in head but not in
// base, excluding merge commits. This is used to gather PR context for LLM
// summarisation without noise from merge-back commits.
// Returns an empty slice (and no error) when base and head are identical.
func (r *Runner) LogBetween(base, head string) ([]string, error) {
	r.log.Debug("running log between", "base", base, "head", head)
	out, err := r.run(
		"log",
		fmt.Sprintf("%s..%s", base, head),
		"--no-merges",
		"--reverse",
		"--format=%s",
	)
	if err != nil {
		r.log.Error("log between failed", "base", base, "head", head, "error", err)
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		r.log.Debug("log between returned no commits", "base", base, "head", head)
		return []string{}, nil
	}
	msgs := strings.Split(out, "\n")
	r.log.Debug("log between completed", "base", base, "head", head, "commits", len(msgs))
	return msgs, nil
}

// run executes a git subcommand in r.dir and returns stdout.
// stderr is captured and included in the error message on failure so
// callers get actionable context without needing to inspect os.Stderr.
func (r *Runner) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}

	return stdout.String(), nil
}
