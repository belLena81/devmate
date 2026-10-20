package git

import (
	"bytes"
	"devmate/internal/domain"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Runner executes git commands in a working directory; it implements domain.GitClient.
type Runner struct {
	dir  string
	root func() (string, error)
	log  *slog.Logger
}

// New returns a Runner rooted at dir. An empty dir resolves the enclosing repository lazily on first use.
func New(dir string, log *slog.Logger) *Runner {
	r := &Runner{dir: dir, log: log.With("component", "git")}
	if dir == "" {
		r.root = sync.OnceValues(RepoRoot)
	}
	return r
}

// RepoRoot returns the top-level directory of the repository enclosing the current directory.
func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", domain.ErrNotGitRepository
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Runner) workDir() (string, error) {
	if r.root == nil {
		return r.dir, nil
	}
	return r.root()
}

// DiffCached returns the staged diff, ignoring whitespace-only changes.
func (r *Runner) DiffCached() (string, error) {
	r.log.Debug("running diff --cached")
	out, err := r.run("diff", "--cached", "-w")
	if err != nil {
		r.log.Debug("diff --cached failed", "error", err)
		return "", err
	}
	r.log.Debug("diff --cached completed", "bytes", len(out))
	return out, nil
}

// validRef rejects refs that git could parse as options.
func validRef(ref string) error {
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("%w %q: ref names may not start with '-'", domain.ErrInvalidRef, ref)
	}
	if ref == "" {
		return domain.ErrEmptyRef
	}
	return nil
}

// LogBetween returns the subjects of commits in head but not in base, excluding merges.
func (r *Runner) LogBetween(base, head string) ([]string, error) {
	if err := validRef(base); err != nil {
		return nil, err
	}
	if err := validRef(head); err != nil {
		return nil, err
	}
	r.log.Debug("running log between", "base", base, "head", head)
	out, err := r.run(
		"log",
		"--no-merges",
		"--reverse",
		"--format=%s",
		"--end-of-options",
		fmt.Sprintf("%s..%s", base, head),
	)
	if err != nil {
		r.log.Debug("log between failed", "base", base, "head", head, "error", err)
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

func (r *Runner) run(args ...string) (string, error) {
	dir, err := r.workDir()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = filteredEnv()

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

// filteredEnv returns os.Environ() without GIT_* variables so the caller's git context cannot leak into subprocesses.
func filteredEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "GIT_") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
