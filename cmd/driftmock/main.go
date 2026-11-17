package main

import (
	"os"

	"github.com/pandusatrianura/driftmock/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
