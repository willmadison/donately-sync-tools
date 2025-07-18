package main

import (
	"os"

	"github.com/willmadison/donately-sync-tools/cli"
)

func main() {
	env := cli.Environment{
		Stderr: os.Stderr,
		Stdout: os.Stdout,
		Stdin:  os.Stdin,
	}

	os.Exit(cli.Run(env))
}
