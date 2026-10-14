// Package commands provides CLI command implementations for the patchmon-agent
package commands

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"patchmon-agent/internal/config"
	"patchmon-agent/internal/pkgversion"

	"github.com/spf13/cobra"
)

// configCmd represents the config command and subcommands
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  "Manage configuration settings for the PatchMon agent.",
}

// configShowCmd shows current configuration
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  "Display the current configuration settings for the PatchMon agent.",
	RunE: func(_ *cobra.Command, _ []string) error {
		return showConfig()
	},
}

// configSetAPICmd configures API credentials
var configSetAPICmd = &cobra.Command{
	Use:   "set-api <API_ID> <API_KEY> <SERVER_URL>",
	Short: "Configure API credentials for this host",
	Long: `Configure API credentials for the PatchMon server.

Example:
  patchmon-agent config set-api patchmon_1a2b3c4d abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890 http://patchmon.example.com

With --host-id, config.yml is given that host ID. Without it, a host ID is
kept when the API ID is unchanged and cleared otherwise, then learned again
from the server.`,
	Args: cobra.ExactArgs(3),
	RunE: func(_ *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		apiID := args[0]
		apiKey := args[1]
		serverURL := args[2]

		return configureCreds(apiID, apiKey, serverURL, setAPIHostID)
	},
}

var setAPIHostID string

var configSyncHostIDCmd = &cobra.Command{
	Use:   "sync-host-id",
	Short: "Replace the host ID in config.yml with the server's",
	Long: `Ask the PatchMon server which host these credentials belong to and write
that host ID to config.yml, replacing any different value already there.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := checkRoot(); err != nil {
			return err
		}
		return syncHostID()
	},
}

func init() {
	// Add subcommands to config
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetAPICmd)
	configCmd.AddCommand(configSyncHostIDCmd)
	configSetAPICmd.Flags().StringVar(&setAPIHostID, "host-id", "", "host ID to write to config.yml")
}

func syncHostID() error {
	resp, err := pingServer()
	if err != nil {
		return err
	}
	if !config.ValidHostID(resp.HostID) {
		return fmt.Errorf("the server did not return a host ID; it needs PatchMon 2.1.4 or later")
	}
	old := cfgManager.GetHostID()
	if old == resp.HostID {
		fmt.Printf("host_id is already %s\n", old)
		return nil
	}
	if err := cfgManager.SetHostID(resp.HostID); err != nil {
		return fmt.Errorf("failed to save host_id: %w", err)
	}
	if old == "" {
		old = "(none)"
	}
	fmt.Printf("host_id changed from %s to %s\n", old, resp.HostID)
	logger.WithField("old_host_id", old).WithField("host_id", resp.HostID).Info("host_id synchronised with the server")
	return nil
}

func showConfig() error {
	cfg := cfgManager.GetConfig()
	err := cfgManager.LoadCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}
	creds := cfgManager.GetCredentials()

	fmt.Printf("Configuration:\n")
	if cfg.PatchmonServer != "" {
		fmt.Printf("  Server: %s\n", cfg.PatchmonServer)
	} else {
		fmt.Printf("  Server: Not configured\n")
	}
	fmt.Printf("  Agent Version: %s\n", pkgversion.Version)
	fmt.Printf("  Config Version: %s\n", valueOrNone(cfg.ConfigVersion))
	fmt.Printf("  Host ID: %s\n", valueOrNone(cfg.HostID))
	fmt.Printf("  Config File: %s\n", cfgManager.GetConfigFile())
	fmt.Printf("  Credentials File: %s\n", cfg.CredentialsFile)
	fmt.Printf("  Log File: %s\n", cfg.LogFile)
	fmt.Printf("  Log Level: %s\n", cfg.LogLevel)

	updateCheckEnabled, updateCheckInterval := cfgManager.GetDockerUpdateCheck()
	fmt.Printf("\nDocker Update Check:\n")
	fmt.Printf("  Enabled: %t (requires integrations.docker)\n", updateCheckEnabled)
	fmt.Printf("  Interval: %d hours\n", int(updateCheckInterval.Hours()))

	fmt.Printf("\nAlpine secdb:\n")
	fmt.Printf("  Enabled: %t (Alpine hosts only)\n", cfgManager.GetAPKSecDBEnabled())

	fmt.Printf("\nCredentials:\n")
	if creds != nil {
		fmt.Printf("  API ID: %s\n", creds.APIID)
		// Show only first 8 characters of API key for security
		if len(creds.APIKey) >= 0 {
			fmt.Print("  API Key: Set ✅\n")
		} else {
			fmt.Print("  API Key: Not set ❌\n")
		}
	} else {
		fmt.Printf("  Credentials: Not configured\n")
	}

	return nil
}

func valueOrNone(v string) string {
	if v == "" {
		return "(none)"
	}
	return v
}

func configureCreds(apiID, apiKey, serverURL, hostID string) error {
	logger.Info("Setting up credentials...")

	// Validate credentials not empty
	if strings.TrimSpace(apiID) == "" || strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("API ID and API Key must be set")
	}

	// Validate server URL format
	if hostID != "" && !config.ValidHostID(hostID) {
		return fmt.Errorf("invalid host ID %q", hostID)
	}

	if _, err := url.Parse(serverURL); err != nil {
		return fmt.Errorf("invalid server URL format: %w", err)
	}

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return fmt.Errorf("invalid server URL format. Must start with http:// or https://")
	}

	if loadErr := cfgManager.LoadError(); loadErr != nil {
		// The logger writes only to the agent log, and this is the one
		// deliberately destructive path, so the operator gets it on stderr too.
		fmt.Fprintf(os.Stderr, "warning: %s could not be parsed (%v) and is being replaced with defaults; any other settings it held are lost\n",
			cfgManager.GetConfigFile(), loadErr)
		logger.WithError(loadErr).WithField("path", cfgManager.GetConfigFile()).
			Warn("Existing config could not be parsed and is being replaced with defaults, any settings it held are lost")
	}

	previousAPIID := ""
	if cfgManager.LoadCredentials() == nil {
		if creds := cfgManager.GetCredentials(); creds != nil {
			previousAPIID = creds.APIID
		}
	}

	if err := cfgManager.SetPatchmonServer(serverURL); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	switch {
	case hostID != "":
		if err := cfgManager.SetHostID(hostID); err != nil {
			return fmt.Errorf("failed to save host_id: %w", err)
		}
	case previousAPIID != apiID && cfgManager.GetHostID() != "":
		logger.Info("API ID changed, clearing host_id so it is learned again from the server")
		if err := cfgManager.ClearHostID(); err != nil {
			return fmt.Errorf("failed to clear host_id: %w", err)
		}
	}

	// Save credentials
	if err := cfgManager.SaveCredentials(apiID, apiKey); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	logger.Info("Configuration saved successfully")
	logger.WithField("path", cfgManager.GetConfigFile()).Info("Config saved")
	logger.WithField("path", cfgManager.GetConfig().CredentialsFile).Info("Credentials saved")

	// Test credentials
	logger.Info("Testing connection...")
	resp, err := pingServer()
	if err != nil {
		logger.WithError(err).Error("Connection test failed")
		return err
	}
	reconcileHostID(resp.HostID)

	logger.Info("Connectivity test successful")
	logger.Info("API credentials are valid")

	return nil
}
