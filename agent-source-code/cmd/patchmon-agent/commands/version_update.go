package commands

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"patchmon-agent/internal/config"
	"patchmon-agent/internal/utils"
	"patchmon-agent/internal/version"

	"github.com/spf13/cobra"
)

const (
	serverTimeout       = 30 * time.Second
	versionCheckTimeout = 10 * time.Second // Shorter timeout for version checks
)

type ServerVersionResponse struct {
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	Size         int64  `json:"size"`
	Hash         string `json:"hash"`
	DownloadURL  string `json:"downloadUrl"`
	BinaryData   []byte `json:"-"` // Binary data (not serialized to JSON)
}

type ServerVersionInfo struct {
	CurrentVersion           string   `json:"currentVersion"`
	LatestVersion            string   `json:"latestVersion"`
	HasUpdate                bool     `json:"hasUpdate"`
	AutoUpdateDisabled       bool     `json:"autoUpdateDisabled"`
	AutoUpdateDisabledReason string   `json:"autoUpdateDisabledReason"`
	LastChecked              string   `json:"lastChecked"`
	SupportedArchitectures   []string `json:"supportedArchitectures"`
	Hash                     string   `json:"hash"` // SHA256 hash for integrity verification
}

// checkVersionCmd represents the check-version command
var checkVersionCmd = &cobra.Command{
	Use:   "check-version",
	Short: "Check for agent updates",
	Long:  "Check if there are any updates available for the PatchMon agent.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		return checkVersion()
	},
}

// updateAgentCmd represents the update-agent command
var updateAgentCmd = &cobra.Command{
	Use:   "update-agent",
	Short: "Update agent to latest version",
	Long:  "Download and install the latest version of the PatchMon agent.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		return updateAgent()
	},
}

func checkVersion() error {
	logger.Info("Checking for agent updates...")

	versionInfo, err := getServerVersionInfo()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	currentVersion := strings.TrimPrefix(version.Version, "v")
	latestVersion := strings.TrimPrefix(versionInfo.LatestVersion, "v")

	if versionInfo.HasUpdate {
		logger.Info("Agent update available!")
		fmt.Printf("  Current version: %s\n", currentVersion)
		fmt.Printf("  Latest version: %s\n", latestVersion)
		fmt.Printf("\nTo update, run: patchmon-agent update-agent\n")
	} else if versionInfo.AutoUpdateDisabled && latestVersion != currentVersion {
		logger.WithFields(map[string]interface{}{
			"current": currentVersion,
			"latest":  latestVersion,
			"reason":  versionInfo.AutoUpdateDisabledReason,
		}).Info("New update available but auto-update is disabled")
		fmt.Printf("Current version: %s\n", currentVersion)
		fmt.Printf("Latest version: %s\n", latestVersion)
		fmt.Printf("Status: %s\n", versionInfo.AutoUpdateDisabledReason)
		fmt.Printf("\nTo update manually, run: patchmon-agent update-agent\n")
	} else {
		logger.WithField("version", currentVersion).Info("Agent is up to date")
		fmt.Printf("Agent is up to date (version %s)\n", currentVersion)
	}

	return nil
}

func updateAgent() error {
	logger.Info("Updating agent...")

	// Check if we recently updated to prevent update loops
	// This is especially important on ARM systems where restart might not work properly
	if err := checkRecentUpdate(); err != nil {
		logger.WithError(err).Warn("Recent update detected, skipping to prevent update loop")
		return fmt.Errorf("update skipped: %w", err)
	}

	// Get current executable path
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the actual binary path (important for Alpine and other systems)
	// This ensures we update the actual file, not just a symlink
	resolvedPath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		logger.WithError(err).WithField("path", executablePath).Warn("Could not resolve symlinks, using original path")
		// Use original path if symlink resolution fails
	} else if resolvedPath != executablePath {
		logger.WithField("original", executablePath).WithField("resolved", resolvedPath).Debug("Resolved executable symlink")
		executablePath = resolvedPath
	}

	// Get current version for comparison
	currentVersion := strings.TrimPrefix(version.Version, "v")

	// First, check server version info to see if update is needed
	logger.Debug("Checking server for latest version...")
	versionInfo, err := getServerVersionInfo()
	if err != nil {
		logger.WithError(err).Warn("Failed to get version info, proceeding with update anyway")
	} else {
		latestVersion := strings.TrimPrefix(versionInfo.LatestVersion, "v")
		logger.WithField("current", currentVersion).WithField("latest", latestVersion).Debug("Version check")

		// Check if update is actually needed
		if currentVersion == latestVersion && !versionInfo.HasUpdate {
			logger.WithField("version", currentVersion).Info("Agent is already at the latest version, skipping update")
			return nil
		}
	}

	// Get latest binary info from server
	binaryInfo, err := getLatestBinaryFromServer()
	if err != nil {
		return fmt.Errorf("failed to get latest binary information: %w", err)
	}

	newAgentData := binaryInfo.BinaryData
	if len(newAgentData) == 0 {
		return fmt.Errorf("no binary data received from server")
	}

	// SECURITY: Verify binary integrity against server-provided hash
	// This prevents supply chain attacks where binary could be tampered during download
	// SECURITY: Hash verification is MANDATORY for binary integrity
	if versionInfo == nil || versionInfo.Hash == "" {
		logger.Error("Server did not provide hash for binary verification - refusing to update")
		return fmt.Errorf("binary hash not provided by server - refusing to update without integrity verification (update your PatchMon server)")
	}

	actualHash := fmt.Sprintf("%x", sha256.Sum256(newAgentData))
	if actualHash != versionInfo.Hash {
		logger.WithFields(map[string]interface{}{
			"expected": versionInfo.Hash,
			"actual":   actualHash,
		}).Error("Binary hash verification failed - possible tampering detected")
		return fmt.Errorf("binary hash mismatch: expected %s, got %s", versionInfo.Hash, actualHash)
	}
	logger.WithField("hash", actualHash).Info("Binary integrity verified successfully")

	// Get the new version from server version info (more reliable than parsing binary output)
	newVersion := currentVersion // Default to current if we can't determine
	if versionInfo != nil && versionInfo.LatestVersion != "" {
		newVersion = strings.TrimPrefix(versionInfo.LatestVersion, "v")
	}

	logger.WithField("current", currentVersion).WithField("new", newVersion).Info("Proceeding with update")
	logger.Info("Using downloaded agent binary...")

	// Clean up old backups before creating new one (keep only last 3)
	cleanupOldBackups(executablePath)

	// Create backup of current executable
	backupPath := fmt.Sprintf("%s.backup.%s", executablePath, time.Now().Format("20060102_150405"))
	if err := copyFile(executablePath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	logger.WithField("path", backupPath).Info("Backup saved")

	// Write new version to temporary file (reuse the temp file we already created for version check)
	tempPath := executablePath + ".new"
	if err := os.WriteFile(tempPath, newAgentData, 0755); err != nil {
		return fmt.Errorf("failed to write new agent: %w", err)
	}

	// Verify the new executable works and check its version
	logger.Debug("Validating new executable...")
	testCmd := exec.Command(tempPath, "check-version")
	testCmd.Env = os.Environ() // Preserve environment variables
	if err := testCmd.Run(); err != nil {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			logger.WithError(removeErr).Warn("Failed to remove temporary file after validation failure")
		}
		return fmt.Errorf("new agent executable is invalid: %w", err)
	}
	logger.Debug("New executable validation passed")

	// Verify the downloaded binary version matches expected version
	// This prevents issues where wrong binary might be downloaded
	logger.Debug("Verifying downloaded binary version...")
	versionCmd := exec.Command(tempPath, "version")
	versionCmd.Env = os.Environ()
	versionOutput, err := versionCmd.Output()
	if err == nil {
		// Try to extract version from output (format: "PatchMon Agent v1.3.4" or "1.3.4")
		versionStr := strings.TrimSpace(string(versionOutput))
		// Remove "PatchMon Agent v" prefix if present
		versionStr = strings.TrimPrefix(versionStr, "PatchMon Agent v")
		versionStr = strings.TrimPrefix(versionStr, "v")
		versionStr = strings.TrimSpace(versionStr)

		if versionStr != "" && versionStr != newVersion {
			logger.WithFields(map[string]interface{}{
				"expected": newVersion,
				"actual":   versionStr,
			}).Warn("Downloaded binary version mismatch - this may indicate server issue, but proceeding")
		} else if versionStr == newVersion {
			logger.WithField("version", versionStr).Debug("Downloaded binary version verified")
		}
	} else {
		logger.WithError(err).Debug("Could not verify binary version (non-critical)")
	}

	// Replace current executable atomically
	// On Linux, we can rename over a running executable - the old process keeps using the old inode
	// When the service restarts, it will use the new binary
	logger.Debug("Replacing executable atomically...")
	if err := os.Rename(tempPath, executablePath); err != nil {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			logger.WithError(removeErr).Warn("Failed to remove temporary file after rename failure")
		}
		// Check if it's a filesystem/permission issue
		if os.IsPermission(err) {
			return fmt.Errorf("failed to replace executable (permission denied): %w. Ensure the binary is writable", err)
		}
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	// Ensure the new binary has correct permissions (in case umask affected it)
	if err := os.Chmod(executablePath, 0755); err != nil {
		logger.WithError(err).Warn("Failed to set executable permissions on new binary")
		// Don't fail the update for this, but log it
	}

	logger.WithField("version", newVersion).Info("Agent updated successfully")

	// Mark that we just updated to prevent immediate re-update loops
	markRecentUpdate()

	// Restart the service to pick up the new binary
	// This is critical - the old process is still running the old binary
	logger.Info("Restarting patchmon-agent service to load new binary...")
	if err := restartService(executablePath, newVersion); err != nil {
		logger.WithError(err).Error("Failed to restart service - new binary is in place but old process is still running")
		logger.Warn("Manual service restart required to complete update")
		return fmt.Errorf("update completed but service restart failed: %w", err)
	}

	// After restarting, the old process should exit to allow the new process to start
	// The new process will send a report on startup automatically
	logger.Info("Service restart initiated - exiting to allow new process to start")
	logger.Info("New process will report on startup with version " + newVersion)

	// Exit gracefully - systemd will start the new process with the new binary
	// Note: os.Exit terminates the process, so the return below is unreachable
	os.Exit(0)
	return nil // Unreachable, but satisfies function signature
}

// getServerVersionInfo fetches version information from the PatchMon server
func getServerVersionInfo() (*ServerVersionInfo, error) {
	cfgManager := config.New()
	if err := cfgManager.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgManager.GetConfig()

	// Load credentials for API authentication
	if err := cfgManager.LoadCredentials(); err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}
	credentials := cfgManager.GetCredentials()

	architecture := getArchitecture()
	currentVersion := strings.TrimPrefix(version.Version, "v")
	url := fmt.Sprintf("%s/api/v1/hosts/agent/version?arch=%s&type=go&currentVersion=%s", cfg.PatchmonServer, architecture, currentVersion)

	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", fmt.Sprintf("patchmon-agent/%s", version.Version))
	req.Header.Set("X-API-ID", credentials.APIID)
	req.Header.Set("X-API-KEY", credentials.APIKey)

	// Create HTTP client with proper timeouts (shorter for version checks)
	httpClient := &http.Client{
		Timeout: versionCheckTimeout,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 5 * time.Second,
		},
	}

	// SECURITY: Configure for insecure SSL if needed (NOT RECOMMENDED)
	// Even with hash verification, TLS provides important protections
	if cfg.SkipSSLVerify {
		// SECURITY: Block skip_ssl_verify in production environments
		if utils.IsProductionEnvironment() {
			logger.Error("╔══════════════════════════════════════════════════════════════════╗")
			logger.Error("║  SECURITY ERROR: skip_ssl_verify is BLOCKED in production!       ║")
			logger.Error("║  Set PATCHMON_ENV to 'development' to enable insecure mode.      ║")
			logger.Error("║  This setting cannot be used when PATCHMON_ENV=production        ║")
			logger.Error("╚══════════════════════════════════════════════════════════════════╝")
			return nil, fmt.Errorf("skip_ssl_verify is blocked in production environment")
		}

		logger.Warn("⚠️  TLS verification disabled for version check - NOT RECOMMENDED")
		httpClient.Transport = &http.Transport{
			ResponseHeaderTimeout: 5 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithError(closeErr).Debug("Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var versionInfo ServerVersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return nil, fmt.Errorf("failed to decode version info: %w", err)
	}

	return &versionInfo, nil
}

// getLatestBinaryFromServer fetches the latest binary information from the PatchMon server
func getLatestBinaryFromServer() (*ServerVersionResponse, error) {
	cfgManager := config.New()
	if err := cfgManager.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	cfg := cfgManager.GetConfig()

	// Load credentials for API authentication
	if err := cfgManager.LoadCredentials(); err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}
	credentials := cfgManager.GetCredentials()

	architecture := getArchitecture()
	url := fmt.Sprintf("%s/api/v1/hosts/agent/download?arch=%s", cfg.PatchmonServer, architecture)

	ctx, cancel := context.WithTimeout(context.Background(), serverTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", fmt.Sprintf("patchmon-agent/%s", version.Version))
	req.Header.Set("X-API-ID", credentials.APIID)
	req.Header.Set("X-API-KEY", credentials.APIKey)

	// SECURITY: Configure HTTP client for insecure SSL if needed
	// WARNING: This is dangerous for binary downloads even with hash verification!
	// An attacker could provide both a malicious binary AND a matching hash.
	// TLS ensures we're talking to the legitimate server.
	httpClient := http.DefaultClient
	if cfg.SkipSSLVerify {
		// SECURITY: Block skip_ssl_verify in production environments
		if utils.IsProductionEnvironment() {
			logger.Error("╔══════════════════════════════════════════════════════════════════╗")
			logger.Error("║  SECURITY ERROR: skip_ssl_verify is BLOCKED in production!       ║")
			logger.Error("║  Set PATCHMON_ENV to 'development' to enable insecure mode.      ║")
			logger.Error("║  This setting cannot be used when PATCHMON_ENV=production        ║")
			logger.Error("╚══════════════════════════════════════════════════════════════════╝")
			return nil, fmt.Errorf("skip_ssl_verify is blocked in production environment")
		}

		logger.Error("╔══════════════════════════════════════════════════════════════════╗")
		logger.Error("║  CRITICAL: TLS verification DISABLED for binary download!        ║")
		logger.Error("║  This is a severe security risk - MITM attacks are possible.     ║")
		logger.Error("║  Hash verification provides some protection, but TLS ensures     ║")
		logger.Error("║  you're communicating with the legitimate server.                ║")
		logger.Error("║  Use a valid TLS certificate in production!                      ║")
		logger.Error("╚══════════════════════════════════════════════════════════════════╝")
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithError(closeErr).Debug("Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// SECURITY: Limit binary download size to prevent DoS attacks
	// Max 100MB should be more than enough for the agent binary
	const maxBinarySize = 100 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxBinarySize+1)

	// Read the binary data with size limit
	binaryData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read binary data: %w", err)
	}

	// Check if we hit the size limit (read more than maxBinarySize)
	if int64(len(binaryData)) > maxBinarySize {
		return nil, fmt.Errorf("binary size exceeds maximum allowed (%d MB)", maxBinarySize/(1024*1024))
	}

	// Calculate hash
	hash := fmt.Sprintf("%x", sha256.Sum256(binaryData))

	return &ServerVersionResponse{
		Version:      version.Version, // We'll get the actual version from the server later
		Architecture: architecture,
		Size:         int64(len(binaryData)),
		Hash:         hash,
		DownloadURL:  url,
		BinaryData:   binaryData, // Store the binary data directly
	}, nil
}

// getArchitecture returns the architecture string for the current platform
func getArchitecture() string {
	return runtime.GOARCH
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// SECURITY: Use 0700 for backup files (owner-only access)
	return os.WriteFile(dst, data, 0700)
}

// cleanupOldBackups removes old backup files, keeping only the last 3
func cleanupOldBackups(executablePath string) {
	// Find all backup files
	backupDir := filepath.Dir(executablePath)
	backupBase := filepath.Base(executablePath)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		logger.WithError(err).Debug("Could not read directory to clean up backups")
		return
	}

	var backupFiles []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, backupBase+".backup.") {
			backupFiles = append(backupFiles, filepath.Join(backupDir, name))
		}
	}

	// If we have more than 3 backups, remove the oldest ones
	if len(backupFiles) > 3 {
		// Sort by modification time (oldest first)
		type fileInfo struct {
			path string
			time time.Time
		}
		var filesWithTime []fileInfo
		for _, path := range backupFiles {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			filesWithTime = append(filesWithTime, fileInfo{
				path: path,
				time: info.ModTime(),
			})
		}

		// Sort by time (oldest first)
		for i := 0; i < len(filesWithTime)-1; i++ {
			for j := i + 1; j < len(filesWithTime); j++ {
				if filesWithTime[i].time.After(filesWithTime[j].time) {
					filesWithTime[i], filesWithTime[j] = filesWithTime[j], filesWithTime[i]
				}
			}
		}

		// Remove oldest files (keep last 3)
		toRemove := len(filesWithTime) - 3
		for i := 0; i < toRemove; i++ {
			if err := os.Remove(filesWithTime[i].path); err != nil {
				logger.WithError(err).WithField("path", filesWithTime[i].path).Debug("Failed to remove old backup")
			} else {
				logger.WithField("path", filesWithTime[i].path).Debug("Removed old backup file")
			}
		}
		logger.WithField("removed", toRemove).WithField("kept", 3).Info("Cleaned up old backup files")
	}
}

// checkRecentUpdate checks if we updated recently to prevent update loops
func checkRecentUpdate() error {
	updateMarkerPath := "/etc/patchmon/.last_update_timestamp"

	// Check if marker file exists
	info, err := os.Stat(updateMarkerPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No recent update, allow update
			return nil
		}
		// Other error, allow update (non-critical)
		return nil
	}

	// Check if update was within last 5 minutes
	timeSinceUpdate := time.Since(info.ModTime())
	if timeSinceUpdate < 5*time.Minute {
		return fmt.Errorf("update was performed %v ago, waiting to prevent update loop", timeSinceUpdate)
	}

	// Update was more than 5 minutes ago, allow update
	return nil
}

// markRecentUpdate creates a timestamp file to mark that we just updated
func markRecentUpdate() {
	updateMarkerPath := "/etc/patchmon/.last_update_timestamp"

	// SECURITY: Ensure directory exists with restrictive permissions
	if err := os.MkdirAll("/etc/patchmon", 0700); err != nil {
		logger.WithError(err).Debug("Could not create /etc/patchmon directory (non-critical)")
		return
	}

	// Create or update the timestamp file
	file, err := os.Create(updateMarkerPath)
	if err != nil {
		logger.WithError(err).Debug("Could not create update marker file (non-critical)")
		return
	}
	if err := file.Close(); err != nil {
		logger.WithError(err).Debug("Could not close update marker file (non-critical)")
	}

	// Set permissions
	if err := os.Chmod(updateMarkerPath, 0644); err != nil {
		logger.WithError(err).Debug("Could not set permissions on update marker file (non-critical)")
	}
	logger.Debug("Marked recent update to prevent update loops")
}

// restartService restarts the patchmon-agent service (supports systemd and OpenRC)
func restartService(executablePath, expectedVersion string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Detect init system and use appropriate restart command
	if _, err := exec.LookPath("systemctl"); err == nil {
		// Systemd is available
		// Since we're running inside the service, we can't stop ourselves directly
		// Instead, we'll create a helper script that runs after we exit
		logger.Debug("Detected systemd, scheduling service restart via helper script")

		// SECURITY: Ensure /etc/patchmon directory exists with restrictive permissions
		// Using 0700 to prevent other users from reading/writing to this directory
		if err := os.MkdirAll("/etc/patchmon", 0700); err != nil {
			logger.WithError(err).Warn("Failed to create /etc/patchmon directory, will try anyway")
		}

		// Create a helper script that will restart the service after we exit
		// SECURITY: TOCTOU mitigation measures:
		// 1) Use random suffix to prevent predictable paths
		// 2) Use O_EXCL flag for atomic creation (fail if file exists)
		// 3) 0700 permissions on dir and file (owner-only)
		// 4) Script is deleted immediately after execution
		// 5) Verify no symlink attacks before execution
		helperScript := `#!/bin/sh
# Wait a moment for the current process to exit
sleep 2
# Restart the service using systemctl
systemctl restart patchmon-agent 2>&1 || systemctl start patchmon-agent 2>&1
# Clean up this script
rm -f "$0"
`
		// Generate random suffix to prevent predictable path attacks
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			logger.WithError(err).Warn("Failed to generate random suffix, using fallback")
			randomBytes = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
		}
		helperPath := filepath.Join("/etc/patchmon", fmt.Sprintf("restart-%s.sh", hex.EncodeToString(randomBytes)))

		// SECURITY: Verify the directory is not a symlink (prevent symlink attacks)
		dirInfo, err := os.Lstat("/etc/patchmon")
		if err == nil && dirInfo.Mode()&os.ModeSymlink != 0 {
			logger.Warn("Security: /etc/patchmon is a symlink, refusing to create helper script")
			os.Exit(0) // Fall through to exit approach
		}

		// SECURITY: Use O_EXCL to atomically create file (fail if exists - prevents race conditions)
		file, err := os.OpenFile(helperPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			logger.WithError(err).Warn("Failed to create restart helper script, will exit and rely on systemd auto-restart")
			// Fall through to exit approach
		} else {
			// Write the script content to the file
			if _, err := file.WriteString(helperScript); err != nil {
				logger.WithError(err).Warn("Failed to write restart helper script")
				file.Close()
				os.Remove(helperPath)
				// Fall through to exit approach
			} else {
				file.Close()

				// SECURITY: Verify the file we're about to execute is the one we created
				// Check it's a regular file, not a symlink that was swapped in
				fileInfo, err := os.Lstat(helperPath)
				if err != nil || fileInfo.Mode()&os.ModeSymlink != 0 {
					logger.Warn("Security: helper script may have been tampered with, refusing to execute")
					os.Remove(helperPath)
					os.Exit(0)
				}

				// Execute the helper script in background (detached from current process)
				// SECURITY: Avoid shell interpretation by executing directly with nohup
				cmd := exec.Command("nohup", helperPath)
				cmd.Stdout = nil
				cmd.Stderr = nil
				// Detach from parent process group to ensure script continues after we exit
				cmd.SysProcAttr = &syscall.SysProcAttr{
					Setpgid: true,
					Pgid:    0,
				}
				if err := cmd.Start(); err != nil {
					logger.WithError(err).Warn("Failed to start restart helper script, will exit and rely on systemd auto-restart")
					// Clean up script
					if removeErr := os.Remove(helperPath); removeErr != nil {
						logger.WithError(removeErr).Debug("Failed to remove helper script")
					}
					// Fall through to exit approach
				} else {
					logger.Info("Scheduled service restart via helper script, exiting now...")
					// Give the helper script a moment to start
					time.Sleep(500 * time.Millisecond)
					// Exit gracefully - the helper script will restart the service
					os.Exit(0)
				}
			}
		}

		// Fallback: If helper script approach failed, just exit and let systemd handle it
		// Systemd with Restart=always should restart on exit
		logger.Info("Exiting to allow systemd to restart service with new binary...")
		os.Exit(0)
		// os.Exit never returns, but we need this for code flow
		return nil
	} else if _, err := exec.LookPath("rc-service"); err == nil {
		// OpenRC is available (Alpine Linux)
		// Since we're running inside the service, we can't stop ourselves directly
		// Instead, we'll create a helper script that runs after we exit
		logger.Debug("Detected OpenRC, scheduling service restart via helper script")

		// SECURITY: Ensure /etc/patchmon directory exists with restrictive permissions
		// Using 0700 to prevent other users from reading/writing to this directory
		if err := os.MkdirAll("/etc/patchmon", 0700); err != nil {
			logger.WithError(err).Warn("Failed to create /etc/patchmon directory, will try anyway")
		}

		// Create a helper script that will restart the service after we exit
		// SECURITY: TOCTOU mitigation measures:
		// 1) Use random suffix to prevent predictable paths
		// 2) Use O_EXCL flag for atomic creation (fail if file exists)
		// 3) 0700 permissions on dir and file (owner-only)
		// 4) Script is deleted immediately after execution
		// 5) Verify no symlink attacks before execution
		helperScript := `#!/bin/sh
# Wait a moment for the current process to exit
sleep 2
# Restart the service
rc-service patchmon-agent restart 2>&1 || rc-service patchmon-agent start 2>&1
# Clean up this script
rm -f "$0"
`
		// Generate random suffix to prevent predictable path attacks
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			logger.WithError(err).Warn("Failed to generate random suffix, using fallback")
			randomBytes = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
		}
		helperPath := filepath.Join("/etc/patchmon", fmt.Sprintf("restart-%s.sh", hex.EncodeToString(randomBytes)))

		// SECURITY: Verify the directory is not a symlink (prevent symlink attacks)
		dirInfo, err := os.Lstat("/etc/patchmon")
		if err == nil && dirInfo.Mode()&os.ModeSymlink != 0 {
			logger.Warn("Security: /etc/patchmon is a symlink, refusing to create helper script")
			os.Exit(0) // Fall through to exit approach
		}

		// SECURITY: Use O_EXCL to atomically create file (fail if exists - prevents race conditions)
		file, err := os.OpenFile(helperPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			logger.WithError(err).Warn("Failed to create restart helper script, will exit and rely on OpenRC auto-restart")
			// Fall through to exit approach
		} else {
			// Write the script content to the file
			if _, err := file.WriteString(helperScript); err != nil {
				logger.WithError(err).Warn("Failed to write restart helper script")
				file.Close()
				os.Remove(helperPath)
				// Fall through to exit approach
			} else {
				file.Close()

				// SECURITY: Verify the file we're about to execute is the one we created
				// Check it's a regular file, not a symlink that was swapped in
				fileInfo, err := os.Lstat(helperPath)
				if err != nil || fileInfo.Mode()&os.ModeSymlink != 0 {
					logger.Warn("Security: helper script may have been tampered with, refusing to execute")
					os.Remove(helperPath)
					os.Exit(0)
				}

				// Execute the helper script in background (detached from current process)
				// SECURITY: Avoid shell interpretation by executing directly with nohup
				cmd := exec.Command("nohup", helperPath)
				cmd.Stdout = nil
				cmd.Stderr = nil
				// Detach from parent process group to ensure script continues after we exit
				cmd.SysProcAttr = &syscall.SysProcAttr{
					Setpgid: true,
					Pgid:    0,
				}
				if err := cmd.Start(); err != nil {
					logger.WithError(err).Warn("Failed to start restart helper script, will exit and rely on OpenRC auto-restart")
					// Clean up script
					if removeErr := os.Remove(helperPath); removeErr != nil {
						logger.WithError(removeErr).Debug("Failed to remove helper script")
					}
					// Fall through to exit approach
				} else {
					logger.Info("Scheduled service restart via helper script, exiting now...")
					// Give the helper script a moment to start
					time.Sleep(500 * time.Millisecond)
					// Exit gracefully - the helper script will restart the service
					os.Exit(0)
				}
			}
		}

		// Fallback: If helper script approach failed, just exit and let OpenRC handle it
		// OpenRC with command_background="yes" should restart on exit
		logger.Info("Exiting to allow OpenRC to restart service with new binary...")
		os.Exit(0)
		// os.Exit never returns, but we need this for code flow
		return nil
	} else {
		// Fallback: try to kill and let service manager restart it
		logger.Warn("No known init system detected, attempting to restart via process signal")
		// Try to find and kill the process, service manager should restart it
		killCmd := exec.CommandContext(ctx, "pkill", "-HUP", "patchmon-agent")
		if err := killCmd.Run(); err != nil {
			return fmt.Errorf("failed to restart service: no init system detected and pkill failed: %w", err)
		}
		logger.Info("Sent HUP signal to agent process")
		return nil
	}
}

// Removed update-crontab command (cron is no longer used)
