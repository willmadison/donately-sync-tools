package main

import (
	"embed"
	"os"

	"github.com/willmadison/donately-sync-tools/cli"
)

var (
	//go:embed static/inputs/*.csv
	files embed.FS

	//go:embed static/donor-dashboard/dist/*
	ui embed.FS
)

func main() {
	env := cli.Environment{
		Stderr: os.Stderr,
		Stdout: os.Stdout,
		Stdin:  os.Stdin,
		Files:  files,
		UI:     ui,
	}

	os.Exit(cli.Run(env))
}
