package main

import (
	"devmate/cli"
	"devmate/internal/config"
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

	svc := service.New(nil, ollamaClient, cache, ollamaClient.Model(), log)
	svc.Progress = spinner
	svc.ChunkThreshold = cfg.Service.ChunkThreshold
	svc.MaxConcurrency = cfg.Service.MaxConcurrency
	svc.MaxRetries = cfg.Service.MaxRetries
	svc.RetryBaseDelay = cfg.Service.RetryBaseDelay()

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
