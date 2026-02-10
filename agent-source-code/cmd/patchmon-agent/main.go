// Package main is the entry point for the patchmon-agent application
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
