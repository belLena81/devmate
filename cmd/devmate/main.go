package main

import (
	"context"
	"devmate/cli"
	"devmate/internal/config"
	"devmate/internal/domain"
	"devmate/internal/infra/git"
	"devmate/internal/infra/llm"
	"devmate/internal/infra/progress"
	"devmate/internal/service"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := cli.NewLazyApp(buildService)
	if err := app.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func buildService(flags cli.GlobalFlags) (*service.Service, error) {
	cfg, err := config.LoadWith(config.Options{Path: flags.ConfigPath, Debug: flags.Verbose})
	if err != nil {
		return nil, err
	}

	level := cfg.Log.SlogLevel()
	switch {
	case flags.Verbose:
		level = slog.LevelDebug
	case flags.Quiet:
		level = slog.LevelError
	}

	stderr := progress.NewLockedWriter(os.Stderr)
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	ollamaClient := llm.NewOllamaClientFromConfig(llm.ClientConfig{
		BaseURL:          cfg.Ollama.BaseURL,
		GeneratePath:     cfg.Ollama.GeneratePath,
		Model:            cfg.Ollama.Model,
		RequestTimeout:   cfg.Ollama.RequestTimeout(),
		MaxResponseBytes: cfg.Ollama.MaxResponseBytes,
	}, llm.WithLogger(log))

	var reporter domain.Progress = progress.NewPlain(stderr)
	if term.IsTerminal(int(os.Stderr.Fd())) {
		reporter = progress.NewWriter(stderr)
	}

	return service.New(
		git.New("", log),
		ollamaClient,
		buildCache(cfg.Cache.Dir, log),
		ollamaClient.Model(),
		log,
		service.WithProgress(reporter),
		service.WithChunkThreshold(cfg.Service.ChunkThreshold),
		service.WithMaxConcurrency(cfg.Service.MaxConcurrency),
		service.WithMaxRetries(cfg.Service.MaxRetries),
		service.WithRetryBaseDelay(cfg.Service.RetryBaseDelay()),
	), nil
}

func buildCache(dir string, log *slog.Logger) service.Cache {
	if dir == "" {
		log.Warn("could not resolve cache dir, caching disabled")
		return service.NoopCache{}
	}
	log.Debug("cache enabled", "dir", dir)
	return service.NewDiskCache(dir)
}
