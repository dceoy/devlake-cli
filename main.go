package main

import (
	"os"

	"github.com/dceoy/devlake-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
