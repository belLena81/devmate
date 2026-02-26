package main

import (
	"devmate/cli"
	"devmate/internal/infra"
	"os"
)

func main() {
	git := infra.NewGitClient()
	llm := infra.NewOllamaClient()
	app := cli.NewApp(git, llm)
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}
