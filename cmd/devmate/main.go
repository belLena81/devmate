package main

import (
	"devmate/cli"
	"devmate/internal/infra/llm"
	"devmate/internal/service"
	"log/slog"
	"os"
	"path/filepath"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ollamaClient := llm.NewOllamaClient(llm.WithLogger(log))

	cache := buildCache(log)

	svc := service.New(nil, ollamaClient, cache, ollamaClient.Model(), log)
	app := cli.NewAppWithService(svc)
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildCache(log *slog.Logger) service.Cache {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Error("could not resolve home dir, caching disabled", "error", err)
		return service.NoopCache{}
	}
	dir := filepath.Join(home, ".cache", "devmate")
	log.Debug("cache enabled", "dir", dir)
	return service.NewDiskCache(dir)
}
