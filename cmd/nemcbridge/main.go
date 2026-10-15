package main

import (
	"fmt"
	"os"

	"nemcbridge/internal/app"
)

// Set by the release workflow. Local builds retain the dev defaults.
var version = "dev"
var commit = "unknown"

func main() {
	if err := app.Run(version, commit); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
