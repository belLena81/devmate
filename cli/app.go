package cli

import (
	"devmate/internal/domain"
	"devmate/internal/infra/git"
	"devmate/internal/service"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

type App struct {
	rootCmd       *cobra.Command
	commitService CommitService
	branchService BranchService
	prService     PrService
}

func NewApp(llm domain.LLM) *App {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	repoRoot, err := git.RepoRoot()
	if err != nil {
		log.Error("failed to find git repo root", "error", err)
		os.Exit(1)
	}

	runner := git.New(repoRoot, log)
	svc := service.New(runner, llm, log)

	app := &App{
		commitService: svc,
		branchService: svc,
		prService:     svc,
	}
	app.rootCmd = buildRootCmd(app)
	return app
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
