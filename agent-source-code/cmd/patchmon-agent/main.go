// Package main is the entry point for the patchmon-agent application
package main

import (
	"os"
	"runtime"
	"runtime/debug"

	"patchmon-agent/cmd/patchmon-agent/commands"
)

func main() {
	// Memory optimization: Set GOGC to 50 for more aggressive garbage collection
	// This reduces memory usage at the cost of slightly more CPU for GC
	debug.SetGCPercent(50)

	// Set soft memory limit to 100MB to prevent excessive memory growth
	// Go will try to keep RSS below this value by triggering GC more frequently
	debug.SetMemoryLimit(100 * 1024 * 1024) // 100 MB

	// Limit max threads to reduce overhead
	runtime.GOMAXPROCS(2)

	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
