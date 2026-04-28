package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	launchdServiceLabel = "net.patchmon.patchmon-agent"
	launchdPlistPath    = "/Library/LaunchDaemons/net.patchmon.patchmon-agent.plist"
	launchdLogDir       = "/etc/patchmon/logs"
)

var installServiceCmd = &cobra.Command{
	Use:   "install-service",
	Short: "Install patchmon-agent as a launchd service on macOS",
	Long: `Install the patchmon-agent launchd service on macOS.
This command writes a system LaunchDaemon plist to /Library/LaunchDaemons and loads it so the agent
starts automatically on boot and restarts if it exits unexpectedly.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("install-service is only supported on macOS")
		}
		if err := checkRoot(); err != nil {
			return err
		}
		return installLaunchdService()
	},
}

func init() {
	rootCmd.AddCommand(installServiceCmd)
}

func installLaunchdService() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine agent executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlink: %w", err)
	}
	if _, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("agent executable not found at %s: %w", execPath, err)
	}

	if _, err := exec.LookPath("launchctl"); err != nil {
		return fmt.Errorf("launchctl not found: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(launchdPlistPath), 0755); err != nil {
		return fmt.Errorf("failed to create LaunchDaemons directory: %w", err)
	}
	if err := os.MkdirAll(launchdLogDir, 0750); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", launchdLogDir, err)
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/patchmon-agent.out.log</string>
    <key>StandardErrorPath</key>
    <string>%s/patchmon-agent.err.log</string>
    <key>WorkingDirectory</key>
    <string>/</string>
</dict>
</plist>
`, launchdServiceLabel, execPath, launchdLogDir, launchdLogDir)

	if err := os.WriteFile(launchdPlistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("failed to write launchd plist: %w", err)
	}

	if err := os.Chmod(launchdPlistPath, 0644); err != nil {
		return fmt.Errorf("failed to set permissions on launchd plist: %w", err)
	}

	if err := bootstrapLaunchdService(); err != nil {
		return err
	}

	logger.Infof("Installed launchd service %s", launchdServiceLabel)
	return nil
}

func bootstrapLaunchdService() error {
	if exec.Command("launchctl", "print", "system/"+launchdServiceLabel).Run() == nil {
		return kickstartLaunchdService()
	}

	if err := exec.Command("launchctl", "bootstrap", "system", launchdPlistPath).Run(); err != nil {
		if err2 := exec.Command("launchctl", "load", "-w", launchdPlistPath).Run(); err2 != nil {
			return fmt.Errorf("failed to bootstrap or load launchd service: %v, %v", err, err2)
		}
	}

	return kickstartLaunchdService()
}

func kickstartLaunchdService() error {
	if err := exec.Command("launchctl", "kickstart", "-k", "system/"+launchdServiceLabel).Run(); err != nil {
		return fmt.Errorf("failed to kickstart launchd service: %w", err)
	}
	return nil
}
