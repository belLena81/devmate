// cli/app.go
package cli

import (
	"devmate/internal/domain"
	"devmate/internal/service"

	"github.com/spf13/cobra"
)

type App struct {
	root          *cobra.Command
	commitService CommitService
	// branchService BranchService  (add as you implement)
	// prService     PrService
}

func NewApp(git domain.GitClient, llm domain.LLM) *App {
	svc := &service.Service{Git: git, LLM: llm}
	app := &App{
		commitService: svc,
	}
	app.root = buildRootCmd(app)
	return app
}

func (a *App) Execute() error {
	return a.root.Execute()
}

func (a *App) RootCmd() *cobra.Command {
	return a.root
}

func InjectCommitService(app *App, svc CommitService) {
	app.commitService = svc
}

func buildRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "devmate",
		Short: "Devmate is a read-only developer assistant",
		Long: `devmate is a CLI tool that streamlines common development workflows.

It helps you standardize commits, create branches from JIRA tickets,
and open pull requests with consistent naming conventions.`,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newCommitCmd(app))
	root.AddCommand(newBranchCmd(app))
	root.AddCommand(newPrCmd(app))
	return root
}
