package main

import (
	"os"

	"patchmon-agent/cmd/patchmon-agent/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
