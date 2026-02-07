package commands

import (
	"fmt"
	"net/url"
	"strings"

	"patchmon-agent/internal/version"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		return showConfig()
	},
}

// configSetAPICmd configures API credentials
var configSetAPICmd = &cobra.Command{
	Use:   "set-api <API_ID> <API_KEY> <SERVER_URL>",
	Short: "Configure API credentials for this host",
	Long: `Configure API credentials for the PatchMon server.

Example:
  patchmon-agent config set-api patchmon_1a2b3c4d abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890 http://patchmon.example.com`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		apiID := args[0]
		apiKey := args[1]
		serverURL := args[2]

		return configureCreds(apiID, apiKey, serverURL)
	},
}

func init() {
	// Add subcommands to config
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetAPICmd)
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
	fmt.Printf("  Agent Version: %s\n", version.Version)
	fmt.Printf("  Config File: %s\n", cfgManager.GetConfigFile())
	fmt.Printf("  Credentials File: %s\n", cfg.CredentialsFile)
	fmt.Printf("  Log File: %s\n", cfg.LogFile)
	fmt.Printf("  Log Level: %s\n", cfg.LogLevel)

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

func configureCreds(apiID, apiKey, serverURL string) error {
	logger.Info("Setting up credentials...")

	// Validate credentials not empty
	if strings.TrimSpace(apiID) == "" || strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("API ID and API Key must be set")
	}

	// Validate server URL format
	if _, err := url.Parse(serverURL); err != nil {
		return fmt.Errorf("invalid server URL format: %w", err)
	}

	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return fmt.Errorf("invalid server URL format. Must start with http:// or https://")
	}

	// Set server URL in config
	cfg := cfgManager.GetConfig()
	cfg.PatchmonServer = serverURL

	// Save config
	if err := cfgManager.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Save credentials
	if err := cfgManager.SaveCredentials(apiID, apiKey); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	logger.Info("Configuration saved successfully")
	logger.WithField("path", cfgManager.GetConfigFile()).Info("Config saved")
	logger.WithField("path", cfg.CredentialsFile).Info("Credentials saved")

	// Test credentials
	logger.Info("Testing connection...")
	_, err := pingServer()
	if err != nil {
		logger.WithError(err).Error("Connection test failed")
		return err
	}

	logger.Info("✅ Connectivity test successful")
	logger.Info("✅ API credentials are valid")

	return nil
}
