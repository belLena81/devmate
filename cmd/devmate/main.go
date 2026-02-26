package main

import (
	"devmate/cli"
	"devmate/internal/infra"
	"os"
)

func main() {
	llm := infra.NewOllamaClient()
	app := cli.NewApp(llm)
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}
