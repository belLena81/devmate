package cli

import (
	"devmate/internal/domain"
	"devmate/internal/infra/git"
	"devmate/internal/service"
	"log/slog"

	"github.com/spf13/cobra"
)

type App struct {
	rootCmd       *cobra.Command
	commitService CommitService
	branchService BranchService
	prService     PrService
	cacheService  CacheService
}

// NewApp constructs the CLI application wired to llm, with caching disabled.
func NewApp(llm domain.LLM) (*App, error) {
	log := slog.Default()

	repoRoot, err := git.RepoRoot()
	if err != nil {
		log.Error("failed to find git repo root", "error", err)
		return nil, err
	}
	gitClient := git.New(repoRoot, log)

	svc := service.New(gitClient, llm, service.NoopCache{}, "", log)
	return newAppFromService(svc), nil
}

// NewAppWithService constructs the CLI application from a fully wired service without modifying it.
func NewAppWithService(svc *service.Service) (*App, error) {
	return newAppFromService(svc), nil
}

func newAppFromService(svc *service.Service) *App {
	app := &App{
		commitService: svc,
		branchService: svc,
		prService:     svc,
		cacheService:  newCacheSvcAdapter(svc.Cache()),
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

func InjectBranchService(app *App, svc BranchService) {
	app.branchService = svc
}

func InjectPrService(app *App, svc PrService) {
	app.prService = svc
}

func InjectCacheService(app *App, svc CacheService) {
	app.cacheService = svc
}

type cacheSvcAdapter struct {
	cache service.Cache
}

func newCacheSvcAdapter(c service.Cache) *cacheSvcAdapter {
	if c == nil {
		return &cacheSvcAdapter{cache: service.NoopCache{}}
	}
	return &cacheSvcAdapter{cache: c}
}

func (a *cacheSvcAdapter) Clean() error {
	return a.cache.Clear()
}

func (a *cacheSvcAdapter) Stat() ([]service.CacheEntry, error) {
	inspector, ok := a.cache.(service.CacheInspector)
	if !ok {
		return []service.CacheEntry{}, nil
	}
	return inspector.Stat()
}
