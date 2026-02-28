package cli_test

// app_test.go uses the external test package (cli_test) so it can only access
// exported identifiers — the same surface a real caller (main.go) would use.
// This keeps the tests honest about the public API of the App.

import (
	"bytes"
	"context"
	"devmate/cli"
	"devmate/internal/config"
	"devmate/internal/domain"
	"devmate/internal/service"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type stubLLM struct {
	response string
	err      error
}

func (s *stubLLM) Generate(_ context.Context, _ string) (string, error) { return s.response, s.err }

// spyCommitService captures the options passed to DraftMessage.
type spyCommitService struct {
	receivedOpts service.CommitOptions
	response     string
	err          error
}

func (s *spyCommitService) DraftMessage(_ context.Context, o service.CommitOptions) (string, error) {
	s.receivedOpts = o
	return s.response, s.err
}

// spyBranchService captures the options passed to DraftBranchName.
type spyBranchService struct {
	receivedOpts service.BranchOptions
	response     string
	err          error
}

func (s *spyBranchService) DraftBranchName(_ context.Context, o service.BranchOptions) (string, error) {
	s.receivedOpts = o
	return s.response, s.err
}

// spyPrService captures the options passed to DraftPrDescription.
type spyPrService struct {
	receivedOpts service.PrOptions
	response     string
	err          error
}

func (s *spyPrService) DraftPrDescription(_ context.Context, o service.PrOptions) (string, error) {
	s.receivedOpts = o
	return s.response, s.err
}

func newApp(t *testing.T) *cli.App {
	t.Helper()
	app, err := cli.NewApp(&stubLLM{})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	return app
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestNewAppWithService_DoesNotMutateService asserts that NewAppWithService
// does not modify any field of the Service it receives. The caller (main.go)
// retains full ownership of the service after construction.
func TestNewAppWithService_DoesNotMutateService(t *testing.T) {
	svc := service.New(nil, &stubLLM{}, service.NoopCache{}, config.DefaultOllamaModel, noopLogger(), service.WithChunkThreshold(0))

	_, err := cli.NewAppWithService(svc)
	if err != nil {
		t.Fatalf("NewAppWithService: %v", err)
	}

	// The constructor must never assign a new GitClient to the caller's service.
	if svc.Git() != nil {
		t.Error("NewAppWithService must not mutate svc.Git (unexpected side-effect)")
	}
}

// ---------------------------------------------------------------------------
// App construction
// ---------------------------------------------------------------------------

func TestNewApp_ReturnsNonNilApp(t *testing.T) {
	app, err := cli.NewApp(&stubLLM{})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
}

func TestApp_RootCmd_IsNamed_devmate(t *testing.T) {
	if newApp(t).RootCmd().Name() != "devmate" {
		t.Errorf("expected root command name %q", "devmate")
	}
}

func TestNewAppWithService_ReturnsNonNilApp(t *testing.T) {
	svc := service.New(nil, &stubLLM{}, service.NoopCache{}, config.DefaultOllamaModel, noopLogger(), service.WithChunkThreshold(0))
	app, err := cli.NewAppWithService(svc)
	if err != nil {
		t.Fatalf("NewAppWithService: %v", err)
	}
	if app == nil {
		t.Fatal("NewAppWithService returned nil")
	}
}

// ---------------------------------------------------------------------------
// Subcommand registration
// ---------------------------------------------------------------------------

func TestApp_CacheCmd_IsRegistered(t *testing.T) {
	cmd, _, err := newApp(t).RootCmd().Find([]string{"cache"})
	if err != nil || cmd.Name() != "cache" {
		t.Fatal("cache command not registered")
	}
}

func TestApp_CacheCleanCmd_IsRegistered(t *testing.T) {
	cmd, _, err := newApp(t).RootCmd().Find([]string{"cache", "clean"})
	if err != nil || cmd.Name() != "clean" {
		t.Fatal("cache clean command not registered")
	}
}

func TestApp_CacheStatCmd_IsRegistered(t *testing.T) {
	cmd, _, err := newApp(t).RootCmd().Find([]string{"cache", "stat"})
	if err != nil || cmd.Name() != "stat" {
		t.Fatal("cache stat command not registered")
	}
}

// TestNewAppWithService_CacheService_DerivedFromServiceCache asserts that
// NewAppWithService automatically derives the cacheService from svc.Cache so
// callers do not need to wire it manually.
func TestNewAppWithService_CacheService_DerivedFromServiceCache(t *testing.T) {
	svc := service.New(nil, &stubLLM{}, service.NoopCache{}, config.DefaultOllamaModel, noopLogger(), service.WithChunkThreshold(0))
	app, err := cli.NewAppWithService(svc)
	if err != nil {
		t.Fatalf("NewAppWithService: %v", err)
	}

	// cache stat should work (returning empty list) without any additional wiring.
	var buf bytes.Buffer
	app.RootCmd().SetOut(&buf)
	app.RootCmd().SetArgs([]string{"cache", "stat"})
	if err := app.Execute(); err != nil {
		t.Fatalf("cache stat with NoopCache: %v", err)
	}
	cmd, _, err := newApp(t).RootCmd().Find([]string{"commit"})
	if err != nil || cmd.Name() != "commit" {
		t.Fatal("commit command not registered")
	}
}

func TestApp_BranchCmd_IsRegistered(t *testing.T) {
	cmd, _, err := newApp(t).RootCmd().Find([]string{"branch"})
	if err != nil || cmd.Name() != "branch" {
		t.Fatal("branch command not registered")
	}
}

func TestApp_PrCmd_IsRegistered(t *testing.T) {
	cmd, _, err := newApp(t).RootCmd().Find([]string{"pr"})
	if err != nil || cmd.Name() != "pr" {
		t.Fatal("pr command not registered")
	}
}

// ---------------------------------------------------------------------------
// commit command
// ---------------------------------------------------------------------------

func TestApp_Execute_Commit_PrintsLLMResponse(t *testing.T) {
	app := newApp(t)
	cli.InjectCommitService(app, &spyCommitService{response: "feat(auth): add token refresh"})

	var buf bytes.Buffer
	app.RootCmd().SetOut(&buf)
	app.RootCmd().SetArgs([]string{"commit"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "feat(auth): add token refresh") {
		t.Errorf("expected LLM response in stdout, got %q", buf.String())
	}
}

func TestApp_Execute_Commit_GitError_ReturnsError(t *testing.T) {
	app := newApp(t)
	cli.InjectCommitService(app, &spyCommitService{err: errors.New("not a git repo")})
	app.RootCmd().SetArgs([]string{"commit"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error when git fails")
	}
}

func TestApp_Execute_Commit_LLMError_ReturnsError(t *testing.T) {
	app := newApp(t)
	cli.InjectCommitService(app, &spyCommitService{err: errors.New("LLM unavailable")})
	app.RootCmd().SetArgs([]string{"commit"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestApp_Execute_Commit_InvalidType_ReturnsError(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"commit", "--type", "invalid"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestApp_Execute_Commit_ShortAndDetailed_MutuallyExclusive(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"commit", "--short", "--detailed"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
}

func TestApp_Execute_Commit_DetailedFlag_PropagatesMode(t *testing.T) {
	spy := &spyCommitService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"commit", "--detailed"})
	cli.InjectCommitService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Mode != domain.Detailed {
		t.Errorf("expected Detailed mode, got %v", spy.receivedOpts.Mode)
	}
}

func TestApp_Execute_Commit_TypeFlag_PropagatesType(t *testing.T) {
	spy := &spyCommitService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"commit", "--type", "refactor"})
	cli.InjectCommitService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Type != domain.Refactor {
		t.Errorf("expected Refactor type, got %v", spy.receivedOpts.Type)
	}
}

func TestApp_Execute_Commit_ExplainFlag_Propagates(t *testing.T) {
	spy := &spyCommitService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"commit", "--explain"})
	cli.InjectCommitService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spy.receivedOpts.Explain {
		t.Error("expected Explain=true, got false")
	}
}

// ---------------------------------------------------------------------------
// branch command
// ---------------------------------------------------------------------------

func TestApp_Execute_Branch_PrintsLLMResponse(t *testing.T) {
	app := newApp(t)
	cli.InjectBranchService(app, &spyBranchService{response: "feat/add-auth"})

	var buf bytes.Buffer
	app.RootCmd().SetOut(&buf)
	app.RootCmd().SetArgs([]string{"branch", "add user authentication"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "feat/add-auth") {
		t.Errorf("expected branch name in stdout, got %q", buf.String())
	}
}

func TestApp_Execute_Branch_MissingArg_ReturnsError(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error for missing task argument")
	}
}

func TestApp_Execute_Branch_LLMError_ReturnsError(t *testing.T) {
	app := newApp(t)
	cli.InjectBranchService(app, &spyBranchService{err: errors.New("llm failed")})
	app.RootCmd().SetArgs([]string{"branch", "some task"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestApp_Execute_Branch_TypeFlag_PropagatesType(t *testing.T) {
	spy := &spyBranchService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch", "--type", "fix", "fix login crash"})
	cli.InjectBranchService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Type != domain.Fix {
		t.Errorf("expected Fix type, got %v", spy.receivedOpts.Type)
	}
}

func TestApp_Execute_Branch_DetailedFlag_PropagatesMode(t *testing.T) {
	spy := &spyBranchService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch", "--detailed", "add auth"})
	cli.InjectBranchService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Mode != domain.Detailed {
		t.Errorf("expected Detailed mode, got %v", spy.receivedOpts.Mode)
	}
}

func TestApp_Execute_Branch_ExplainFlag_Propagates(t *testing.T) {
	spy := &spyBranchService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch", "--explain", "add auth"})
	cli.InjectBranchService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spy.receivedOpts.Explain {
		t.Error("expected Explain=true, got false")
	}
}

func TestApp_Execute_Branch_ShortAndDetailed_MutuallyExclusive(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch", "--short", "--detailed", "add auth"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
}

func TestApp_Execute_Branch_InvalidType_ReturnsError(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch", "--type", "invalid", "add auth"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestApp_Execute_Branch_PassesTaskToService(t *testing.T) {
	spy := &spyBranchService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"branch", "fix login bug"})
	cli.InjectBranchService(app, spy)

	app.Execute()
	if spy.receivedOpts.Task != "fix login bug" {
		t.Errorf("expected task %q, got %q", "fix login bug", spy.receivedOpts.Task)
	}
}

// ---------------------------------------------------------------------------
// pr command
// ---------------------------------------------------------------------------

func TestApp_Execute_Pr_PrintsLLMResponse(t *testing.T) {
	app := newApp(t)
	cli.InjectPrService(app, &spyPrService{response: "feat: add login PR"})

	var buf bytes.Buffer
	app.RootCmd().SetOut(&buf)
	app.RootCmd().SetArgs([]string{"pr", "feature/login", "main"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "feat: add login PR") {
		t.Errorf("expected PR description in stdout, got %q", buf.String())
	}
}

func TestApp_Execute_Pr_MissingBothArgs_ReturnsError(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error for missing branch arguments")
	}
}

func TestApp_Execute_Pr_MissingSecondArg_ReturnsError(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "feature/login"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error for missing second branch argument")
	}
}

func TestApp_Execute_Pr_LLMError_ReturnsError(t *testing.T) {
	app := newApp(t)
	cli.InjectPrService(app, &spyPrService{err: errors.New("llm failed")})
	app.RootCmd().SetArgs([]string{"pr", "feature/x", "main"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestApp_Execute_Pr_ErrEmptyPR_ReturnsError(t *testing.T) {
	app := newApp(t)
	cli.InjectPrService(app, &spyPrService{err: domain.ErrEmptyPR})
	app.RootCmd().SetArgs([]string{"pr", "feature/x", "main"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected ErrEmptyPR to surface as an error")
	}
	if !errors.Is(err, domain.ErrEmptyPR) {
		t.Errorf("expected ErrEmptyPR, got %v", err)
	}
}

func TestApp_Execute_Pr_PassesSourceAndDestination(t *testing.T) {
	// args[0] = source (feature branch), args[1] = destination (base branch)
	spy := &spyPrService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "feature/add-auth", "main"})
	cli.InjectPrService(app, spy)

	app.Execute()
	if spy.receivedOpts.SourceBranch != "feature/add-auth" {
		t.Errorf("expected source %q, got %q", "feature/add-auth", spy.receivedOpts.SourceBranch)
	}
	if spy.receivedOpts.DestinationBranch != "main" {
		t.Errorf("expected destination %q, got %q", "main", spy.receivedOpts.DestinationBranch)
	}
}

func TestApp_Execute_Pr_TypeFlag_PropagatesType(t *testing.T) {
	spy := &spyPrService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "--type", "feat", "feature/x", "main"})
	cli.InjectPrService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Type != domain.Feat {
		t.Errorf("expected Feat type, got %v", spy.receivedOpts.Type)
	}
}

func TestApp_Execute_Pr_DetailedFlag_PropagatesMode(t *testing.T) {
	spy := &spyPrService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "--detailed", "feature/x", "main"})
	cli.InjectPrService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Mode != domain.Detailed {
		t.Errorf("expected Detailed mode, got %v", spy.receivedOpts.Mode)
	}
}

func TestApp_Execute_Pr_ExplainFlag_Propagates(t *testing.T) {
	spy := &spyPrService{}
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "--explain", "feature/x", "main"})
	cli.InjectPrService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spy.receivedOpts.Explain {
		t.Error("expected Explain=true, got false")
	}
}

func TestApp_Execute_Pr_ShortAndDetailed_MutuallyExclusive(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "--short", "--detailed", "feature/x", "main"})
	if err := app.Execute(); err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
}

func TestApp_Execute_Pr_InvalidType_ReturnsError(t *testing.T) {
	app := newApp(t)
	app.RootCmd().SetArgs([]string{"pr", "--type", "invalid", "feature/x", "main"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Two instances do not share state
// ---------------------------------------------------------------------------

func TestApp_TwoInstances_DoNotShareState(t *testing.T) {
	app1 := newApp(t)
	app2 := newApp(t)
	cli.InjectCommitService(app1, &spyCommitService{response: "feat: one"})
	cli.InjectCommitService(app2, &spyCommitService{response: "fix: two"})

	var buf1, buf2 bytes.Buffer
	app1.RootCmd().SetOut(&buf1)
	app2.RootCmd().SetOut(&buf2)
	app1.RootCmd().SetArgs([]string{"commit"})
	app2.RootCmd().SetArgs([]string{"commit"})

	if err := app1.Execute(); err != nil {
		t.Fatalf("app1 error: %v", err)
	}
	if err := app2.Execute(); err != nil {
		t.Fatalf("app2 error: %v", err)
	}
	if !strings.Contains(buf1.String(), "feat: one") {
		t.Errorf("app1: expected %q, got %q", "feat: one", buf1.String())
	}
	if !strings.Contains(buf2.String(), "fix: two") {
		t.Errorf("app2: expected %q, got %q", "fix: two", buf2.String())
	}
}
