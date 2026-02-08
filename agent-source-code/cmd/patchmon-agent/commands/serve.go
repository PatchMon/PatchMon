package commands

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"patchmon-agent/internal/client"
	"patchmon-agent/internal/config"
	"patchmon-agent/internal/integrations"
	"patchmon-agent/internal/integrations/compliance"
	"patchmon-agent/internal/integrations/docker"
	"patchmon-agent/internal/system"
	"patchmon-agent/internal/utils"
	"patchmon-agent/internal/version"
	"patchmon-agent/pkg/models"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

// serveCmd runs the agent as a long-lived service
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the agent as a service with async updates",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}
		return runService()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runService() error {
	if err := cfgManager.LoadCredentials(); err != nil {
		return err
	}

	httpClient := client.New(cfgManager, logger)
	ctx := context.Background()

	// Get api_id for offset calculation
	apiId := cfgManager.GetCredentials().APIID

	// Load interval from config.yml (with default fallback)
	intervalMinutes := cfgManager.GetConfig().UpdateInterval
	if intervalMinutes <= 0 {
		// Default to 60 if not set or invalid
		intervalMinutes = 60
		logger.WithField("interval", intervalMinutes).Info("Using default interval (not set in config)")
	} else {
		logger.WithField("interval", intervalMinutes).Info("Loaded interval from config.yml")
	}

	// Fetch interval from server and update config if different
	if resp, err := httpClient.GetUpdateInterval(ctx); err == nil && resp.UpdateInterval > 0 {
		if resp.UpdateInterval != intervalMinutes {
			logger.WithFields(map[string]interface{}{
				"config_interval": intervalMinutes,
				"server_interval": resp.UpdateInterval,
			}).Info("Server interval differs from config, updating config.yml")

			if err := cfgManager.SetUpdateInterval(resp.UpdateInterval); err != nil {
				logger.WithError(err).Warn("Failed to save interval to config.yml")
			} else {
				intervalMinutes = resp.UpdateInterval
				logger.WithField("interval", intervalMinutes).Info("Updated interval in config.yml")
			}
		}
	} else if err != nil {
		logger.WithError(err).Warn("Failed to fetch interval from server, using config value")
	}

	// Fetch integration status from server and sync with config.yml
	logger.Info("Syncing integration status from server...")
	if integrationResp, err := httpClient.GetIntegrationStatus(ctx); err == nil && integrationResp.Success {
		configUpdated := false
		for integrationName, serverEnabled := range integrationResp.Integrations {
			configEnabled := cfgManager.IsIntegrationEnabled(integrationName)
			if serverEnabled != configEnabled {
				logger.WithFields(map[string]interface{}{
					"integration":  integrationName,
					"config_value": configEnabled,
					"server_value": serverEnabled,
				}).Info("Integration status differs, updating config.yml")

				if err := cfgManager.SetIntegrationEnabled(integrationName, serverEnabled); err != nil {
					logger.WithError(err).Warn("Failed to save integration status to config.yml")
				} else {
					configUpdated = true
					logger.WithFields(map[string]interface{}{
						"integration": integrationName,
						"enabled":     serverEnabled,
					}).Info("Updated integration status in config.yml")
				}
			}
		}

		if configUpdated {
			// Reload config so in-memory state matches the updated file
			if err := cfgManager.LoadConfig(); err != nil {
				logger.WithError(err).Warn("Failed to reload config after integration update")
			} else {
				logger.Info("Config reloaded, integration settings will be applied")
			}
		} else {
			logger.Debug("Integration status matches config, no update needed")
		}
	} else if err != nil {
		logger.WithError(err).Warn("Failed to fetch integration status from server, using config values")
	}

	// Load or calculate offset based on api_id to stagger reporting times
	var offset time.Duration
	configOffsetSeconds := cfgManager.GetConfig().ReportOffset

	// Calculate what the offset should be based on current api_id and interval
	calculatedOffset := utils.CalculateReportOffset(apiId, intervalMinutes)
	calculatedOffsetSeconds := int(calculatedOffset.Seconds())

	// Use config offset if it exists and matches calculated value, otherwise recalculate and save
	if configOffsetSeconds > 0 && configOffsetSeconds == calculatedOffsetSeconds {
		offset = time.Duration(configOffsetSeconds) * time.Second
		logger.WithFields(map[string]interface{}{
			"api_id":           apiId,
			"interval_minutes": intervalMinutes,
			"offset_seconds":   offset.Seconds(),
		}).Info("Loaded report offset from config.yml")
	} else {
		// Offset not in config or doesn't match, calculate and save it
		offset = calculatedOffset
		if err := cfgManager.SetReportOffset(calculatedOffsetSeconds); err != nil {
			logger.WithError(err).Warn("Failed to save offset to config.yml")
		} else {
			logger.WithFields(map[string]interface{}{
				"api_id":           apiId,
				"interval_minutes": intervalMinutes,
				"offset_seconds":   offset.Seconds(),
			}).Info("Calculated and saved report offset to config.yml")
		}
	}

	// Send startup ping to notify server that agent has started
	logger.Info("🚀 Agent starting up, notifying server...")
	if _, err := httpClient.Ping(ctx); err != nil {
		logger.WithError(err).Warn("startup ping failed, will retry")
	} else {
		logger.Info("✅ Startup notification sent to server")
	}

	// Start websocket loop FIRST so agent appears online immediately
	logger.Info("Establishing WebSocket connection...")
	messages := make(chan wsMsg, 10)
	dockerEvents := make(chan interface{}, 100)
	go wsLoop(messages, dockerEvents)

	// Start integration monitoring (Docker real-time events, etc.)
	startIntegrationMonitoring(ctx, dockerEvents)

	// Report current integration status on startup (wait a moment for WebSocket)
	go func() {
		time.Sleep(2 * time.Second)
		reportIntegrationStatus(ctx)
	}()

	// Run initial report in background so it doesn't block WebSocket
	// Compliance scans can take 5-10 minutes, we don't want agent to appear offline
	go func() {
		logger.Info("Sending initial report on startup (background)...")
		if err := sendReport(false); err != nil {
			logger.WithError(err).Warn("initial report failed")
		} else {
			logger.Info("✅ Initial report sent successfully")
		}
	}()

	// Create ticker with initial interval for package reports
	ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
	defer ticker.Stop()

	// Wait for offset before starting periodic reports
	// This staggers the reporting times across different agents
	offsetTimer := time.NewTimer(offset)
	defer offsetTimer.Stop()

	// Track whether offset period has passed
	offsetPassed := false

	// Track current interval for offset recalculation on updates
	currentInterval := intervalMinutes

	for {
		select {
		case <-offsetTimer.C:
			// Offset period completed, start consuming from ticker normally
			offsetPassed = true
			logger.Debug("Offset period completed, periodic reports will now start")
		case <-ticker.C:
			// Only process ticker events after offset has passed
			if offsetPassed {
				if err := sendReport(false); err != nil {
					logger.WithError(err).Warn("periodic report failed")
				}
			}
		case m := <-messages:
			switch m.kind {
			case "settings_update":
				if m.interval > 0 && m.interval != currentInterval {
					// Save new interval to config.yml
					if err := cfgManager.SetUpdateInterval(m.interval); err != nil {
						logger.WithError(err).Warn("Failed to save interval to config.yml")
					} else {
						logger.WithField("interval", m.interval).Info("Saved new interval to config.yml")
					}

					// Recalculate offset for new interval and save to config.yml
					newOffset := utils.CalculateReportOffset(apiId, m.interval)
					newOffsetSeconds := int(newOffset.Seconds())
					if err := cfgManager.SetReportOffset(newOffsetSeconds); err != nil {
						logger.WithError(err).Warn("Failed to save offset to config.yml")
					}

					logger.WithFields(map[string]interface{}{
						"old_interval":       currentInterval,
						"new_interval":       m.interval,
						"new_offset_seconds": newOffset.Seconds(),
					}).Info("Recalculated and saved offset for new interval")

					// Stop old ticker
					ticker.Stop()

					// Create new ticker with updated interval
					ticker = time.NewTicker(time.Duration(m.interval) * time.Minute)
					currentInterval = m.interval

					// Reset offset timer for new interval
					offsetTimer.Stop()
					offsetTimer = time.NewTimer(newOffset)
					offsetPassed = false // Reset flag for new interval

					logger.WithField("new_interval", m.interval).Info("interval updated, no report sent")
				}
			case "report_now":
				if err := sendReport(false); err != nil {
					logger.WithError(err).Warn("report_now failed")
				}
			case "update_agent":
				if err := updateAgent(); err != nil {
					logger.WithError(err).Warn("update_agent failed")
				}
			case "refresh_integration_status":
				logger.Info("Refreshing integration status on server request...")
				go reportIntegrationStatus(ctx)
			case "docker_inventory_refresh":
				logger.Info("Refreshing Docker inventory on server request...")
				go refreshDockerInventory(ctx)
			case "update_notification":
				logger.WithField("version", m.version).Info("Update notification received from server")
				if m.force {
					logger.Info("Force update requested, updating agent now")
					if err := updateAgent(); err != nil {
						logger.WithError(err).Warn("forced update failed")
					}
				} else {
					logger.Info("Update available, run 'patchmon-agent update-agent' to update")
				}
			case "integration_toggle":
				if err := toggleIntegration(m.integrationName, m.integrationEnabled); err != nil {
					logger.WithError(err).Warn("integration_toggle failed")
				} else {
					logger.WithFields(map[string]interface{}{
						"integration": m.integrationName,
						"enabled":     m.integrationEnabled,
					}).Info("Integration toggled successfully, service will restart")
				}
			case "compliance_scan":
				logger.WithFields(map[string]interface{}{
					"profile_type":       m.profileType,
					"profile_id":         m.profileID,
					"enable_remediation": m.enableRemediation,
				}).Info("Running on-demand compliance scan...")
				go func(msg wsMsg) {
					options := &models.ComplianceScanOptions{
						ProfileID:            msg.profileID,
						EnableRemediation:    msg.enableRemediation,
						FetchRemoteResources: msg.fetchRemoteResources,
					}
					if err := runComplianceScanWithOptions(options); err != nil {
						logger.WithError(err).Warn("compliance_scan failed")
					} else {
						if msg.enableRemediation {
							logger.Info("On-demand compliance scan with remediation completed successfully")
						} else {
							logger.Info("On-demand compliance scan completed successfully")
						}
					}
				}(m)
			case "upgrade_ssg":
				logger.Info("Upgrading SSG content packages...")
				go func() {
					if err := upgradeSSGContent(); err != nil {
						logger.WithError(err).Warn("upgrade_ssg failed")
					} else {
						logger.Info("SSG content packages upgraded successfully")
					}
				}()
			case "remediate_rule":
				logger.WithField("rule_id", m.ruleID).Info("Remediating single rule...")
				go func(ruleID string) {
					if err := remediateSingleRule(ruleID); err != nil {
						logger.WithError(err).WithField("rule_id", ruleID).Warn("remediate_rule failed")
					} else {
						logger.WithField("rule_id", ruleID).Info("Single rule remediation completed")
					}
				}(m.ruleID)
			case "docker_image_scan":
				logger.WithFields(map[string]interface{}{
					"image_name":      m.imageName,
					"container_name":  m.containerName,
					"scan_all_images": m.scanAllImages,
				}).Info("Running Docker image CVE scan...")
				go func(msg wsMsg) {
					if err := runDockerImageScan(msg.imageName, msg.containerName, msg.scanAllImages); err != nil {
						logger.WithError(err).Warn("docker_image_scan failed")
					} else {
						logger.Info("Docker image CVE scan completed successfully")
					}
				}(m)
			case "set_compliance_mode":
				logger.WithField("mode", m.complianceMode).Info("Setting compliance mode...")
				// Convert string mode to ComplianceMode type
				var mode config.ComplianceMode
				switch m.complianceMode {
				case "disabled":
					mode = config.ComplianceDisabled
				case "on-demand":
					mode = config.ComplianceOnDemand
				case "enabled":
					mode = config.ComplianceEnabled
				default:
					logger.WithField("mode", m.complianceMode).Warn("Invalid compliance mode, ignoring")
					continue
				}
				if err := cfgManager.SetComplianceMode(mode); err != nil {
					logger.WithError(err).Warn("Failed to set compliance mode")
				} else {
					logger.WithField("mode", m.complianceMode).Info("Compliance mode updated in config.yml")
				}
			case "set_compliance_on_demand_only":
				// Legacy handler - convert to mode and use new handler
				logger.WithField("on_demand_only", m.complianceOnDemandOnly).Info("Setting compliance on-demand only mode (legacy)...")
				var mode config.ComplianceMode
				if m.complianceOnDemandOnly {
					mode = config.ComplianceOnDemand
				} else {
					mode = config.ComplianceEnabled
				}
				if err := cfgManager.SetComplianceMode(mode); err != nil {
					logger.WithError(err).Warn("Failed to set compliance mode")
				} else {
					logger.WithField("mode", string(mode)).Info("Compliance mode updated in config.yml (from legacy on-demand-only)")
				}
			case "ssh_proxy":
				logger.WithField("session_id", m.sshProxySessionID).Info("Handling SSH proxy connection request")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					go handleSshProxy(m, wsConn)
				}
			case "ssh_proxy_input":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleSshProxyInput(m, wsConn)
				}
			case "ssh_proxy_resize":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleSshProxyResize(m, wsConn)
				}
			case "ssh_proxy_disconnect":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleSshProxyDisconnect(m, wsConn)
				}
			}
		}
	}
}

// upgradeSSGContent upgrades the SCAP Security Guide content packages
func upgradeSSGContent() error {
	// Create compliance integration to access the OpenSCAP scanner
	complianceInteg := compliance.New(logger)
	if err := complianceInteg.UpgradeSSGContent(); err != nil {
		return err
	}

	// Send updated status to backend after successful upgrade
	logger.Info("Sending updated compliance status to backend...")
	httpClient := client.New(cfgManager, logger)
	ctx := context.Background()

	// Get new scanner details
	openscapScanner := compliance.NewOpenSCAPScanner(logger)
	scannerDetails := openscapScanner.GetScannerDetails()

	// Check if Docker integration is enabled for Docker Bench and oscap-docker info
	dockerIntegrationEnabled := cfgManager.IsIntegrationEnabled("docker")
	if dockerIntegrationEnabled {
		dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
		scannerDetails.DockerBenchAvailable = dockerBenchScanner.IsAvailable()

		oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
		scannerDetails.OscapDockerAvailable = oscapDockerScanner.IsAvailable()
	}

	// Send updated status
	if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
		Integration: "compliance",
		Enabled:     cfgManager.IsIntegrationEnabled("compliance"),
		Status:      "ready",
		Message:     "SSG content upgraded successfully",
		ScannerInfo: scannerDetails,
	}); err != nil {
		logger.WithError(err).Warn("Failed to send updated compliance status")
		// Don't fail the upgrade just because status update failed
	} else {
		logger.Info("Updated compliance status sent to backend")
	}

	return nil
}

// remediateSingleRule remediates a single failed compliance rule
func remediateSingleRule(ruleID string) error {
	if ruleID == "" {
		return fmt.Errorf("rule ID is required")
	}

	logger.WithField("rule_id", ruleID).Info("Starting single rule remediation")

	// Create compliance integration to run remediation
	complianceInteg := compliance.New(logger)
	if !complianceInteg.IsAvailable() {
		return fmt.Errorf("compliance scanning not available on this system")
	}

	// Run scan with remediation for just this rule
	// Use level1_server as the default profile - it contains most common rules
	// The --rule flag will filter to just the specified rule
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	options := &models.ComplianceScanOptions{
		ProfileID:         "level1_server", // Use default CIS Level 1 Server profile
		RuleID:            ruleID,          // Filter to this specific rule
		EnableRemediation: true,
	}

	logger.WithFields(map[string]interface{}{
		"profile_id": options.ProfileID,
		"rule_id":    options.RuleID,
	}).Info("Running single rule remediation with oscap")

	_, err := complianceInteg.CollectWithOptions(ctx, options)
	if err != nil {
		return fmt.Errorf("remediation failed: %w", err)
	}

	logger.WithField("rule_id", ruleID).Info("Single rule remediation completed successfully")
	return nil
}

// reportIntegrationStatus reports the current status of all enabled integrations
// This ensures the server knows about integration states and scanner capabilities
// Called on startup and periodically based on server settings
func reportIntegrationStatus(ctx context.Context) {
	logger.Debug("Reporting integration status...")

	// Create HTTP client for API calls
	httpClient := client.New(cfgManager, logger)

	// Report compliance integration status if enabled
	if cfgManager.IsIntegrationEnabled("compliance") {
		// Create scanners to check actual availability
		openscapScanner := compliance.NewOpenSCAPScanner(logger)
		dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
		oscapDockerScanner := compliance.NewOscapDockerScanner(logger)

		// Get scanner details (includes OS info, profiles, etc.)
		scannerDetails := openscapScanner.GetScannerDetails()

		// Build components status map based on ACTUAL availability
		components := make(map[string]string)

		// Check OpenSCAP availability
		if openscapScanner.IsAvailable() {
			components["openscap"] = "ready"
		} else {
			components["openscap"] = "failed"
		}

		// Check Docker integration and related tools
		dockerIntegrationEnabled := cfgManager.IsIntegrationEnabled("docker")
		scannerDetails.DockerBenchAvailable = dockerBenchScanner.IsAvailable()

		if dockerIntegrationEnabled {
			if dockerBenchScanner.IsAvailable() {
				components["docker-bench"] = "ready"
				scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
					ID:          "docker-bench",
					Name:        "Docker Bench for Security",
					Description: "CIS Docker Benchmark security checks",
					Type:        "docker-bench",
				})
			} else {
				components["docker-bench"] = "failed"
			}

			// Check oscap-docker for container image CVE scanning
			scannerDetails.OscapDockerAvailable = oscapDockerScanner.IsAvailable()
			if oscapDockerScanner.IsAvailable() {
				components["oscap-docker"] = "ready"
				scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
					ID:          "docker-image-cve",
					Name:        "Docker Image CVE Scan",
					Description: "Scan Docker images for known CVEs using OpenSCAP",
					Type:        "oscap-docker",
					Category:    "docker",
				})
			} else {
				// Check if we're on Ubuntu/Debian where oscap-docker is not supported
				if _, err := exec.LookPath("apt-get"); err == nil {
					// Ubuntu/Debian - oscap-docker requires 'atomic' package which isn't available
					components["oscap-docker"] = "unavailable"
				} else {
					components["oscap-docker"] = "failed"
				}
			}
		} else {
			// Docker integration not enabled - mark as unavailable (not failed)
			components["docker-bench"] = "unavailable"
			components["oscap-docker"] = "unavailable"
		}

		// Determine overall status based on component statuses
		overallStatus := "ready"
		statusMessage := "Compliance tools ready"
		hasReady := false
		hasFailed := false

		for _, status := range components {
			if status == "ready" {
				hasReady = true
			}
			if status == "failed" {
				hasFailed = true
			}
		}

		if hasFailed && hasReady {
			overallStatus = "partial"
			statusMessage = "Some compliance tools failed to install"
		} else if hasFailed && !hasReady {
			overallStatus = "error"
			statusMessage = "All compliance tools failed to install"
		}

		if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
			Integration: "compliance",
			Enabled:     true,
			Status:      overallStatus,
			Message:     statusMessage,
			Components:  components,
			ScannerInfo: scannerDetails,
		}); err != nil {
			logger.WithError(err).Warn("Failed to report compliance status on startup")
		} else {
			logger.WithField("status", overallStatus).Info("✅ Compliance integration status reported")
		}
	}

	// Report docker integration status if enabled
	if cfgManager.IsIntegrationEnabled("docker") {
		dockerInteg := docker.New(logger)
		if dockerInteg.IsAvailable() {
			if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
				Integration: "docker",
				Enabled:     true,
				Status:      "ready",
				Message:     "Docker monitoring ready",
			}); err != nil {
				logger.WithError(err).Warn("Failed to report docker status on startup")
			} else {
				logger.Info("✅ Docker integration status reported")
			}
		}
	}
}

// refreshDockerInventory collects and sends Docker inventory data on demand
// Called when the server requests a Docker data refresh
func refreshDockerInventory(ctx context.Context) {
	logger.Info("Starting Docker inventory refresh...")

	// Check if Docker integration is enabled
	if !cfgManager.IsIntegrationEnabled("docker") {
		logger.Warn("Docker integration is not enabled, skipping refresh")
		return
	}

	// Create Docker integration
	dockerInteg := docker.New(logger)
	if !dockerInteg.IsAvailable() {
		logger.Warn("Docker is not available on this system")
		return
	}

	// Collect Docker data with timeout
	collectCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	dockerData, err := dockerInteg.Collect(collectCtx)
	if err != nil {
		logger.WithError(err).Warn("Failed to collect Docker data")
		return
	}

	// Get system info for payload
	systemDetector := system.New(logger)
	hostname, _ := systemDetector.GetHostname()
	machineID := systemDetector.GetMachineID()

	// Extract Docker data from integration data
	data, ok := dockerData.Data.(*models.DockerData)
	if !ok {
		logger.Warn("Failed to extract Docker data from integration")
		return
	}

	// Create payload
	payload := &models.DockerPayload{
		DockerData:   *data,
		Hostname:     hostname,
		MachineID:    machineID,
		AgentVersion: version.Version,
	}

	logger.WithFields(map[string]interface{}{
		"containers": len(data.Containers),
		"images":     len(data.Images),
		"volumes":    len(data.Volumes),
		"networks":   len(data.Networks),
	}).Info("Sending Docker inventory to server...")

	// Create HTTP client and send data
	httpClient := client.New(cfgManager, logger)
	sendCtx, sendCancel := context.WithTimeout(ctx, 30*time.Second)
	defer sendCancel()

	response, err := httpClient.SendDockerData(sendCtx, payload)
	if err != nil {
		logger.WithError(err).Warn("Failed to send Docker inventory")
		return
	}

	logger.WithFields(map[string]interface{}{
		"containers": response.ContainersReceived,
		"images":     response.ImagesReceived,
		"volumes":    response.VolumesReceived,
		"networks":   response.NetworksReceived,
	}).Info("Docker inventory refresh completed successfully")
}

// startIntegrationMonitoring starts real-time monitoring for integrations that support it
func startIntegrationMonitoring(ctx context.Context, eventChan chan<- interface{}) {
	// Create integration manager
	integrationMgr := integrations.NewManager(logger)

	// Set enabled checker to respect config.yml settings
	integrationMgr.SetEnabledChecker(func(name string) bool {
		return cfgManager.IsIntegrationEnabled(name)
	})

	// Register integrations
	dockerInteg := docker.New(logger)
	integrationMgr.Register(dockerInteg)

	// Start monitoring for real-time integrations
	realtimeIntegrations := integrationMgr.GetRealtimeIntegrations()
	for _, integration := range realtimeIntegrations {
		logger.WithField("integration", integration.Name()).Info("Starting real-time monitoring")

		// Start monitoring in a goroutine
		go func(integ integrations.RealtimeIntegration) {
			if err := integ.StartMonitoring(ctx, eventChan); err != nil {
				logger.WithError(err).Warn("Failed to start integration monitoring")
			}
		}(integration)
	}
}

type wsMsg struct {
	kind                   string
	interval               int
	version                string
	force                  bool
	integrationName        string
	integrationEnabled     bool
	profileType            string // For compliance_scan: openscap, docker-bench, all
	profileID              string // For compliance_scan: specific XCCDF profile ID
	enableRemediation      bool   // For compliance_scan: enable auto-remediation
	fetchRemoteResources   bool   // For compliance_scan: fetch remote resources
	ruleID                 string // For remediate_rule: specific rule ID to remediate
	imageName              string // For docker_image_scan: Docker image to scan
	containerName          string // For docker_image_scan: Docker container to scan
	scanAllImages          bool   // For docker_image_scan: scan all images on system
	complianceOnDemandOnly bool   // For set_compliance_on_demand_only (legacy)
	complianceMode         string // For set_compliance_mode: "disabled", "on-demand", or "enabled"
	// SSH proxy fields
	sshProxySessionID string // Unique session ID for SSH proxy
	sshProxyHost      string // SSH target host
	sshProxyPort      int    // SSH target port
	sshProxyUsername  string // SSH username
	sshProxyPassword  string // SSH password
	sshProxyPrivateKey string // SSH private key
	sshProxyPassphrase string // SSH private key passphrase
	sshProxyTerminal   string // Terminal type
	sshProxyCols       int    // Terminal columns
	sshProxyRows       int    // Terminal rows
	sshProxyData       string // SSH input data
}

// Input validation patterns for WebSocket message fields
// These prevent command injection by ensuring only safe characters are allowed
var (
	// Profile IDs: alphanumeric, underscores, dots, hyphens (e.g., xccdf_org.ssgproject.content_profile_level1_server)
	validProfileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)
	// Rule IDs: same as profile IDs (e.g., xccdf_org.ssgproject.content_rule_audit_rules_...)
	validRuleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)
	// Docker image names: alphanumeric, slashes, colons, dots, hyphens, underscores (e.g., ubuntu:22.04, myregistry.io/app:v1)
	validDockerImagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-/:@]*$`)
	// Docker container names: alphanumeric, underscores, hyphens (e.g., my-container, container_1)
	validDockerContainerPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\-]*$`)
)

// validateProfileID validates a compliance profile ID to prevent command injection
func validateProfileID(profileID string) error {
	if profileID == "" {
		return nil // Empty is allowed - will use default
	}
	if len(profileID) > 256 {
		return fmt.Errorf("profile ID too long (max 256 chars)")
	}
	if !validProfileIDPattern.MatchString(profileID) {
		return fmt.Errorf("invalid profile ID: contains disallowed characters")
	}
	return nil
}

// validateRuleID validates a compliance rule ID to prevent command injection
func validateRuleID(ruleID string) error {
	if ruleID == "" {
		return fmt.Errorf("rule ID is required")
	}
	if len(ruleID) > 256 {
		return fmt.Errorf("rule ID too long (max 256 chars)")
	}
	if !validRuleIDPattern.MatchString(ruleID) {
		return fmt.Errorf("invalid rule ID: contains disallowed characters")
	}
	return nil
}

// validateDockerImageName validates a Docker image name to prevent command injection
func validateDockerImageName(imageName string) error {
	if imageName == "" {
		return nil // Empty is allowed when scanning all images
	}
	if len(imageName) > 512 {
		return fmt.Errorf("image name too long (max 512 chars)")
	}
	if !validDockerImagePattern.MatchString(imageName) {
		return fmt.Errorf("invalid Docker image name: contains disallowed characters")
	}
	return nil
}

// validateDockerContainerName validates a Docker container name to prevent command injection
func validateDockerContainerName(containerName string) error {
	if containerName == "" {
		return nil // Empty is allowed when scanning images
	}
	if len(containerName) > 256 {
		return fmt.Errorf("container name too long (max 256 chars)")
	}
	if !validDockerContainerPattern.MatchString(containerName) {
		return fmt.Errorf("invalid Docker container name: contains disallowed characters")
	}
	return nil
}

// ComplianceScanProgress represents a progress update during compliance scanning
type ComplianceScanProgress struct {
	Phase       string  `json:"phase"`        // started, evaluating, parsing, completed, failed
	ProfileName string  `json:"profile_name"` // Name of the profile being scanned
	Message     string  `json:"message"`      // Human-readable progress message
	Progress    float64 `json:"progress"`     // 0-100 percentage (approximate)
	Error       string  `json:"error,omitempty"`
}

// Global channel for compliance scan progress updates
var complianceProgressChan = make(chan ComplianceScanProgress, 10)

// Global WebSocket connection for SSH proxy (set in connectOnce)
var globalWsConn *websocket.Conn
var globalWsConnMu sync.RWMutex

func wsLoop(out chan<- wsMsg, dockerEvents <-chan interface{}) {
	backoff := time.Second
	for {
		if err := connectOnce(out, dockerEvents); err != nil {
			logger.WithError(err).Warn("ws disconnected; retrying")
		}
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func connectOnce(out chan<- wsMsg, dockerEvents <-chan interface{}) error {
	server := cfgManager.GetConfig().PatchmonServer
	if server == "" {
		return nil
	}
	apiID := cfgManager.GetCredentials().APIID
	apiKey := cfgManager.GetCredentials().APIKey

	// Convert http(s) -> ws(s)
	wsURL := server
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(wsURL, "https://")
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	} else if strings.HasPrefix(wsURL, "wss://") {
		// Already a WebSocket secure URL, use as-is
		// No conversion needed
	} else if strings.HasPrefix(wsURL, "ws://") {
		// Already a WebSocket URL, use as-is
		// No conversion needed
	} else {
		// No protocol prefix - assume HTTPS and use WSS
		logger.WithField("server", server).Warn("Server URL missing protocol prefix, assuming HTTPS")
		wsURL = "wss://" + wsURL
	}
	if strings.HasSuffix(wsURL, "/") {
		wsURL = strings.TrimRight(wsURL, "/")
	}
	wsURL = wsURL + "/api/" + cfgManager.GetConfig().APIVersion + "/agents/ws"
	header := http.Header{}
	header.Set("X-API-ID", apiID)
	header.Set("X-API-KEY", apiKey)

	// SECURITY: Configure WebSocket dialer for insecure connections if needed
	// WARNING: This exposes the agent to man-in-the-middle attacks!
	dialer := websocket.DefaultDialer
	if cfgManager.GetConfig().SkipSSLVerify {
		// SECURITY: Block skip_ssl_verify in production environments
		if utils.IsProductionEnvironment() {
			logger.Error("╔══════════════════════════════════════════════════════════════════╗")
			logger.Error("║  SECURITY ERROR: skip_ssl_verify is BLOCKED in production!       ║")
			logger.Error("║  Set PATCHMON_ENV to 'development' to enable insecure mode.      ║")
			logger.Error("║  This setting cannot be used when PATCHMON_ENV=production        ║")
			logger.Error("╚══════════════════════════════════════════════════════════════════╝")
			logger.Fatal("Refusing to start with skip_ssl_verify=true in production environment")
		}

		logger.Error("╔══════════════════════════════════════════════════════════════════╗")
		logger.Error("║  SECURITY WARNING: TLS verification DISABLED for WebSocket!      ║")
		logger.Error("║  Commands from server could be intercepted or modified.          ║")
		logger.Error("║  Use a valid TLS certificate in production!                      ║")
		logger.Error("╚══════════════════════════════════════════════════════════════════╝")
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return err
	}

	// Create a done channel to signal goroutines to stop when connection closes
	done := make(chan struct{})
	defer func() {
		close(done) // Signal all goroutines to stop
		if err := conn.Close(); err != nil {
			logger.WithError(err).Warn("Failed to close WebSocket connection")
		}
	}()

	// ping loop - now with cancellation support
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return // Connection closed, exit goroutine
				}
			}
		}
	}()

	// Set read deadlines and extend them on pong frames to avoid idle timeouts
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})

	// SECURITY: Limit WebSocket message size to prevent DoS attacks (64KB max)
	conn.SetReadLimit(64 * 1024)

	logger.WithField("url", wsURL).Info("WebSocket connected")

	// Store connection globally for SSH proxy handlers
	globalWsConnMu.Lock()
	globalWsConn = conn
	globalWsConnMu.Unlock()
	defer func() {
		globalWsConnMu.Lock()
		globalWsConn = nil
		globalWsConnMu.Unlock()
	}()

	// Create a goroutine to send Docker events through WebSocket - with cancellation support
	go func() {
		for {
			select {
			case <-done:
				return
			case event, ok := <-dockerEvents:
				if !ok {
					return // Channel closed
				}
				if dockerEvent, ok := event.(models.DockerStatusEvent); ok {
					eventJSON, err := json.Marshal(map[string]interface{}{
						"type":         "docker_status",
						"event":        dockerEvent,
						"container_id": dockerEvent.ContainerID,
						"name":         dockerEvent.Name,
						"status":       dockerEvent.Status,
						"timestamp":    dockerEvent.Timestamp,
					})
					if err != nil {
						logger.WithError(err).Warn("Failed to marshal Docker event")
						continue
					}

					if err := conn.WriteMessage(websocket.TextMessage, eventJSON); err != nil {
						logger.WithError(err).Debug("Failed to send Docker event via WebSocket")
						return
					}
				}
			}
		}
	}()

	// Create a goroutine to send compliance scan progress updates through WebSocket
	go func() {
		for {
			select {
			case <-done:
				return
			case progress, ok := <-complianceProgressChan:
				if !ok {
					return // Channel closed
				}
				progressJSON, err := json.Marshal(map[string]interface{}{
					"type":         "compliance_scan_progress",
					"phase":        progress.Phase,
					"profile_name": progress.ProfileName,
					"message":      progress.Message,
					"progress":     progress.Progress,
					"error":        progress.Error,
					"timestamp":    time.Now().Format(time.RFC3339),
				})
				if err != nil {
					logger.WithError(err).Warn("Failed to marshal compliance progress event")
					continue
				}

				if err := conn.WriteMessage(websocket.TextMessage, progressJSON); err != nil {
					logger.WithError(err).Debug("Failed to send compliance progress via WebSocket")
					return
				}
				logger.WithFields(map[string]interface{}{
					"phase":   progress.Phase,
					"message": progress.Message,
				}).Debug("Sent compliance progress update via WebSocket")
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		logger.WithField("raw_message", string(data)).Debug("WebSocket message received")
		var payload struct {
			Type                 string `json:"type"`
			UpdateInterval       int    `json:"update_interval"`
			Version              string `json:"version"`
			Force                bool   `json:"force"`
			Message              string `json:"message"`
			Integration          string `json:"integration"`
			Enabled              bool   `json:"enabled"`
			ProfileType          string `json:"profile_type"`           // For compliance_scan
			ProfileID            string `json:"profile_id"`             // For compliance_scan: specific XCCDF profile ID
			EnableRemediation    bool   `json:"enable_remediation"`     // For compliance_scan
			FetchRemoteResources bool   `json:"fetch_remote_resources"` // For compliance_scan
			RuleID               string `json:"rule_id"`                // For remediate_rule: specific rule to remediate
			ImageName            string `json:"image_name"`             // For docker_image_scan: Docker image to scan
			ContainerName        string `json:"container_name"`         // For docker_image_scan: container to scan
			ScanAllImages        bool   `json:"scan_all_images"`        // For docker_image_scan: scan all images
			OnDemandOnly         bool   `json:"on_demand_only"`         // For set_compliance_on_demand_only (legacy)
			Mode                 string `json:"mode"`                   // For set_compliance_mode: "disabled", "on-demand", or "enabled"
			// SSH proxy fields
			SessionID   string `json:"session_id"`   // SSH proxy session ID
			Host        string `json:"host"`         // SSH proxy target host
			Port        int    `json:"port"`         // SSH proxy target port
			Username    string `json:"username"`     // SSH username
			Password    string `json:"password"`     // SSH password
			PrivateKey  string `json:"private_key"`  // SSH private key
			Passphrase  string `json:"passphrase"`   // SSH private key passphrase
			Terminal    string `json:"terminal"`     // Terminal type
			Cols        int    `json:"cols"`          // Terminal columns
			Rows        int    `json:"rows"`          // Terminal rows
			Data        string `json:"data"`          // SSH input data
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			logger.WithError(err).WithField("data", string(data)).Warn("Failed to parse WebSocket message")
			continue
		}
		logger.WithField("type", payload.Type).Debug("Parsed WebSocket message type")
		switch payload.Type {
		case "settings_update":
			logger.WithField("interval", payload.UpdateInterval).Info("settings_update received")
			out <- wsMsg{kind: "settings_update", interval: payload.UpdateInterval}
		case "report_now":
			logger.Info("report_now received")
			out <- wsMsg{kind: "report_now"}
		case "update_agent":
			logger.Info("update_agent received")
			out <- wsMsg{kind: "update_agent"}
		case "refresh_integration_status":
			logger.Info("refresh_integration_status received")
			out <- wsMsg{kind: "refresh_integration_status"}
		case "docker_inventory_refresh":
			logger.Info("docker_inventory_refresh received")
			out <- wsMsg{kind: "docker_inventory_refresh"}
		case "update_notification":
			logger.WithFields(map[string]interface{}{
				"version": payload.Version,
				"force":   payload.Force,
				"message": payload.Message,
			}).Info("update_notification received")
			out <- wsMsg{
				kind:    "update_notification",
				version: payload.Version,
				force:   payload.Force,
			}
		case "integration_toggle":
			logger.WithFields(map[string]interface{}{
				"integration": payload.Integration,
				"enabled":     payload.Enabled,
			}).Info("integration_toggle received")
			out <- wsMsg{
				kind:               "integration_toggle",
				integrationName:    payload.Integration,
				integrationEnabled: payload.Enabled,
			}
		case "compliance_scan":
			// Validate profile ID to prevent command injection
			if err := validateProfileID(payload.ProfileID); err != nil {
				logger.WithError(err).WithField("profile_id", payload.ProfileID).Warn("Invalid profile ID in compliance_scan message")
				continue
			}
			profileType := payload.ProfileType
			if profileType == "" {
				profileType = "all"
			}
			logger.WithFields(map[string]interface{}{
				"profile_type":       profileType,
				"profile_id":         payload.ProfileID,
				"enable_remediation": payload.EnableRemediation,
			}).Info("compliance_scan received")
			out <- wsMsg{
				kind:                 "compliance_scan",
				profileType:          profileType,
				profileID:            payload.ProfileID,
				enableRemediation:    payload.EnableRemediation,
				fetchRemoteResources: payload.FetchRemoteResources,
			}
		case "upgrade_ssg":
			logger.Info("upgrade_ssg received from WebSocket")
			out <- wsMsg{kind: "upgrade_ssg"}
			logger.Info("upgrade_ssg sent to message channel")
		case "remediate_rule":
			// Validate rule ID to prevent command injection
			if err := validateRuleID(payload.RuleID); err != nil {
				logger.WithError(err).WithField("rule_id", payload.RuleID).Warn("Invalid rule ID in remediate_rule message")
				continue
			}
			logger.WithField("rule_id", payload.RuleID).Info("remediate_rule received")
			out <- wsMsg{kind: "remediate_rule", ruleID: payload.RuleID}
		case "docker_image_scan":
			// Validate Docker image and container names to prevent command injection
			if err := validateDockerImageName(payload.ImageName); err != nil {
				logger.WithError(err).WithField("image_name", payload.ImageName).Warn("Invalid image name in docker_image_scan message")
				continue
			}
			if err := validateDockerContainerName(payload.ContainerName); err != nil {
				logger.WithError(err).WithField("container_name", payload.ContainerName).Warn("Invalid container name in docker_image_scan message")
				continue
			}
			logger.WithFields(map[string]interface{}{
				"image_name":      payload.ImageName,
				"container_name":  payload.ContainerName,
				"scan_all_images": payload.ScanAllImages,
			}).Info("docker_image_scan received")
			out <- wsMsg{
				kind:          "docker_image_scan",
				imageName:     payload.ImageName,
				containerName: payload.ContainerName,
				scanAllImages: payload.ScanAllImages,
			}
		case "set_compliance_mode":
			logger.WithField("mode", payload.Mode).Info("set_compliance_mode received")
			// Validate mode
			validModes := map[string]bool{"disabled": true, "on-demand": true, "enabled": true}
			if !validModes[payload.Mode] {
				logger.WithField("mode", payload.Mode).Warn("Invalid compliance mode, ignoring")
				continue
			}
			out <- wsMsg{
				kind:           "set_compliance_mode",
				complianceMode: payload.Mode,
			}
		case "set_compliance_on_demand_only":
			// Legacy handler - convert to new format
			logger.WithField("on_demand_only", payload.OnDemandOnly).Info("set_compliance_on_demand_only received (legacy)")
			mode := "enabled"
			if payload.OnDemandOnly {
				mode = "on-demand"
			}
			out <- wsMsg{
				kind:           "set_compliance_mode",
				complianceMode: mode,
			}
		case "ssh_proxy":
			// Validate SSH proxy is enabled in config
			if !cfgManager.IsIntegrationEnabled("ssh-proxy-enabled") {
				logger.Warn("SSH proxy requested but not enabled in config.yml")
				// Send error back to backend
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					errorMsg := "SSH proxy is not enabled.\n\n" +
						"To enable SSH proxy, edit the file /etc/patchmon/config.yml and add the following:\n\n" +
						"integrations:\n" +
						"    ssh-proxy-enabled: true\n\n" +
						"Note: This cannot be pushed from the server to the agent and should require you to manually do this for security reasons."
					sendSshProxyError(wsConn, payload.SessionID, errorMsg)
				}
				continue
			}
			// Validate session ID
			if payload.SessionID == "" {
				logger.Warn("SSH proxy request missing session_id")
				continue
			}
			// Validate host
			if err := validateSshProxyHost(payload.Host); err != nil {
				logger.WithError(err).WithField("host", payload.Host).Warn("Invalid SSH proxy host")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					sendSshProxyError(wsConn, payload.SessionID, fmt.Sprintf("Invalid host: %v", err))
				}
				continue
			}
			// Validate port
			if payload.Port < 1 || payload.Port > 65535 {
				logger.WithField("port", payload.Port).Warn("Invalid SSH proxy port")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					sendSshProxyError(wsConn, payload.SessionID, "Invalid port (must be 1-65535)")
				}
				continue
			}
			logger.WithFields(map[string]interface{}{
				"session_id": payload.SessionID,
				"host":       payload.Host,
				"port":       payload.Port,
				"username":   payload.Username,
			}).Info("ssh_proxy received")
			out <- wsMsg{
				kind:                "ssh_proxy",
				sshProxySessionID:    payload.SessionID,
				sshProxyHost:         payload.Host,
				sshProxyPort:         payload.Port,
				sshProxyUsername:     payload.Username,
				sshProxyPassword:     payload.Password,
				sshProxyPrivateKey:   payload.PrivateKey,
				sshProxyPassphrase:   payload.Passphrase,
				sshProxyTerminal:     payload.Terminal,
				sshProxyCols:         payload.Cols,
				sshProxyRows:         payload.Rows,
			}
		case "ssh_proxy_input":
			if payload.SessionID == "" {
				logger.Warn("ssh_proxy_input missing session_id")
				continue
			}
			out <- wsMsg{
				kind:             "ssh_proxy_input",
				sshProxySessionID: payload.SessionID,
				sshProxyData:      payload.Data,
			}
		case "ssh_proxy_resize":
			if payload.SessionID == "" {
				logger.Warn("ssh_proxy_resize missing session_id")
				continue
			}
			out <- wsMsg{
				kind:             "ssh_proxy_resize",
				sshProxySessionID: payload.SessionID,
				sshProxyCols:      payload.Cols,
				sshProxyRows:      payload.Rows,
			}
		case "ssh_proxy_disconnect":
			if payload.SessionID == "" {
				logger.Warn("ssh_proxy_disconnect missing session_id")
				continue
			}
			out <- wsMsg{
				kind:             "ssh_proxy_disconnect",
				sshProxySessionID: payload.SessionID,
			}
		default:
			if payload.Type != "" && payload.Type != "connected" {
				logger.WithField("type", payload.Type).Warn("Unknown WebSocket message type")
			}
		}
	}
}

// toggleIntegration toggles an integration on or off and restarts the service
func toggleIntegration(integrationName string, enabled bool) error {
	logger.WithFields(map[string]interface{}{
		"integration": integrationName,
		"enabled":     enabled,
	}).Info("Toggling integration")

	// Handle compliance tools installation/removal
	if integrationName == "compliance" {
		// Create HTTP client for sending status updates
		httpClient := client.New(cfgManager, logger)
		ctx := context.Background()

		components := make(map[string]string)
		var overallStatus string
		var statusMessage string

		if enabled {
			logger.Info("Compliance enabled - installing required tools...")
			overallStatus = "installing"

		// Send initial "installing" status
		if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
			Integration: "compliance",
			Enabled:     true,
			Status:      "installing",
			Message:     "Installing compliance tools...",
		}); err != nil {
			logger.WithError(err).Warn("Failed to send initial compliance installation status")
		}

			// Install OpenSCAP
			openscapScanner := compliance.NewOpenSCAPScanner(logger)
			if err := openscapScanner.EnsureInstalled(); err != nil {
				logger.WithError(err).Warn("Failed to install OpenSCAP (will try again on next scan)")
				components["openscap"] = "failed"
			} else {
				logger.Info("OpenSCAP installed successfully")
				components["openscap"] = "ready"
			}

			// Pre-pull Docker Bench image only if Docker integration is enabled AND Docker is available
			dockerIntegrationEnabled := cfgManager.IsIntegrationEnabled("docker")
			if dockerIntegrationEnabled {
				dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
				if dockerBenchScanner.IsAvailable() {
					if err := dockerBenchScanner.EnsureInstalled(); err != nil {
						logger.WithError(err).Warn("Failed to pre-pull Docker Bench image (will pull on first scan)")
						components["docker-bench"] = "failed"
					} else {
						logger.Info("Docker Bench image pulled successfully")
						components["docker-bench"] = "ready"
					}
				} else {
					components["docker-bench"] = "unavailable"
				}

				// Install oscap-docker for container image CVE scanning
				oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
				if !oscapDockerScanner.IsAvailable() {
					if err := oscapDockerScanner.EnsureInstalled(); err != nil {
						// Check if it's a platform limitation (not available on this OS) vs installation failure
						errMsg := err.Error()
						if strings.Contains(errMsg, "not available") || strings.Contains(errMsg, "not supported") {
							logger.WithError(err).Info("oscap-docker not available on this platform")
							components["oscap-docker"] = "unavailable"
						} else {
							logger.WithError(err).Warn("Failed to install oscap-docker (container CVE scanning won't be available)")
							components["oscap-docker"] = "failed"
						}
					} else {
						logger.Info("oscap-docker installed successfully")
						components["oscap-docker"] = "ready"
					}
				} else {
					logger.Info("oscap-docker already available")
					components["oscap-docker"] = "ready"
				}
			} else {
				logger.Debug("Docker integration not enabled, skipping Docker Bench and oscap-docker setup")
				// Don't add docker-bench to components at all if integration is not enabled
			}

			// Determine overall status
			allReady := true
			for _, status := range components {
				if status == "failed" {
					allReady = false
					break
				}
			}
			if allReady {
				overallStatus = "ready"
				statusMessage = "Compliance tools installed and ready"
			} else {
				overallStatus = "partial"
				statusMessage = "Some compliance tools failed to install"
			}

			// Get detailed scanner info to send with status
			scannerDetails := openscapScanner.GetScannerDetails()

			// Add Docker Bench and oscap-docker info if available
			if dockerIntegrationEnabled {
				dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
				scannerDetails.DockerBenchAvailable = dockerBenchScanner.IsAvailable()
				if scannerDetails.DockerBenchAvailable {
					scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
						ID:          "docker-bench",
						Name:        "Docker Bench for Security",
						Description: "CIS Docker Benchmark security checks",
						Type:        "docker-bench",
					})
				}

				// Add oscap-docker info for container image CVE scanning
				oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
				scannerDetails.OscapDockerAvailable = oscapDockerScanner.IsAvailable()
				if oscapDockerScanner.IsAvailable() {
					scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
						ID:          "docker-image-cve",
						Name:        "Docker Image CVE Scan",
						Description: "Scan Docker images for known CVEs using OpenSCAP",
						Type:        "oscap-docker",
						Category:    "docker",
					})
				}
			}

		// Send final status with scanner info
		if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
			Integration: "compliance",
			Enabled:     enabled,
			Status:      overallStatus,
			Message:     statusMessage,
			Components:  components,
			ScannerInfo: scannerDetails,
		}); err != nil {
			logger.WithError(err).Warn("Failed to send final compliance status")
		}
			return nil // Skip the generic status send below

		} else {
			logger.Info("Compliance disabled - removing tools...")
			overallStatus = "removing"

		// Send initial "removing" status
		if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
			Integration: "compliance",
			Enabled:     false,
			Status:      "removing",
			Message:     "Removing compliance tools...",
		}); err != nil {
			logger.WithError(err).Warn("Failed to send initial compliance removal status")
		}

			// Remove OpenSCAP packages
			openscapScanner := compliance.NewOpenSCAPScanner(logger)
			if err := openscapScanner.Cleanup(); err != nil {
				logger.WithError(err).Warn("Failed to remove OpenSCAP packages")
				components["openscap"] = "cleanup-failed"
			} else {
				logger.Info("OpenSCAP packages removed successfully")
				components["openscap"] = "removed"
			}

			// Clean up Docker Bench images
			dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
			if dockerBenchScanner.IsAvailable() {
				if err := dockerBenchScanner.Cleanup(); err != nil {
					logger.WithError(err).Debug("Failed to cleanup Docker Bench image")
					components["docker-bench"] = "cleanup-failed"
				} else {
					components["docker-bench"] = "removed"
				}
			}

			overallStatus = "disabled"
			statusMessage = "Compliance disabled and tools removed"
			logger.Info("Compliance cleanup complete")

			// Send final status update for disable
			if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
				Integration: "compliance",
				Enabled:     enabled,
				Status:      overallStatus,
				Message:     statusMessage,
				Components:  components,
			}); err != nil {
				logger.WithError(err).Warn("Failed to send final compliance disable status")
			}
		}
	}

	// Handle Docker Bench and oscap-docker installation when Docker is enabled AND Compliance is already enabled
	if integrationName == "docker" && enabled {
		if cfgManager.IsIntegrationEnabled("compliance") {
			logger.Info("Docker enabled with Compliance already active - setting up Docker scanning tools...")
			httpClient := client.New(cfgManager, logger)
			ctx := context.Background()

			openscapScanner := compliance.NewOpenSCAPScanner(logger)
			scannerDetails := openscapScanner.GetScannerDetails()

			// Setup Docker Bench
			dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
			if dockerBenchScanner.IsAvailable() {
				if err := dockerBenchScanner.EnsureInstalled(); err != nil {
					logger.WithError(err).Warn("Failed to pre-pull Docker Bench image (will pull on first scan)")
				} else {
					logger.Info("Docker Bench image pulled successfully")
					scannerDetails.DockerBenchAvailable = true
					scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
						ID:          "docker-bench",
						Name:        "Docker Bench for Security",
						Description: "CIS Docker Benchmark security checks",
						Type:        "docker-bench",
					})
				}
			} else {
				logger.Warn("Docker daemon not available - Docker Bench cannot be used")
			}

			// Setup oscap-docker for container image CVE scanning
			oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
			if !oscapDockerScanner.IsAvailable() {
				if err := oscapDockerScanner.EnsureInstalled(); err != nil {
					logger.WithError(err).Warn("Failed to install oscap-docker (container CVE scanning won't be available)")
				} else {
					logger.Info("oscap-docker installed successfully")
					scannerDetails.OscapDockerAvailable = true
					scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
						ID:          "docker-image-cve",
						Name:        "Docker Image CVE Scan",
						Description: "Scan Docker images for known CVEs using OpenSCAP",
						Type:        "oscap-docker",
						Category:    "docker",
					})
				}
			} else {
				logger.Info("oscap-docker already available")
				scannerDetails.OscapDockerAvailable = true
				scannerDetails.AvailableProfiles = append(scannerDetails.AvailableProfiles, models.ScanProfileInfo{
					ID:          "docker-image-cve",
					Name:        "Docker Image CVE Scan",
					Description: "Scan Docker images for known CVEs using OpenSCAP",
					Type:        "oscap-docker",
					Category:    "docker",
				})
			}

			// Send updated compliance status with Docker scanning tools
			if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
				Integration: "compliance",
				Enabled:     true,
				Status:      "ready",
				Message:     "Docker scanning tools now available",
				ScannerInfo: scannerDetails,
			}); err != nil {
				logger.WithError(err).Warn("Failed to send compliance status with Docker tools")
			}
		}
	}

	// Update config.yml
	if err := cfgManager.SetIntegrationEnabled(integrationName, enabled); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	logger.Info("Config updated, restarting patchmon-agent service...")

	// Restart the service to apply changes (supports systemd and OpenRC)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := exec.LookPath("systemctl"); err == nil {
		// Systemd is available
		logger.Debug("Detected systemd, using systemctl restart")
		cmd := exec.CommandContext(ctx, "systemctl", "restart", "patchmon-agent")
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.WithError(err).Warn("Failed to restart service (this is not critical)")
			return fmt.Errorf("failed to restart service: %w, output: %s", err, string(output))
		}
		logger.WithField("output", string(output)).Debug("Service restart command completed")
		logger.Info("Service restarted successfully")
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
			if closeErr := file.Close(); closeErr != nil {
				logger.WithError(closeErr).Warn("Failed to close file after write error")
			}
			if err := os.Remove(helperPath); err != nil {
				logger.WithError(err).Warn("Failed to remove helper script after write error")
			}
			// Fall through to exit approach
		} else {
			if err := file.Close(); err != nil {
				logger.WithError(err).Warn("Failed to close restart helper script file")
			}

				// SECURITY: Verify the file we're about to execute is the one we created
				// Check it's a regular file, not a symlink that was swapped in
				fileInfo, err := os.Lstat(helperPath)
				if err != nil || fileInfo.Mode()&os.ModeSymlink != 0 {
					logger.Warn("Security: helper script may have been tampered with, refusing to execute")
					if err := os.Remove(helperPath); err != nil {
						logger.WithError(err).Warn("Failed to remove tampered helper script")
					}
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
		logger.Info("Exiting to allow OpenRC to restart service with updated config...")
		os.Exit(0)
		// os.Exit never returns, but we need this for code flow
		return nil
	} else {
		logger.Warn("No known init system detected, attempting to restart via process signal")
		// Try to find and kill the process, service manager should restart it
		killCmd := exec.CommandContext(ctx, "pkill", "-HUP", "patchmon-agent")
		if err := killCmd.Run(); err != nil {
			logger.WithError(err).Warn("Failed to restart service (this is not critical)")
			return fmt.Errorf("failed to restart service: no init system detected and pkill failed: %w", err)
		}
		logger.Info("Sent HUP signal to agent process")
		return nil
	}
}

// runComplianceScan runs an on-demand compliance scan and sends results to server (backwards compatible)
func runComplianceScan(profileType string) error {
	return runComplianceScanWithOptions(&models.ComplianceScanOptions{
		ProfileID: profileType,
	})
}

// sendComplianceProgress sends a progress update via the global channel
func sendComplianceProgress(phase, profileName, message string, progress float64, errMsg string) {
	select {
	case complianceProgressChan <- ComplianceScanProgress{
		Phase:       phase,
		ProfileName: profileName,
		Message:     message,
		Progress:    progress,
		Error:       errMsg,
	}:
		// Successfully sent
	default:
		// Channel full or no listener, skip to avoid blocking
		logger.Debug("Compliance progress channel full, skipping update")
	}
}

// runComplianceScanWithOptions runs an on-demand compliance scan with options and sends results to server
func runComplianceScanWithOptions(options *models.ComplianceScanOptions) error {
	profileName := options.ProfileID
	if profileName == "" {
		profileName = "default"
	}

	logger.WithFields(map[string]interface{}{
		"profile_id":         options.ProfileID,
		"enable_remediation": options.EnableRemediation,
	}).Info("Starting on-demand compliance scan")

	// Send progress: started
	sendComplianceProgress("started", profileName, "Initializing compliance scan...", 5, "")

	// Create compliance integration
	complianceInteg := compliance.New(logger)
	// Set Docker integration status - Docker Bench only runs if Docker integration is enabled
	complianceInteg.SetDockerIntegrationEnabled(cfgManager.IsIntegrationEnabled("docker"))

	if !complianceInteg.IsAvailable() {
		sendComplianceProgress("failed", profileName, "Compliance scanning not available", 0, "compliance scanning not available on this system")
		return fmt.Errorf("compliance scanning not available on this system")
	}

	// Send progress: evaluating
	sendComplianceProgress("evaluating", profileName, "Running OpenSCAP evaluation (this may take several minutes)...", 15, "")

	// Run the scan with options (25 minutes to allow for complex systems)
	// OpenSCAP CIS Level 1 Server can take 15+ minutes on systems with many packages
	// Docker Bench needs additional time after OpenSCAP completes
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	integrationData, err := complianceInteg.CollectWithOptions(ctx, options)
	if err != nil {
		sendComplianceProgress("failed", profileName, "Scan failed", 0, err.Error())
		return fmt.Errorf("compliance scan failed: %w", err)
	}

	// Send progress: parsing
	sendComplianceProgress("parsing", profileName, "Processing scan results...", 80, "")

	// Extract compliance data
	complianceData, ok := integrationData.Data.(*models.ComplianceData)
	if !ok {
		sendComplianceProgress("failed", profileName, "Failed to extract compliance data", 0, "failed to extract compliance data")
		return fmt.Errorf("failed to extract compliance data")
	}

	if len(complianceData.Scans) == 0 {
		logger.Info("No compliance scans to send")
		sendComplianceProgress("completed", profileName, "Scan completed (no results)", 100, "")
		return nil
	}

	// Send progress: sending
	sendComplianceProgress("sending", profileName, "Uploading results to server...", 90, "")

	// Get system info
	systemDetector := system.New(logger)
	hostname, _ := systemDetector.GetHostname()
	machineID := systemDetector.GetMachineID()

	// Create payload
	payload := &models.CompliancePayload{
		ComplianceData: *complianceData,
		Hostname:       hostname,
		MachineID:      machineID,
		AgentVersion:   version.Version,
	}

	// Debug: log what we're about to send
	for i, scan := range payload.Scans {
		statusCounts := map[string]int{}
		for _, r := range scan.Results {
			statusCounts[r.Status]++
		}
		logger.WithFields(map[string]interface{}{
			"scan_index":      i,
			"profile_name":    scan.ProfileName,
			"profile_type":    scan.ProfileType,
			"total_results":   len(scan.Results),
			"result_statuses": statusCounts,
			"scan_passed":     scan.Passed,
			"scan_failed":     scan.Failed,
			"scan_warnings":   scan.Warnings,
			"scan_skipped":    scan.Skipped,
		}).Info("DEBUG: Compliance payload scan details before sending")
	}

	// Send to server
	httpClient := client.New(cfgManager, logger)
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer sendCancel()

	response, err := httpClient.SendComplianceData(sendCtx, payload)
	if err != nil {
		sendComplianceProgress("failed", profileName, "Failed to send results", 0, err.Error())
		return fmt.Errorf("failed to send compliance data: %w", err)
	}

	// Send progress: completed with score
	score := float64(0)
	if len(complianceData.Scans) > 0 {
		score = complianceData.Scans[0].Score
	}
	completedMsg := fmt.Sprintf("Scan completed! Score: %.1f%%", score)
	sendComplianceProgress("completed", profileName, completedMsg, 100, "")

	logFields := map[string]interface{}{
		"scans_received": response.ScansReceived,
		"message":        response.Message,
	}
	if options.EnableRemediation {
		logFields["remediation_enabled"] = true
	}
	logger.WithFields(logFields).Info("On-demand compliance scan results sent to server")

	return nil
}

// runDockerImageScan runs a CVE scan on Docker images using oscap-docker
func runDockerImageScan(imageName, containerName string, scanAllImages bool) error {
	logger.WithFields(map[string]interface{}{
		"image_name":      imageName,
		"container_name":  containerName,
		"scan_all_images": scanAllImages,
	}).Info("Starting Docker image CVE scan")

	// Check if Docker integration is enabled
	if !cfgManager.IsIntegrationEnabled("docker") {
		return fmt.Errorf("docker integration is not enabled")
	}

	// Check if compliance integration is enabled (required for oscap-docker)
	if !cfgManager.IsIntegrationEnabled("compliance") {
		return fmt.Errorf("compliance integration is not enabled (required for oscap-docker)")
	}

	// Create oscap-docker scanner
	oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
	if !oscapDockerScanner.IsAvailable() {
		sendComplianceProgress("failed", "Docker Image CVE Scan", "oscap-docker not available", 0, "oscap-docker is not installed or Docker is not running")
		return fmt.Errorf("oscap-docker is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var scans []*models.ComplianceScan

	if scanAllImages {
		// Scan all Docker images
		sendComplianceProgress("started", "Docker Image CVE Scan", "Scanning all Docker images for CVEs...", 5, "")

		results, err := oscapDockerScanner.ScanAllImages(ctx)
		if err != nil {
			sendComplianceProgress("failed", "Docker Image CVE Scan", "Failed to scan images", 0, err.Error())
			return fmt.Errorf("failed to scan all images: %w", err)
		}
		scans = results
	} else if imageName != "" {
		// Scan specific image
		sendComplianceProgress("started", "Docker Image CVE Scan", fmt.Sprintf("Scanning image %s for CVEs...", imageName), 5, "")

		scan, err := oscapDockerScanner.ScanImage(ctx, imageName)
		if err != nil {
			sendComplianceProgress("failed", "Docker Image CVE Scan", "Failed to scan image", 0, err.Error())
			return fmt.Errorf("failed to scan image %s: %w", imageName, err)
		}
		scans = append(scans, scan)
	} else if containerName != "" {
		// Scan specific container
		sendComplianceProgress("started", "Docker Image CVE Scan", fmt.Sprintf("Scanning container %s for CVEs...", containerName), 5, "")

		scan, err := oscapDockerScanner.ScanContainer(ctx, containerName)
		if err != nil {
			sendComplianceProgress("failed", "Docker Image CVE Scan", "Failed to scan container", 0, err.Error())
			return fmt.Errorf("failed to scan container %s: %w", containerName, err)
		}
		scans = append(scans, scan)
	} else {
		return fmt.Errorf("no image or container specified for scan")
	}

	if len(scans) == 0 {
		sendComplianceProgress("completed", "Docker Image CVE Scan", "No images to scan", 100, "")
		logger.Info("No Docker images to scan")
		return nil
	}

	// Send progress: parsing
	sendComplianceProgress("parsing", "Docker Image CVE Scan", "Processing scan results...", 80, "")

	// Convert pointer slice to value slice for ComplianceData
	scanValues := make([]models.ComplianceScan, len(scans))
	for i, scan := range scans {
		scanValues[i] = *scan
	}

	// Create compliance data structure
	complianceData := &models.ComplianceData{
		Scans: scanValues,
	}

	// Send progress: sending
	sendComplianceProgress("sending", "Docker Image CVE Scan", "Uploading results to server...", 90, "")

	// Get system info
	systemDetector := system.New(logger)
	hostname, _ := systemDetector.GetHostname()
	machineID := systemDetector.GetMachineID()

	// Create payload
	payload := &models.CompliancePayload{
		ComplianceData: *complianceData,
		Hostname:       hostname,
		MachineID:      machineID,
		AgentVersion:   version.Version,
	}

	// Send to server
	httpClient := client.New(cfgManager, logger)
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer sendCancel()

	response, err := httpClient.SendComplianceData(sendCtx, payload)
	if err != nil {
		sendComplianceProgress("failed", "Docker Image CVE Scan", "Failed to send results", 0, err.Error())
		return fmt.Errorf("failed to send Docker image scan data: %w", err)
	}

	// Send progress: completed
	totalCVEs := 0
	for _, scan := range scans {
		totalCVEs += scan.Failed
	}
	completedMsg := fmt.Sprintf("Scan completed! Found %d CVEs across %d images", totalCVEs, len(scans))
	sendComplianceProgress("completed", "Docker Image CVE Scan", completedMsg, 100, "")

	logger.WithFields(map[string]interface{}{
		"scans_received": response.ScansReceived,
		"images_scanned": len(scans),
		"cves_found":     totalCVEs,
	}).Info("Docker image CVE scan results sent to server")

	return nil
}

// validateSshProxyHost validates SSH proxy host to prevent injection
func validateSshProxyHost(host string) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if len(host) > 255 {
		return fmt.Errorf("host too long (max 255 chars)")
	}
	// Allow localhost, IP addresses, and valid hostnames
	validHostPattern := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$|^localhost$|^(\d{1,3}\.){3}\d{1,3}$`)
	if !validHostPattern.MatchString(host) {
		return fmt.Errorf("invalid host format")
	}
	return nil
}

// SSH proxy session management
type sshProxySession struct {
	client    *ssh.Client
	session   *ssh.Session
	stdin     io.WriteCloser
	stdout    io.Reader
	stderr    io.Reader
	conn      *websocket.Conn
	sessionID string
	mu        sync.Mutex
}

var sshProxySessions = make(map[string]*sshProxySession)
var sshProxySessionsMu sync.RWMutex

// sendSshProxyMessage sends a message to backend via WebSocket
func sendSshProxyMessage(conn *websocket.Conn, msgType string, sessionID string, data interface{}) {
	msg := map[string]interface{}{
		"type":       msgType,
		"session_id": sessionID,
	}
	if data != nil {
		msg["data"] = data
	}
	if msgType == "ssh_proxy_error" {
		if errMsg, ok := data.(string); ok {
			msg["message"] = errMsg
		}
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		logger.WithError(err).Error("Failed to marshal SSH proxy message")
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, msgJSON); err != nil {
		logger.WithError(err).Error("Failed to send SSH proxy message")
	}
}

func sendSshProxyError(conn *websocket.Conn, sessionID string, message string) {
	sendSshProxyMessage(conn, "ssh_proxy_error", sessionID, message)
}

func sendSshProxyData(conn *websocket.Conn, sessionID string, data string) {
	sendSshProxyMessage(conn, "ssh_proxy_data", sessionID, data)
}

func sendSshProxyConnected(conn *websocket.Conn, sessionID string) {
	sendSshProxyMessage(conn, "ssh_proxy_connected", sessionID, nil)
}

func sendSshProxyClosed(conn *websocket.Conn, sessionID string) {
	sendSshProxyMessage(conn, "ssh_proxy_closed", sessionID, nil)
}

// handleSshProxy establishes SSH connection and manages proxy session
func handleSshProxy(m wsMsg, conn *websocket.Conn) {
	sessionID := m.sshProxySessionID
	host := m.sshProxyHost
	if host == "" {
		host = "localhost"
	}
	port := m.sshProxyPort
	if port == 0 {
		port = 22
	}
	username := m.sshProxyUsername
	if username == "" {
		username = "root"
	}

	logger.WithFields(map[string]interface{}{
		"session_id": sessionID,
		"host":       host,
		"port":       port,
		"username":   username,
	}).Info("Establishing SSH proxy connection")

	// Create SSH client config
	config := &ssh.ClientConfig{
		User:            username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Accept any host key
		Timeout:         20 * time.Second,
	}

	// Set up authentication
	if m.sshProxyPrivateKey != "" {
		// Use private key authentication
		signer, err := ssh.ParsePrivateKey([]byte(m.sshProxyPrivateKey))
		if err != nil && m.sshProxyPassphrase != "" {
			// Try with passphrase
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(m.sshProxyPrivateKey), []byte(m.sshProxyPassphrase))
		}
		if err != nil {
			logger.WithError(err).Error("Failed to parse SSH private key")
			sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to parse private key: %v", err))
			return
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if m.sshProxyPassword != "" {
		// Use password authentication
		config.Auth = []ssh.AuthMethod{ssh.Password(m.sshProxyPassword)}
	} else {
		sendSshProxyError(conn, sessionID, "No authentication method provided (password or private key required)")
		return
	}

	// Connect to SSH server
	address := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		logger.WithError(err).Error("Failed to connect to SSH server")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	// Create session
	session, err := client.NewSession()
	if err != nil {
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close SSH client after session creation error")
		}
		logger.WithError(err).Error("Failed to create SSH session")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Set up terminal
	terminal := m.sshProxyTerminal
	if terminal == "" {
		terminal = "xterm-256color"
	}
	cols := m.sshProxyCols
	if cols == 0 {
		cols = 80
	}
	rows := m.sshProxyRows
	if rows == 0 {
		rows = 24
	}

	// Request PTY
	if err := session.RequestPty(terminal, rows, cols, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close session after PTY request error")
		}
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close client after PTY request error")
		}
		logger.WithError(err).Error("Failed to request PTY")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to request PTY: %v", err))
		return
	}

	// Set up stdin, stdout, stderr
	stdin, err := session.StdinPipe()
	if err != nil {
		if closeErr := session.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close session after stdin pipe error")
		}
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close client after stdin pipe error")
		}
		logger.WithError(err).Error("Failed to get stdin pipe")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to get stdin: %v", err))
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		if closeErr := stdin.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close stdin after stdout pipe error")
		}
		if closeErr := session.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close session after stdout pipe error")
		}
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close client after stdout pipe error")
		}
		logger.WithError(err).Error("Failed to get stdout pipe")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to get stdout: %v", err))
		return
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		if closeErr := stdin.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close stdin after stderr pipe error")
		}
		if closeErr := session.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close session after stderr pipe error")
		}
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close client after stderr pipe error")
		}
		logger.WithError(err).Error("Failed to get stderr pipe")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to get stderr: %v", err))
		return
	}

	// Start shell
	if err := session.Shell(); err != nil {
		if closeErr := stdin.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close stdin after shell start error")
		}
		if closeErr := session.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close session after shell start error")
		}
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close client after shell start error")
		}
		logger.WithError(err).Error("Failed to start shell")
		sendSshProxyError(conn, sessionID, fmt.Sprintf("Failed to start shell: %v", err))
		return
	}

	// Create session object
	proxySession := &sshProxySession{
		client:    client,
		session:   session,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		conn:      conn,
		sessionID: sessionID,
	}

	// Store session
	sshProxySessionsMu.Lock()
	sshProxySessions[sessionID] = proxySession
	sshProxySessionsMu.Unlock()

	// Send connected message
	sendSshProxyConnected(conn, sessionID)

	// Forward stdout to WebSocket
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := stdout.Read(buffer)
			if n > 0 {
				sendSshProxyData(conn, sessionID, string(buffer[:n]))
			}
			if err != nil {
				if err != io.EOF {
					logger.WithError(err).Error("Error reading from SSH stdout")
				}
				break
			}
		}
		// Clean up on stdout close
		handleSshProxyDisconnect(wsMsg{sshProxySessionID: sessionID}, conn)
	}()

	// Forward stderr to WebSocket
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := stderr.Read(buffer)
			if n > 0 {
				sendSshProxyData(conn, sessionID, string(buffer[:n]))
			}
			if err != nil {
				if err != io.EOF {
					logger.WithError(err).Error("Error reading from SSH stderr")
				}
				break
			}
		}
	}()

	// Wait for session to end
	go func() {
		err := session.Wait()
		if err != nil {
			logger.WithError(err).Debug("SSH session ended with error")
		}
		handleSshProxyDisconnect(wsMsg{sshProxySessionID: sessionID}, conn)
	}()
}

// handleSshProxyInput sends input to SSH session
func handleSshProxyInput(m wsMsg, conn *websocket.Conn) {
	sshProxySessionsMu.RLock()
	proxySession, exists := sshProxySessions[m.sshProxySessionID]
	sshProxySessionsMu.RUnlock()

	if !exists {
		logger.WithField("session_id", m.sshProxySessionID).Warn("SSH proxy session not found for input")
		return
	}

	proxySession.mu.Lock()
	defer proxySession.mu.Unlock()

	if proxySession.stdin != nil {
		if _, err := proxySession.stdin.Write([]byte(m.sshProxyData)); err != nil {
			logger.WithError(err).Error("Failed to write to SSH stdin")
		}
	}
}

// handleSshProxyResize resizes SSH terminal
func handleSshProxyResize(m wsMsg, conn *websocket.Conn) {
	sshProxySessionsMu.RLock()
	proxySession, exists := sshProxySessions[m.sshProxySessionID]
	sshProxySessionsMu.RUnlock()

	if !exists {
		logger.WithField("session_id", m.sshProxySessionID).Warn("SSH proxy session not found for resize")
		return
	}

	cols := m.sshProxyCols
	if cols == 0 {
		cols = 80
	}
	rows := m.sshProxyRows
	if rows == 0 {
		rows = 24
	}

	if proxySession.session != nil {
		if err := proxySession.session.WindowChange(rows, cols); err != nil {
			logger.WithError(err).Error("Failed to resize SSH terminal")
		}
	}
}

// handleSshProxyDisconnect closes SSH session
func handleSshProxyDisconnect(m wsMsg, conn *websocket.Conn) {
	sshProxySessionsMu.Lock()
	proxySession, exists := sshProxySessions[m.sshProxySessionID]
	if exists {
		delete(sshProxySessions, m.sshProxySessionID)
	}
	sshProxySessionsMu.Unlock()

	if !exists {
		return
	}

	logger.WithField("session_id", m.sshProxySessionID).Info("Closing SSH proxy session")

	// Close stdin
	if proxySession.stdin != nil {
		if err := proxySession.stdin.Close(); err != nil {
			logger.WithError(err).Warn("Failed to close SSH proxy stdin")
		}
	}

	// Close session
	if proxySession.session != nil {
		if err := proxySession.session.Close(); err != nil {
			logger.WithError(err).Warn("Failed to close SSH proxy session")
		}
	}

	// Close client
	if proxySession.client != nil {
		if err := proxySession.client.Close(); err != nil {
			logger.WithError(err).Warn("Failed to close SSH proxy client")
		}
	}

	// Send closed message
	sendSshProxyClosed(conn, m.sshProxySessionID)
}
