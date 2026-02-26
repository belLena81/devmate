// cli/app.go
package cli

import (
	"devmate/internal/domain"
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

func NewApp(git domain.GitClient, llm domain.LLM) *App {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	svc := &service.Service{Git: git, LLM: llm, Log: log}
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
