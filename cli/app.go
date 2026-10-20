package cli

import (
	"context"
	"devmate/internal/domain"
	"devmate/internal/infra/git"
	"devmate/internal/service"
	"log/slog"

	"github.com/spf13/cobra"
)

// GlobalFlags holds the persistent root flags, available to a ServiceBuilder once parsing is done.
type GlobalFlags struct {
	ConfigPath string
	Verbose    bool
	Quiet      bool
}

// ServiceBuilder constructs the fully wired service for the parsed flags.
type ServiceBuilder func(GlobalFlags) (*service.Service, error)

type App struct {
	rootCmd       *cobra.Command
	flags         GlobalFlags
	build         ServiceBuilder
	commitService CommitService
	branchService BranchService
	prService     PrService
	cacheService  CacheService
}

// NewApp constructs the CLI application wired to llm, with caching disabled.
func NewApp(llm domain.LLM) (*App, error) {
	log := slog.Default()
	svc := service.New(git.New("", log), llm, service.NoopCache{}, "", log)
	return newAppFromService(svc), nil
}

// NewAppWithService constructs the CLI application from a fully wired service without modifying it.
func NewAppWithService(svc *service.Service) (*App, error) {
	return newAppFromService(svc), nil
}

// NewLazyApp defers service construction until a command actually runs, so
// help, version and completion work without a config file or a git repository.
func NewLazyApp(build ServiceBuilder) *App {
	app := &App{build: build}
	app.rootCmd = buildRootCmd(app)
	return app
}

func newAppFromService(svc *service.Service) *App {
	app := &App{}
	app.wire(svc)
	app.rootCmd = buildRootCmd(app)
	return app
}

func (a *App) wire(svc *service.Service) {
	a.commitService = svc
	a.branchService = svc
	a.prService = svc
	a.cacheService = newCacheSvcAdapter(svc.Cache())
}

func (a *App) ensureServices(*cobra.Command, []string) error {
	if a.build == nil || a.commitService != nil {
		return nil
	}
	svc, err := a.build(a.flags)
	if err != nil {
		return err
	}
	a.wire(svc)
	return nil
}

func (a *App) Execute() error {
	return a.rootCmd.Execute()
}

// ExecuteContext runs the CLI with ctx, which is forwarded to every command and cancels in-flight LLM calls.
func (a *App) ExecuteContext(ctx context.Context) error {
	return a.rootCmd.ExecuteContext(ctx)
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
