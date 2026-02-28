package cli

import (
	"devmate/internal/domain"
	"devmate/internal/infra/git"
	"devmate/internal/service"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
)

type App struct {
	rootCmd       *cobra.Command
	commitService CommitService
	branchService BranchService
	prService     PrService
}

// NewApp constructs the CLI application wired to the given LLM.
// The git runner is resolved from the working directory.
// Caching is disabled — use NewAppWithService for full wiring including cache.
func NewApp(llm domain.LLM) (*App, error) {
	svc := service.New(nil, llm, service.NoopCache{}, "", slog.Default())
	return newAppFromService(svc)
}

// NewAppWithService constructs the CLI application from a fully wired service.
// This is the production path used by main.go (includes cache and model name).
func NewAppWithService(svc *service.Service) (*App, error) {
	return newAppFromService(svc)
}

func newAppFromService(svc *service.Service) (*App, error) {
	// Reuse the logger already configured on the service (respects the log
	// level from config). Fall back to slog.Default() only when svc.Log is
	// nil, which happens in tests that build a bare service without a logger.
	log := svc.Log
	if log == nil {
		log = slog.Default()
	}

	repoRoot, err := git.RepoRoot()
	if err != nil {
		log.Error("failed to find git repo root", "error", err)
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}
	svc.Git = git.New(repoRoot, log)

	app := &App{
		commitService: svc,
		branchService: svc,
		prService:     svc,
	}
	app.rootCmd = buildRootCmd(app)
	return app, nil
}

func (a *App) Execute() error {
	return a.rootCmd.Execute()
}

func (a *App) RootCmd() *cobra.Command {
	return a.rootCmd
}

func InjectCommitService(app *App, svc CommitService) {
	app.commitService = svc
}

func InjectBranchService(app *App, svc BranchService) {
	app.branchService = svc
}

func InjectPrService(app *App, svc PrService) {
	app.prService = svc
}
