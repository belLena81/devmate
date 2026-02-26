package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes git commands in a specific working directory
type Runner struct {
	dir string
}

// New returns a Runner rooted at dir. Pass an empty string to use the
// current working directory.
func New(dir string) *Runner {
	return &Runner{dir: dir}
}

// DiffCached returns the output of `git diff --cached` — the staged diff.
// Returns an empty string (and no error) when nothing is staged.
func (r *Runner) DiffCached() (string, error) {
	return r.run("diff", "--cached")
}

// LogBetween returns the commit subject lines that exist in head but not in
// base, excluding merge commits. This is used to gather PR context for LLM
// summarisation without noise from merge-back commits.
// Returns an empty slice (and no error) when base and head are identical.
func (r *Runner) LogBetween(base, head string) ([]string, error) {
	out, err := r.run(
		"log",
		fmt.Sprintf("%s..%s", base, head),
		"--no-merges",
		"--reverse",
		"--format=%s",
	)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// run executes a git subcommand in r.dir and returns combined stdout.
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
