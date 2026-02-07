package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"patchmon-agent/internal/client"
	"patchmon-agent/pkg/models"
)

// pingCmd represents the ping command
var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Test connectivity and credentials",
	Long:  "Test connectivity to the PatchMon server and validate API credentials.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		_, err := pingServer()
		if err != nil {
			return err
		}

		fmt.Println("✅ API credentials are valid")
		fmt.Println("✅ Connectivity test successful")
		return nil
	},
}

// pingServer tests connectivity to the server and validates credentials
func pingServer() (*models.PingResponse, error) {
	// Load credentials
	if err := cfgManager.LoadCredentials(); err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	// Create client and ping
	httpClient := client.New(cfgManager, logger)
	ctx := context.Background()
	response, err := httpClient.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("connectivity test failed: %w", err)
	}

	return response, nil
}
