package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"patchmon-agent/internal/system"
	"patchmon-agent/internal/utils"
	"patchmon-agent/internal/version"

	"github.com/spf13/cobra"
)

// diagnosticsCmd represents the diagnostics command
var diagnosticsCmd = &cobra.Command{
	Use:   "diagnostics",
	Short: "Show detailed system diagnostics",
	Long:  "Display comprehensive diagnostic information about the agent, system, and configuration.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showDiagnostics()
	},
}

func showDiagnostics() error {
	cfg := cfgManager.GetConfig()

	fmt.Printf("PatchMon Agent Diagnostics v%s\n\n", version.Version)

	// System Information
	fmt.Printf("System Information:\n")

	systemDetector := system.New(logger)

	osType, osVersion, err := systemDetector.DetectOS()
	if err != nil {
		fmt.Printf("  OS: %s (detection failed: %v)\n", runtime.GOOS, err)
	} else {
		fmt.Printf("  OS: %s %s\n", osType, osVersion)
	}

	fmt.Printf("  Architecture: %s\n", runtime.GOARCH)

	kernelVersion := systemDetector.GetKernelVersion()
	fmt.Printf("  Kernel: %s\n", kernelVersion)

	if hostname, err := os.Hostname(); err == nil {
		fmt.Printf("  Hostname: %s\n", hostname)
	}

	// Show machine ID
	machineID := systemDetector.GetMachineID()
	fmt.Printf("  Machine ID: %s\n", machineID)

	fmt.Printf("\n")

	// Agent Information
	fmt.Printf("Agent Information:\n")
	fmt.Printf("  Version: %s\n", version.Version)
	fmt.Printf("  Config File: %s\n", cfgManager.GetConfigFile())
	fmt.Printf("  Credentials File: %s\n", cfg.CredentialsFile)
	fmt.Printf("  Log File: %s\n", cfg.LogFile)
	fmt.Printf("  Log Level: %s\n", cfg.LogLevel)
	fmt.Printf("\n")

	// Configuration Status
	fmt.Printf("Configuration Status:\n")
	configFile := cfgManager.GetConfigFile()
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("  ✅ Config file exists\n")
	} else {
		fmt.Printf("  ❌ Config file not found (using defaults)\n")
	}
	if _, err := os.Stat(cfg.CredentialsFile); err == nil {
		fmt.Printf("  ✅ Credentials file exists\n")
	} else {
		fmt.Printf("  ❌ Credentials file not found\n")
	}
	fmt.Printf("\n")

	// Network Connectivity & API Credentials
	fmt.Printf("Network Connectivity & API Credentials:\n")
	fmt.Printf("  Server URL: %s\n", cfg.PatchmonServer)

	// Basic network connectivity test
	serverHost, serverPort := extractUrlHostAndPort(cfg.PatchmonServer)
	if isReachable := utils.TcpPing(serverHost, serverPort); isReachable {
		fmt.Printf("  ✅ Server is reachable\n")
	} else {
		fmt.Printf("  ❌ Server is not reachable\n")
	}

	// API credentials and server connectivity test
	fmt.Printf("  ⏳ API connectivity test in progress...")

	// Temporarily disable logging output during diagnostics
	originalOutput := logger.Out
	logger.SetOutput(io.Discard)
	_, pingErr := pingServer()
	logger.SetOutput(originalOutput)

	// Clear the progress line and show result
	fmt.Printf("\r") // Return to beginning of line
	if pingErr != nil {
		fmt.Printf("  ❌ API connectivity not available: %v\n", pingErr)
	} else {
		fmt.Printf("  ✅ API is reachable and credentials are valid\n")
	}
	fmt.Printf("\n")

	// Recent Logs
	fmt.Printf("Last 10 log entries:\n")
	if logLines := getRecentLogs(cfg.LogFile); len(logLines) > 0 {
		for _, line := range logLines {
			fmt.Printf("  %s\n", line)
		}
	} else {
		fmt.Printf("  No recent logs found or log file does not exist.\n")
	}

	return nil
}

// extractUrlHostAndPort extracts the host and port from a URL string
func extractUrlHostAndPort(url string) (host string, port string) {
	trimmed := strings.TrimPrefix(url, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	hostPort := strings.SplitN(trimmed, "/", 2)[0]
	if strings.Contains(hostPort, ":") {
		parts := strings.SplitN(hostPort, ":", 2)
		host = parts[0]
		port = parts[1]
	} else {
		host = hostPort
		if strings.HasPrefix(url, "https://") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port
}

// getRecentLogs reads the last maxLines lines from the specified log file
func getRecentLogs(logFile string) (lines []string) {
	file, err := os.Open(logFile)
	if err != nil {
		return lines
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.WithError(closeErr).WithField("file", logFile).Debug("Failed to close log file")
		}
	}()

	const maxLines = 10
	const readBlockSize = 4096

	stat, err := file.Stat()
	if err != nil {
		return lines
	}

	var (
		size     = stat.Size()
		buf      []byte
		lineEnds []int
	)

	for offset := size; offset > 0 && len(lineEnds) <= maxLines; {
		readSize := min(offset, int64(readBlockSize))
		offset -= readSize

		tmp := make([]byte, readSize)
		_, err := file.ReadAt(tmp, offset)
		if err != nil {
			break
		}
		buf = append(tmp, buf...)

		// Find newlines in the newly read block
		for i := len(tmp) - 1; i >= 0; i-- {
			if tmp[i] == '\n' {
				lineEnds = append([]int{int(offset) + i + 1}, lineEnds...)
				if len(lineEnds) > maxLines {
					break
				}
			}
		}
	}

	var start int64
	if len(lineEnds) > maxLines {
		start = int64(lineEnds[len(lineEnds)-maxLines-1])
	} else {
		start = 0
	}

	// Seek to the start position
	if _, err := file.Seek(start, 0); err != nil {
		// If seek fails, we can't read from the desired position
		// Log the error but continue - we'll just return empty lines
		logger.WithError(err).WithField("file", logFile).Debug("Failed to seek in log file")
		return lines
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}
