package main

import (
	"devmate/cli"
	"devmate/internal/infra/llm"
	"log/slog"
	"os"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ollamaClient := llm.NewOllamaClient(
		llm.WithLogger(log),
		llm.WithModel("llama3.2:3b"),
		// llm.WithBaseURL("http://localhost:11434"), // override if Ollama runs elsewhere
	)

	app := cli.NewApp(ollamaClient)
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}
