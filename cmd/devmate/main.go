package main

import (
	"devmate/cli"
	"devmate/internal/config"
	"devmate/internal/infra/git"
	"devmate/internal/infra/llm"
	"devmate/internal/infra/progress"
	"devmate/internal/service"
	"log/slog"
	"os"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Config errors are fatal: a bad config file should not silently fall
		// back to defaults, as the user clearly intended a different value.
		slog.New(slog.NewTextHandler(os.Stderr, nil)).
			Error("failed to load config", "error", err)
		os.Exit(1)
	}

	stderr := progress.NewLockedWriter(os.Stderr)

	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{
		Level: cfg.Log.SlogLevel(),
	}))

	ollamaClient := llm.NewOllamaClientFromConfig(llm.ClientConfig{
		BaseURL:        cfg.Ollama.BaseURL,
		Model:          cfg.Ollama.Model,
		RequestTimeout: cfg.Ollama.RequestTimeout(),
	}, llm.WithLogger(log))

	cache := buildCache(cfg.Cache.Dir, log)
	spinner := progress.NewWriter(stderr)

	repoRoot, err := git.RepoRoot()
	if err != nil {
		log.Error("failed to find git repo root", "error", err)
		os.Exit(1)
	}
	gitClient := git.New(repoRoot, log)

	svc := service.New(
		gitClient,
		ollamaClient,
		cache,
		ollamaClient.Model(),
		log,
		service.WithProgress(spinner),
		service.WithChunkThreshold(cfg.Service.ChunkThreshold),
		service.WithMaxConcurrency(cfg.Service.MaxConcurrency),
		service.WithMaxRetries(cfg.Service.MaxRetries),
		service.WithRetryBaseDelay(cfg.Service.RetryBaseDelay()),
	)

	app, err := cli.NewAppWithService(svc)
	if err != nil {
		log.Error("failed to initialise application", "error", err)
		os.Exit(1)
	}

	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildCache(dir string, log *slog.Logger) service.Cache {
	if dir == "" {
		log.Error("could not resolve cache dir, caching disabled")
		return service.NoopCache{}
	}
	log.Debug("cache enabled", "dir", dir)
	return service.NewDiskCache(dir)
}
