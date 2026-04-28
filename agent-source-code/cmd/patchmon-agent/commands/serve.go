package commands

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"patchmon-agent/internal/client"
	"patchmon-agent/internal/config"
	"patchmon-agent/internal/integrations"
	"patchmon-agent/internal/integrations/compliance"
	"patchmon-agent/internal/integrations/docker"
	"patchmon-agent/internal/logutil"
	"patchmon-agent/internal/packages"
	"patchmon-agent/internal/pkgversion"
	"patchmon-agent/internal/system"
	"patchmon-agent/internal/utils"
	"patchmon-agent/pkg/models"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// serveCmd runs the agent as a long-lived service
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the agent as a service with async updates",
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := checkRoot(); err != nil {
			return err
		}
		return runAsService()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

// agentHostKeyCallback returns a HostKeyCallback that validates against ~/.ssh/known_hosts
// when available, otherwise falls back to InsecureIgnoreHostKey. Used for SSH proxy connections.
func agentHostKeyCallback() ssh.HostKeyCallback {
	home, _ := os.UserHomeDir()
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(knownHostsPath); err == nil {
		cb, err := knownhosts.New(knownHostsPath)
		if err == nil {
			return cb
		}
	}
	return ssh.InsecureIgnoreHostKey()
}

// runServiceLoop is the main service loop. stopCh signals shutdown (nil = run forever on Unix)
func runServiceLoop(stopCh <-chan struct{}) error {
	// When running as Windows service, allow a brief delay for system initialization
	// (network, filesystem) to be ready after SCM starts the process. This addresses
	// first-start issues where the report task would not run.
	if runtime.GOOS == "windows" && isWindowsService() {
		logger.Info("Windows service detected, waiting briefly for system initialization...")
		time.Sleep(5 * time.Second)
	}

	// Load credentials with retry on Windows service (first start may race with installer)
	var loadErr error
	for attempt := 0; attempt < 3; attempt++ {
		loadErr = cfgManager.LoadCredentials()
		if loadErr == nil {
			break
		}
		if runtime.GOOS == "windows" && isWindowsService() && attempt < 2 {
			logger.WithError(loadErr).Warn("Failed to load credentials, retrying in 2s...")
			time.Sleep(2 * time.Second)
		} else {
			return loadErr
		}
	}
	if loadErr != nil {
		return loadErr
	}

	httpClient := client.New(cfgManager, logger)
	ctx := context.Background()

	// Get api_id for offset calculation
	apiID := cfgManager.GetCredentials().APIID

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
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"config_interval": intervalMinutes,
				"server_interval": resp.UpdateInterval,
			})).Info("Server interval differs from config, updating config.yml")

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
				logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
					"integration":  integrationName,
					"config_value": configEnabled,
					"server_value": serverEnabled,
				})).Info("Integration status differs, updating config.yml")

				if err := cfgManager.SetIntegrationEnabled(integrationName, serverEnabled); err != nil {
					logger.WithError(err).Warn("Failed to save integration status to config.yml")
				} else {
					configUpdated = true
					logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
						"integration": integrationName,
						"enabled":     serverEnabled,
					})).Info("Updated integration status in config.yml")
				}
			}
		}

		// Sync compliance scanner toggles from server (when server sends them)
		if integrationResp.ComplianceOpenscapEnabled != nil || integrationResp.ComplianceDockerBenchEnabled != nil {
			configOpenscap := cfgManager.GetComplianceOpenscapEnabled()
			configDockerBench := cfgManager.GetComplianceDockerBenchEnabled()
			serverOpenscap := configOpenscap
			serverDockerBench := configDockerBench
			if integrationResp.ComplianceOpenscapEnabled != nil {
				serverOpenscap = *integrationResp.ComplianceOpenscapEnabled
			}
			if integrationResp.ComplianceDockerBenchEnabled != nil {
				serverDockerBench = *integrationResp.ComplianceDockerBenchEnabled
			}
			if serverOpenscap != configOpenscap || serverDockerBench != configDockerBench {
				logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
					"config_openscap": configOpenscap, "server_openscap": serverOpenscap,
					"config_docker_bench": configDockerBench, "server_docker_bench": serverDockerBench,
				})).Info("Compliance scanner toggles differ, updating config.yml")
				if err := cfgManager.SetComplianceScanners(serverOpenscap, serverDockerBench); err != nil {
					logger.WithError(err).Warn("Failed to save compliance scanner toggles to config.yml")
				} else {
					configUpdated = true
					logger.Info("Updated compliance scanner toggles in config.yml")
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
	calculatedOffset := utils.CalculateReportOffset(apiID, intervalMinutes)
	calculatedOffsetSeconds := int(calculatedOffset.Seconds())

	// Use config offset if it exists and matches calculated value, otherwise recalculate and save
	if configOffsetSeconds > 0 && configOffsetSeconds == calculatedOffsetSeconds {
		offset = time.Duration(configOffsetSeconds) * time.Second
		logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
			"api_id":           apiID,
			"interval_minutes": intervalMinutes,
			"offset_seconds":   offset.Seconds(),
		})).Info("Loaded report offset from config.yml")
	} else {
		// Offset not in config or doesn't match, calculate and save it
		offset = calculatedOffset
		if err := cfgManager.SetReportOffset(calculatedOffsetSeconds); err != nil {
			logger.WithError(err).Warn("Failed to save offset to config.yml")
		} else {
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"api_id":           apiID,
				"interval_minutes": intervalMinutes,
				"offset_seconds":   offset.Seconds(),
			})).Info("Calculated and saved report offset to config.yml")
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
	go func() {
		logger.Info("Sending initial report on startup (background)...")
		if err := sendReport(false); err != nil {
			logger.WithError(err).Warn("initial report failed")
		} else {
			logger.Info("✅ Initial report sent successfully")
		}
	}()

	var compScheduler *complianceScheduler
	if cfgManager.IsIntegrationEnabled("compliance") && !cfgManager.IsComplianceOnDemandOnly() {
		compScheduler = newComplianceScheduler(cfgManager.GetComplianceScanInterval())
		compScheduler.Start()
		defer compScheduler.Stop()
	}

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

	// Create a stop channel that never closes if none provided (for Unix systems)
	effectiveStopCh := stopCh
	if effectiveStopCh == nil {
		effectiveStopCh = make(chan struct{}) // never closed
	}

	for {
		select {
		case <-effectiveStopCh:
			// Shutdown requested
			logger.Info("Shutdown signal received, stopping service...")
			return nil
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
					newOffset := utils.CalculateReportOffset(apiID, m.interval)
					newOffsetSeconds := int(newOffset.Seconds())
					if err := cfgManager.SetReportOffset(newOffsetSeconds); err != nil {
						logger.WithError(err).Warn("Failed to save offset to config.yml")
					}

					logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
						"old_interval":       currentInterval,
						"new_interval":       m.interval,
						"new_offset_seconds": newOffset.Seconds(),
					})).Info("Recalculated and saved offset for new interval")

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
				if m.complianceScanInterval > 0 && compScheduler != nil {
					if err := cfgManager.SetComplianceScanInterval(m.complianceScanInterval); err != nil {
						logger.WithError(err).Warn("Failed to save compliance scan interval to config.yml")
					} else {
						compScheduler.Reset(m.complianceScanInterval)
						logger.WithField("compliance_scan_interval", m.complianceScanInterval).Info("Compliance scan interval updated")
					}
				}
				if m.packageCacheRefreshMode != "" {
					if err := cfgManager.SetPackageCacheRefresh(m.packageCacheRefreshMode, m.packageCacheRefreshMaxAge); err != nil {
						logger.WithError(err).Warn("Failed to save package cache refresh settings to config.yml")
					} else {
						logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
							"mode":    m.packageCacheRefreshMode,
							"max_age": m.packageCacheRefreshMaxAge,
						})).Info("Package cache refresh settings updated")
					}
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
			case "run_patch":
				go func(msg wsMsg) {
					if err := runPatch(msg.patchRunID, msg.patchType, msg.packageNames, msg.dryRun); err != nil {
						logger.WithError(err).Warn("run_patch failed")
					} else {
						logger.Info("run_patch completed successfully")
					}
				}(m)
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
					logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
						"integration": m.integrationName,
						"enabled":     m.integrationEnabled,
					})).Info("Integration toggled successfully, service will restart")
				}
			case "compliance_scan":
				logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
					"profile_type":       m.profileType,
					"profile_id":         m.profileID,
					"enable_remediation": m.enableRemediation,
				})).Info("Running on-demand compliance scan...")
				go func(msg wsMsg) {
					complianceScanCancelMu.Lock()
					if complianceScanSource == "scheduled" && complianceScanCancel != nil {
						complianceScanCancel()
						logger.Info("Cancelled running scheduled scan to run on-demand scan")
					}
					complianceScanCancelMu.Unlock()

					for i := 0; i < 10; i++ {
						if complianceScanRunning.CompareAndSwap(false, true) {
							break
						}
						time.Sleep(500 * time.Millisecond)
					}
					if !complianceScanRunning.Load() {
						complianceScanRunning.Store(true)
					}

					complianceScanCancelMu.Lock()
					complianceScanSource = "on-demand"
					complianceScanCancelMu.Unlock()

					defer func() {
						complianceScanCancelMu.Lock()
						complianceScanSource = ""
						complianceScanCancelMu.Unlock()
						complianceScanRunning.Store(false)
					}()

					ctx, cancel := context.WithCancel(context.Background())
					complianceScanCancelMu.Lock()
					complianceScanCancel = cancel
					complianceScanCancelMu.Unlock()
					defer func() {
						complianceScanCancelMu.Lock()
						complianceScanCancel = nil
						complianceScanCancelMu.Unlock()
					}()
					options := &models.ComplianceScanOptions{
						ProfileID:            msg.profileID,
						EnableRemediation:    msg.enableRemediation,
						FetchRemoteResources: msg.fetchRemoteResources,
						OpenSCAPEnabled:      msg.openscapEnabled,
						DockerBenchEnabled:   msg.dockerBenchEnabled,
					}
					if err := runComplianceScanWithOptions(ctx, options); err != nil {
						if errors.Is(err, context.Canceled) {
							logger.Info("Compliance scan was cancelled")
						} else {
							logger.WithError(err).Warn("compliance_scan failed")
						}
					} else {
						if msg.enableRemediation {
							logger.Info("On-demand compliance scan with remediation completed successfully")
						} else {
							logger.Info("On-demand compliance scan completed successfully")
						}
					}
				}(m)
			case "compliance_scan_cancel":
				complianceScanCancelMu.Lock()
				cancelFn := complianceScanCancel
				complianceScanCancel = nil
				complianceScanCancelMu.Unlock()
				if cancelFn != nil {
					cancelFn()
					logger.Info("Compliance scan cancel requested and sent to running scan")
				} else {
					logger.Debug("Compliance scan cancel requested but no scan is running")
				}
			case "patch_run_stop":
				if v, ok := patchRunCancels.Load(m.patchRunID); ok {
					if cancelFn, ok := v.(context.CancelFunc); ok && cancelFn != nil {
						patchRunStopped.Store(m.patchRunID, true)
						cancelFn()
						logger.WithField("patch_run_id", logutil.Sanitize(m.patchRunID)).Info("Patch run stop honored; interrupt sent")
					}
				} else {
					logger.WithField("patch_run_id", logutil.Sanitize(m.patchRunID)).Debug("Patch run stop requested but no matching run is active")
				}
			case "upgrade_ssg":
				targetVersion := m.version
				logger.WithField("target_version", targetVersion).Info("Upgrading SSG content packages...")
				go func() {
					if err := upgradeSSGContent(targetVersion); err != nil {
						logger.WithError(err).Warn("upgrade_ssg failed")
					} else {
						logger.Info("SSG content packages upgraded successfully")
					}
				}()
			case "install_scanner":
				logger.Info("Install scanner requested (OpenSCAP + SSG)...")
				go func() {
					if err := runInstallScanner(); err != nil {
						logger.WithError(err).Warn("install_scanner failed")
					} else {
						logger.Info("Install scanner completed successfully")
					}
				}()
			case "remediate_rule":
				logger.WithField("rule_id", logutil.Sanitize(m.ruleID)).Info("Remediating single rule...")
				go func(ruleID string) {
					if err := remediateSingleRule(ruleID); err != nil {
						logger.WithError(err).WithField("rule_id", logutil.Sanitize(ruleID)).Warn("remediate_rule failed")
					} else {
						logger.WithField("rule_id", logutil.Sanitize(ruleID)).Info("Single rule remediation completed")
					}
				}(m.ruleID)
			case "docker_image_scan":
				logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
					"image_name":      m.imageName,
					"container_name":  m.containerName,
					"scan_all_images": m.scanAllImages,
				})).Info("Running Docker image CVE scan...")
				go func(msg wsMsg) {
					if err := runDockerImageScan(msg.imageName, msg.containerName, msg.scanAllImages); err != nil {
						logger.WithError(err).Warn("docker_image_scan failed")
					} else {
						logger.Info("Docker image CVE scan completed successfully")
					}
				}(m)
			case "set_compliance_mode":
				logger.WithField("mode", logutil.Sanitize(m.complianceMode)).Info("Setting compliance mode...")
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
					logger.WithField("mode", logutil.Sanitize(m.complianceMode)).Warn("Invalid compliance mode, ignoring")
					continue
				}
				if err := cfgManager.SetComplianceMode(mode); err != nil {
					logger.WithError(err).Warn("Failed to set compliance mode")
				} else {
					logger.WithField("mode", logutil.Sanitize(m.complianceMode)).Info("Compliance mode updated in config.yml")
				}
			case "apply_config":
				if err := applyConfig(m.applyConfig); err != nil {
					logger.WithError(err).Warn("apply_config failed")
				} else {
					logger.Info("apply_config completed, service will restart")
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
				logger.WithField("session_id", logutil.Sanitize(m.sshProxySessionID)).Info("Handling SSH proxy connection request")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					go handleSSHProxy(m, wsConn)
				}
			case "ssh_proxy_input":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleSSHProxyInput(m, wsConn)
				}
			case "ssh_proxy_resize":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleSSHProxyResize(m, wsConn)
				}
			case "ssh_proxy_disconnect":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleSSHProxyDisconnect(m, wsConn)
				}
			case "rdp_proxy":
				logger.WithField("session_id", logutil.Sanitize(m.rdpProxySessionID)).Info("Handling RDP proxy connection request")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					go handleRDPProxy(m, wsConn)
				}
			case "rdp_proxy_input":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleRDPProxyInput(m, wsConn)
				}
			case "rdp_proxy_disconnect":
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					handleRDPProxyDisconnect(m, wsConn)
				}
			}
		}
	}
}

// ssgClientAdapter adapts the agent HTTP client to the SSGContentDownloader interface.
type ssgClientAdapter struct {
	c *client.Client
}

func (a *ssgClientAdapter) GetSSGVersion(ctx context.Context) (string, []string, error) {
	resp, err := a.c.GetSSGVersion(ctx)
	if err != nil {
		return "", nil, err
	}
	return resp.Version, resp.Files, nil
}

func (a *ssgClientAdapter) DownloadSSGContent(ctx context.Context, filename, destPath string) error {
	return a.c.DownloadSSGContent(ctx, filename, destPath)
}

// upgradeSSGContent upgrades the SCAP Security Guide content packages.
// Prefers downloading from PatchMon server; falls back to GitHub if server has no content.
func upgradeSSGContent(targetVersion string) error {
	httpClient := client.New(cfgManager, logger)
	complianceInteg := compliance.New(logger)

	downloader := &ssgClientAdapter{c: httpClient}
	if err := complianceInteg.UpgradeSSGContentFromServer(downloader, targetVersion); err != nil {
		logger.WithError(err).Warn("Server-based SSG upgrade failed, falling back to GitHub...")
		if fallbackErr := complianceInteg.UpgradeSSGContent(); fallbackErr != nil {
			return fmt.Errorf("server upgrade: %w; github fallback: %v", err, fallbackErr)
		}
	}

	logger.Info("Sending updated compliance status to backend...")
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

// runInstallScanner installs OpenSCAP and SSG content (apt/dnf install, update SSG) and reports status via HTTP
// Sends granular install events so the frontend can display real-time progress.
func runInstallScanner() error {
	httpClient := client.New(cfgManager, logger)
	ctx := context.Background()
	enabled := cfgManager.IsIntegrationEnabled("compliance")

	events := make([]models.InstallEvent, 0, 8)

	addEvent := func(step, status, message string) {
		events = append(events, models.InstallEvent{
			Step:      step,
			Status:    status,
			Message:   message,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}

	sendStatus := func(overallStatus, message string, scannerInfo *models.ComplianceScannerDetails) {
		_ = httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
			Integration:   "compliance",
			Enabled:       enabled,
			Status:        overallStatus,
			Message:       message,
			InstallEvents: events,
			ScannerInfo:   scannerInfo,
		})
	}

	// Step 1: Detect OS
	addEvent("detect_os", "in_progress", "Detecting operating system...")
	sendStatus("installing", "Detecting operating system...", nil)

	openscapScanner := compliance.NewOpenSCAPScanner(logger)
	osInfo := openscapScanner.GetOSInfo()
	osDesc := fmt.Sprintf("%s %s (%s)", osInfo.Name, osInfo.Version, osInfo.Family)
	if osInfo.Name == "" {
		osDesc = "unknown OS"
	}

	// Mark detect_os done
	events[len(events)-1] = models.InstallEvent{
		Step:      "detect_os",
		Status:    "done",
		Message:   fmt.Sprintf("Detected %s", osDesc),
		Timestamp: events[len(events)-1].Timestamp,
	}

	// Step 2: Install OpenSCAP packages
	addEvent("install_openscap", "in_progress", "Installing OpenSCAP packages...")
	sendStatus("installing", "Installing OpenSCAP packages...", nil)

	if err := openscapScanner.EnsureInstalled(); err != nil {
		logger.WithError(err).Warn("EnsureInstalled failed")
		events[len(events)-1] = models.InstallEvent{
			Step:      "install_openscap",
			Status:    "failed",
			Message:   fmt.Sprintf("OpenSCAP installation failed: %s", err.Error()),
			Timestamp: events[len(events)-1].Timestamp,
		}
		addEvent("complete", "failed", "Installation failed")
		sendStatus("error", err.Error(), openscapScanner.GetScannerDetails())
		return err
	}

	events[len(events)-1] = models.InstallEvent{
		Step:      "install_openscap",
		Status:    "done",
		Message:   "OpenSCAP packages installed successfully",
		Timestamp: events[len(events)-1].Timestamp,
	}

	// Step 3: Verify installation and SSG content
	addEvent("verify_openscap", "in_progress", "Verifying OpenSCAP installation and SSG content...")
	sendStatus("installing", "Verifying OpenSCAP installation...", nil)

	scannerDetails := openscapScanner.GetScannerDetails()
	verifyMsg := "OpenSCAP verified"
	if scannerDetails.OpenSCAPVersion != "" {
		verifyMsg = fmt.Sprintf("OpenSCAP %s verified", scannerDetails.OpenSCAPVersion)
	}
	if scannerDetails.ContentPackage != "" {
		verifyMsg += fmt.Sprintf(", SSG content: %s", scannerDetails.ContentPackage)
	}
	events[len(events)-1] = models.InstallEvent{
		Step:      "verify_openscap",
		Status:    "done",
		Message:   verifyMsg,
		Timestamp: events[len(events)-1].Timestamp,
	}

	// Step 3b: Sync SSG content from PatchMon server (server is single source of truth).
	// This ensures the agent has the same SSG version the server was built with,
	// regardless of what the OS package manager provided.
	addEvent("sync_ssg", "in_progress", "Syncing SSG content from PatchMon server...")
	sendStatus("installing", "Syncing SSG content from server...", nil)

	downloader := &ssgClientAdapter{c: httpClient}
	if err := openscapScanner.UpgradeSSGContentFromServer(downloader, ""); err != nil {
		logger.WithError(err).Warn("Server-based SSG sync failed (package manager version will be used)")
		events[len(events)-1] = models.InstallEvent{
			Step:      "sync_ssg",
			Status:    "skipped",
			Message:   fmt.Sprintf("Server SSG sync skipped: %s", err.Error()),
			Timestamp: events[len(events)-1].Timestamp,
		}
	} else {
		// Re-read scanner details after server sync
		scannerDetails = openscapScanner.GetScannerDetails()
		syncMsg := "SSG content synced from server"
		if scannerDetails.SSGVersion != "" {
			syncMsg = fmt.Sprintf("SSG content synced from server (v%s)", scannerDetails.SSGVersion)
		}
		events[len(events)-1] = models.InstallEvent{
			Step:      "sync_ssg",
			Status:    "done",
			Message:   syncMsg,
			Timestamp: events[len(events)-1].Timestamp,
		}
	}

	// Step 4: Docker Bench (if docker enabled)
	dockerIntegrationEnabled := cfgManager.IsIntegrationEnabled("docker")
	if dockerIntegrationEnabled {
		addEvent("docker_bench", "in_progress", "Pre-pulling Docker Bench image...")
		sendStatus("installing", "Pre-pulling Docker Bench image...", nil)

		dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
		if dockerBenchScanner.IsAvailable() {
			if err := dockerBenchScanner.EnsureInstalled(); err != nil {
				logger.WithError(err).Warn("Failed to pre-pull Docker Bench image")
				events[len(events)-1] = models.InstallEvent{
					Step:      "docker_bench",
					Status:    "failed",
					Message:   fmt.Sprintf("Docker Bench image pull failed: %s", err.Error()),
					Timestamp: events[len(events)-1].Timestamp,
				}
			} else {
				scannerDetails.DockerBenchAvailable = true
				events[len(events)-1] = models.InstallEvent{
					Step:      "docker_bench",
					Status:    "done",
					Message:   "Docker Bench image pulled successfully",
					Timestamp: events[len(events)-1].Timestamp,
				}
			}
		} else {
			events[len(events)-1] = models.InstallEvent{
				Step:      "docker_bench",
				Status:    "skipped",
				Message:   "Docker not available on this host",
				Timestamp: events[len(events)-1].Timestamp,
			}
		}

		oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
		scannerDetails.OscapDockerAvailable = oscapDockerScanner.IsAvailable()
	} else {
		addEvent("docker_bench", "skipped", "Docker integration not enabled, skipping Docker Bench setup")
	}

	// Step 5: Complete
	addEvent("complete", "done", "Installation complete - scanner is ready")
	sendStatus("ready", "OpenSCAP and SSG content installed and ready", scannerDetails)

	return nil
}

// remediateSingleRule remediates a single failed compliance rule
func remediateSingleRule(ruleID string) error {
	if ruleID == "" {
		return fmt.Errorf("rule ID is required")
	}

	logger.WithField("rule_id", logutil.Sanitize(ruleID)).Info("Starting single rule remediation")

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

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"profile_id": options.ProfileID,
		"rule_id":    options.RuleID,
	})).Info("Running single rule remediation with oscap")

	_, err := complianceInteg.CollectWithOptions(ctx, options)
	if err != nil {
		return fmt.Errorf("remediation failed: %w", err)
	}

	logger.WithField("rule_id", logutil.Sanitize(ruleID)).Info("Single rule remediation completed successfully")
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
		AgentVersion: pkgversion.Version,
	}

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"containers": len(data.Containers),
		"images":     len(data.Images),
		"volumes":    len(data.Volumes),
		"networks":   len(data.Networks),
	})).Info("Sending Docker inventory to server...")

	// Create HTTP client and send data
	httpClient := client.New(cfgManager, logger)
	sendCtx, sendCancel := context.WithTimeout(ctx, 30*time.Second)
	defer sendCancel()

	response, err := httpClient.SendDockerData(sendCtx, payload)
	if err != nil {
		logger.WithError(err).Warn("Failed to send Docker inventory")
		return
	}

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"containers": response.ContainersReceived,
		"images":     response.ImagesReceived,
		"volumes":    response.VolumesReceived,
		"networks":   response.NetworksReceived,
	})).Info("Docker inventory refresh completed successfully")
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
	kind                      string
	interval                  int
	complianceScanInterval    int
	packageCacheRefreshMode   string
	packageCacheRefreshMaxAge int
	version                   string
	force                     bool
	integrationName           string
	integrationEnabled        bool
	profileType               string                 // For compliance_scan: openscap, docker-bench, all
	profileID                 string                 // For compliance_scan: specific XCCDF profile ID
	enableRemediation         bool                   // For compliance_scan: enable auto-remediation
	fetchRemoteResources      bool                   // For compliance_scan: fetch remote resources
	openscapEnabled           *bool                  // For compliance_scan: per-host OpenSCAP scanner toggle
	dockerBenchEnabled        *bool                  // For compliance_scan: per-host Docker Bench scanner toggle
	ruleID                    string                 // For remediate_rule: specific rule ID to remediate
	imageName                 string                 // For docker_image_scan: Docker image to scan
	containerName             string                 // For docker_image_scan: Docker container to scan
	scanAllImages             bool                   // For docker_image_scan: scan all images on system
	complianceOnDemandOnly    bool                   // For set_compliance_on_demand_only (legacy)
	complianceMode            string                 // For set_compliance_mode: "disabled", "on-demand", or "enabled"
	applyConfig               map[string]interface{} // For apply_config: full config to apply
	// SSH proxy fields
	sshProxySessionID  string // Unique session ID for SSH proxy
	sshProxyHost       string // SSH target host
	sshProxyPort       int    // SSH target port
	sshProxyUsername   string // SSH username
	sshProxyPassword   string // SSH password
	sshProxyPrivateKey string // SSH private key
	sshProxyPassphrase string // SSH private key passphrase
	sshProxyTerminal   string // Terminal type
	sshProxyCols       int    // Terminal columns
	sshProxyRows       int    // Terminal rows
	// run_patch fields
	patchRunID   string
	patchType    string
	packageNames []string
	dryRun       bool
	sshProxyData string // SSH input data
	// RDP proxy fields
	rdpProxySessionID string // Unique session ID for RDP proxy
	rdpProxyHost      string // RDP target host (default localhost)
	rdpProxyPort      int    // RDP target port (default 3389)
	rdpProxyData      string // RDP input data (base64)
}

// Input validation patterns for WebSocket message fields
// These prevent command injection by ensuring only safe characters are allowed
var (
	// Profile IDs: alphanumeric, underscores, dots, hyphens (e.g., xccdf_org.ssgproject.content_profile_level1_server)
	validProfileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)
	// Rule IDs: same as profile IDs (e.g., xccdf_org.ssgproject.content_rule_audit_rules_...)
	validRuleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)
	// APT package names: alphanumeric, dots, plus, minus, underscores (no path/command injection)
	validAptPackagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+-_]*$`)
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
var globalWsWriteMu sync.Mutex

var complianceScanRunning atomic.Bool
var complianceScanCancel context.CancelFunc
var complianceScanCancelMu sync.Mutex
var complianceScanSource string

func writeWebSocketTextMessage(conn *websocket.Conn, payload []byte) error {
	globalWsWriteMu.Lock()
	defer globalWsWriteMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.WithError(err).Debug("Failed to set WebSocket write deadline")
	}

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return err
	}
	return nil
}

// patchRunCancels maps patchRunID -> context.CancelFunc for in-flight patch runs.
// Allows the server to request an interrupt via the "patch_run_stop" WS message.
var patchRunCancels sync.Map

// patchRunStopped records patchRunIDs that were explicitly stopped by the server so
// the runner can report stage="cancelled" instead of "failed" after the process exits.
var patchRunStopped sync.Map

type complianceScheduler struct {
	interval time.Duration
	stopCh   chan struct{}
	resetCh  chan time.Duration
}

func newComplianceScheduler(intervalMinutes int) *complianceScheduler {
	return &complianceScheduler{
		interval: time.Duration(intervalMinutes) * time.Minute,
		stopCh:   make(chan struct{}),
		resetCh:  make(chan time.Duration, 1),
	}
}

func (cs *complianceScheduler) Start() {
	go cs.loop()
}

func (cs *complianceScheduler) Stop() {
	close(cs.stopCh)
}

func (cs *complianceScheduler) Reset(intervalMinutes int) {
	newInterval := time.Duration(intervalMinutes) * time.Minute
	select {
	case cs.resetCh <- newInterval:
	default:
	}
}

func (cs *complianceScheduler) loop() {
	logger.WithField("compliance_scan_interval_minutes", int(cs.interval.Minutes())).Info("Compliance scheduler started")

	select {
	case <-time.After(30 * time.Second):
	case <-cs.stopCh:
		return
	}

	runScheduledComplianceScan()

	ticker := time.NewTicker(cs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-cs.stopCh:
			logger.Info("Compliance scheduler stopped")
			return
		case newInterval := <-cs.resetCh:
			ticker.Stop()
			cs.interval = newInterval
			ticker = time.NewTicker(cs.interval)
			logger.WithField("compliance_scan_interval_minutes", int(cs.interval.Minutes())).Info("Compliance scan interval updated")
		case <-ticker.C:
			runScheduledComplianceScan()
		}
	}
}

func wsLoop(out chan<- wsMsg, dockerEvents <-chan interface{}) {
	backoff := time.Second
	for {
		// connectOnce resets backoff to 1s on successful dial so a long-lived
		// agent that drops its WS (e.g. Windows bouncing TermService/firewall
		// when RDP settings change) reconnects fast instead of waiting out the
		// escalated backoff from its prior drops.
		connected, err := connectOnce(out, dockerEvents, &backoff)
		if err != nil {
			logger.WithError(err).Warn("ws disconnected; retrying")
		}
		sleepFor := backoff
		if !connected && backoff < 30*time.Second {
			backoff *= 2
		}
		time.Sleep(sleepFor)
	}
}

func connectOnce(out chan<- wsMsg, dockerEvents <-chan interface{}, backoff *time.Duration) (connected bool, err error) {
	server := cfgManager.GetConfig().PatchmonServer
	if server == "" {
		return false, nil
	}
	apiID := cfgManager.GetCredentials().APIID
	apiKey := cfgManager.GetCredentials().APIKey

	// Convert http(s) -> ws(s)
	wsURL := server
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(wsURL, "https://")
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	} else if strings.HasPrefix(wsURL, "wss://") || strings.HasPrefix(wsURL, "ws://") {
		// Already a WebSocket URL, use as-is - no conversion needed
		_ = wsURL // URL is already in correct format, no action needed
	} else {
		// No protocol prefix - assume HTTPS and use WSS
		logger.WithField("server", logutil.Sanitize(server)).Warn("Server URL missing protocol prefix, assuming HTTPS")
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
	if cfgManager.GetConfig().SkipSSLVerify || client.IsSkipSSLVerifyEnvSet() {
		logger.Warn("TLS verification disabled for WebSocket")
		// Operator-gated insecure TLS for lab/air-gapped deployments with self-signed certs.
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return false, err
	}
	// Reset reconnect backoff now that the session is live. Without this, a
	// long-lived agent that has escalated backoff from earlier drops would wait
	// the full capped interval on the next disconnect even though the link
	// recovered immediately.
	connected = true
	*backoff = time.Second

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

	logger.WithField("url", logutil.Sanitize(wsURL)).Info("WebSocket connected")

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
		// OPTIMIZATION: Add a ticker to prevent goroutine buildup
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Periodic health check
				continue
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

					if err := writeWebSocketTextMessage(conn, eventJSON); err != nil {
						logger.WithError(err).Debug("Failed to send Docker event via WebSocket")
						return
					}
				}
			}
		}
	}()

	// Create a goroutine to send compliance scan progress updates through WebSocket
	go func() {
		// OPTIMIZATION: Add a ticker to prevent goroutine buildup
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Periodic health check
				continue
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

				if err := writeWebSocketTextMessage(conn, progressJSON); err != nil {
					logger.WithError(err).Debug("Failed to send compliance progress via WebSocket")
					return
				}
				logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
					"phase":   progress.Phase,
					"message": progress.Message,
				})).Debug("Sent compliance progress update via WebSocket")
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return connected, err
		}
		var payload struct {
			Type                      string                 `json:"type"`
			UpdateInterval            int                    `json:"update_interval"`
			ComplianceScanInterval    int                    `json:"compliance_scan_interval"`
			PackageCacheRefreshMode   string                 `json:"package_cache_refresh_mode"`
			PackageCacheRefreshMaxAge int                    `json:"package_cache_refresh_max_age"`
			Version                   string                 `json:"version"`
			Force                     bool                   `json:"force"`
			Message                   string                 `json:"message"`
			Integration               string                 `json:"integration"`
			Enabled                   bool                   `json:"enabled"`
			ProfileType               string                 `json:"profile_type"`           // For compliance_scan
			ProfileID                 string                 `json:"profile_id"`             // For compliance_scan: specific XCCDF profile ID
			EnableRemediation         bool                   `json:"enable_remediation"`     // For compliance_scan
			FetchRemoteResources      bool                   `json:"fetch_remote_resources"` // For compliance_scan
			OpenSCAPEnabled           *bool                  `json:"openscap_enabled"`       // For compliance_scan: per-host toggle
			DockerBenchEnabled        *bool                  `json:"docker_bench_enabled"`   // For compliance_scan: per-host toggle
			RuleID                    string                 `json:"rule_id"`                // For remediate_rule: specific rule to remediate
			ImageName                 string                 `json:"image_name"`             // For docker_image_scan: Docker image to scan
			ContainerName             string                 `json:"container_name"`         // For docker_image_scan: container to scan
			ScanAllImages             bool                   `json:"scan_all_images"`        // For docker_image_scan: scan all images
			OnDemandOnly              bool                   `json:"on_demand_only"`         // For set_compliance_on_demand_only (legacy)
			Mode                      string                 `json:"mode"`                   // For set_compliance_mode: "disabled", "on-demand", or "enabled"
			Config                    map[string]interface{} `json:"config"`                 // For apply_config: full config to apply
			// SSH proxy fields
			SessionID  string `json:"session_id"`  // SSH proxy session ID
			Host       string `json:"host"`        // SSH proxy target host
			Port       int    `json:"port"`        // SSH proxy target port
			Username   string `json:"username"`    // SSH username
			Password   string `json:"password"`    // SSH password
			PrivateKey string `json:"private_key"` // SSH private key
			Passphrase string `json:"passphrase"`  // SSH private key passphrase
			Terminal   string `json:"terminal"`    // Terminal type
			Cols       int    `json:"cols"`        // Terminal columns
			Rows       int    `json:"rows"`        // Terminal rows
			Data       string `json:"data"`        // SSH input data
			// run_patch fields
			PatchRunID   string   `json:"patch_run_id"`
			PatchType    string   `json:"patch_type"`
			PackageName  string   `json:"package_name"`
			PackageNames []string `json:"package_names"`
			DryRun       bool     `json:"dry_run"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			logger.WithError(err).WithField("message_bytes", len(data)).Warn("Failed to parse WebSocket message")
			continue
		}
		logger.WithField("type", logutil.Sanitize(payload.Type)).Debug("Parsed WebSocket message type")
		switch payload.Type {
		case "settings_update":
			logger.WithField("interval", payload.UpdateInterval).Info("settings_update received")
			out <- wsMsg{kind: "settings_update", interval: payload.UpdateInterval, complianceScanInterval: payload.ComplianceScanInterval, packageCacheRefreshMode: payload.PackageCacheRefreshMode, packageCacheRefreshMaxAge: payload.PackageCacheRefreshMaxAge}
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
		case "run_patch":
			if payload.PatchRunID == "" {
				logger.Warn("run_patch missing patch_run_id")
				continue
			}
			patchType := payload.PatchType
			if patchType == "" {
				patchType = "patch_all"
			}
			if patchType != "patch_all" && patchType != "patch_package" {
				logger.WithField("patch_type", logutil.Sanitize(patchType)).Warn("Invalid patch_type in run_patch")
				continue
			}
			var packageNames []string
			if len(payload.PackageNames) > 0 {
				for _, n := range payload.PackageNames {
					if validAptPackagePattern.MatchString(n) {
						packageNames = append(packageNames, n)
					} else {
						logger.WithError(fmt.Errorf("invalid package name")).WithField("package_name", logutil.Sanitize(n)).Warn("Invalid package name in run_patch package_names")
					}
				}
				if len(packageNames) == 0 {
					logger.Warn("run_patch package_names had no valid names")
					continue
				}
			} else if payload.PackageName != "" {
				if validAptPackagePattern.MatchString(payload.PackageName) {
					packageNames = []string{payload.PackageName}
				} else {
					logger.WithError(fmt.Errorf("invalid package name")).WithField("package_name", logutil.Sanitize(payload.PackageName)).Warn("Invalid package_name in run_patch")
					continue
				}
			} else if patchType == "patch_package" {
				logger.Warn("run_patch patch_package requires package_name or package_names")
				continue
			}
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"patch_run_id":  payload.PatchRunID,
				"patch_type":    patchType,
				"package_names": packageNames,
				"dry_run":       payload.DryRun,
			})).Info("run_patch received")
			out <- wsMsg{
				kind:         "run_patch",
				patchRunID:   payload.PatchRunID,
				patchType:    patchType,
				packageNames: packageNames,
				dryRun:       payload.DryRun,
			}
		case "update_notification":
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"version": payload.Version,
				"force":   payload.Force,
				"message": payload.Message,
			})).Info("update_notification received")
			out <- wsMsg{
				kind:    "update_notification",
				version: payload.Version,
				force:   payload.Force,
			}
		case "integration_toggle":
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"integration": payload.Integration,
				"enabled":     payload.Enabled,
			})).Info("integration_toggle received")
			out <- wsMsg{
				kind:               "integration_toggle",
				integrationName:    payload.Integration,
				integrationEnabled: payload.Enabled,
			}
		case "compliance_scan":
			// Validate profile ID to prevent command injection
			if err := validateProfileID(payload.ProfileID); err != nil {
				logger.WithError(err).WithField("profile_id", logutil.Sanitize(payload.ProfileID)).Warn("Invalid profile ID in compliance_scan message")
				continue
			}
			profileType := payload.ProfileType
			if profileType == "" {
				profileType = "all"
			}
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"profile_type":       profileType,
				"profile_id":         payload.ProfileID,
				"enable_remediation": payload.EnableRemediation,
			})).Info("compliance_scan received")
			out <- wsMsg{
				kind:                 "compliance_scan",
				profileType:          profileType,
				profileID:            payload.ProfileID,
				enableRemediation:    payload.EnableRemediation,
				fetchRemoteResources: payload.FetchRemoteResources,
				openscapEnabled:      payload.OpenSCAPEnabled,
				dockerBenchEnabled:   payload.DockerBenchEnabled,
			}
		case "compliance_scan_cancel":
			logger.Info("compliance_scan_cancel received")
			out <- wsMsg{kind: "compliance_scan_cancel"}
		case "patch_run_stop":
			if payload.PatchRunID == "" {
				logger.Warn("patch_run_stop missing patch_run_id")
				continue
			}
			logger.WithField("patch_run_id", logutil.Sanitize(payload.PatchRunID)).Info("patch_run_stop received")
			out <- wsMsg{kind: "patch_run_stop", patchRunID: payload.PatchRunID}
		case "upgrade_ssg":
			logger.WithField("version", payload.Version).Info("upgrade_ssg received from WebSocket")
			out <- wsMsg{kind: "upgrade_ssg", version: payload.Version}
			logger.Info("upgrade_ssg sent to message channel")
		case "install_scanner":
			logger.Info("install_scanner received from WebSocket")
			out <- wsMsg{kind: "install_scanner"}
		case "remediate_rule":
			// Validate rule ID to prevent command injection
			if err := validateRuleID(payload.RuleID); err != nil {
				logger.WithError(err).WithField("rule_id", logutil.Sanitize(payload.RuleID)).Warn("Invalid rule ID in remediate_rule message")
				continue
			}
			logger.WithField("rule_id", logutil.Sanitize(payload.RuleID)).Info("remediate_rule received")
			out <- wsMsg{kind: "remediate_rule", ruleID: payload.RuleID}
		case "docker_image_scan":
			// Validate Docker image and container names to prevent command injection
			if err := validateDockerImageName(payload.ImageName); err != nil {
				logger.WithError(err).WithField("image_name", logutil.Sanitize(payload.ImageName)).Warn("Invalid image name in docker_image_scan message")
				continue
			}
			if err := validateDockerContainerName(payload.ContainerName); err != nil {
				logger.WithError(err).WithField("container_name", logutil.Sanitize(payload.ContainerName)).Warn("Invalid container name in docker_image_scan message")
				continue
			}
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"image_name":      payload.ImageName,
				"container_name":  payload.ContainerName,
				"scan_all_images": payload.ScanAllImages,
			})).Info("docker_image_scan received")
			out <- wsMsg{
				kind:          "docker_image_scan",
				imageName:     payload.ImageName,
				containerName: payload.ContainerName,
				scanAllImages: payload.ScanAllImages,
			}
		case "set_compliance_mode":
			logger.WithField("mode", logutil.Sanitize(payload.Mode)).Info("set_compliance_mode received")
			// Validate mode
			validModes := map[string]bool{"disabled": true, "on-demand": true, "enabled": true}
			if !validModes[payload.Mode] {
				logger.WithField("mode", logutil.Sanitize(payload.Mode)).Warn("Invalid compliance mode, ignoring")
				continue
			}
			out <- wsMsg{
				kind:           "set_compliance_mode",
				complianceMode: payload.Mode,
			}
		case "apply_config":
			logger.Info("apply_config received")
			out <- wsMsg{kind: "apply_config", applyConfig: payload.Config}
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
					// Resolve the config path per-OS so Windows hosts see
					// C:\ProgramData\PatchMon\config.yml, not the Linux path.
					errorMsg := "SSH proxy is not enabled.\n\n" +
						"To enable SSH proxy, edit the file " + cfgManager.GetConfigFile() + " and add the following:\n\n" +
						"integrations:\n" +
						"    ssh-proxy-enabled: true\n\n" +
						"Note: This cannot be pushed from the server to the agent and should require you to manually do this for security reasons."
					sendSSHProxyError(wsConn, payload.SessionID, errorMsg)
				}
				continue
			}
			// Validate session ID
			if payload.SessionID == "" {
				logger.Warn("SSH proxy request missing session_id")
				continue
			}
			// Validate host
			if err := validateSSHProxyHost(payload.Host); err != nil {
				logger.WithError(err).WithField("host", payload.Host).Warn("Invalid SSH proxy host")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					sendSSHProxyError(wsConn, payload.SessionID, fmt.Sprintf("Invalid host: %v", err))
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
					sendSSHProxyError(wsConn, payload.SessionID, "Invalid port (must be 1-65535)")
				}
				continue
			}
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"session_id": payload.SessionID,
				"host":       payload.Host,
				"port":       payload.Port,
				"username":   payload.Username,
			})).Info("ssh_proxy received")
			out <- wsMsg{
				kind:               "ssh_proxy",
				sshProxySessionID:  payload.SessionID,
				sshProxyHost:       payload.Host,
				sshProxyPort:       payload.Port,
				sshProxyUsername:   payload.Username,
				sshProxyPassword:   payload.Password,
				sshProxyPrivateKey: payload.PrivateKey,
				sshProxyPassphrase: payload.Passphrase,
				sshProxyTerminal:   payload.Terminal,
				sshProxyCols:       payload.Cols,
				sshProxyRows:       payload.Rows,
			}
		case "ssh_proxy_input":
			if payload.SessionID == "" {
				logger.Warn("ssh_proxy_input missing session_id")
				continue
			}
			out <- wsMsg{
				kind:              "ssh_proxy_input",
				sshProxySessionID: payload.SessionID,
				sshProxyData:      payload.Data,
			}
		case "ssh_proxy_resize":
			if payload.SessionID == "" {
				logger.Warn("ssh_proxy_resize missing session_id")
				continue
			}
			out <- wsMsg{
				kind:              "ssh_proxy_resize",
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
				kind:              "ssh_proxy_disconnect",
				sshProxySessionID: payload.SessionID,
			}
		case "rdp_proxy":
			if !cfgManager.IsIntegrationEnabled("rdp-proxy-enabled") {
				logger.Warn("RDP proxy requested but not enabled in config.yml")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					// Resolve the config path per-OS so Windows hosts see
					// C:\ProgramData\PatchMon\config.yml, not the Linux path.
					errorMsg := "RDP proxy is not enabled.\n\n" +
						"To enable RDP proxy, edit the file " + cfgManager.GetConfigFile() + " and add:\n\n" +
						"integrations:\n" +
						"    rdp-proxy-enabled: true\n\n" +
						"Note: This cannot be pushed from the server and requires manual configuration for security."
					sendRDPProxyError(wsConn, payload.SessionID, errorMsg)
				}
				continue
			}
			if payload.SessionID == "" {
				logger.Warn("rdp_proxy request missing session_id")
				continue
			}
			rdpHost := payload.Host
			if rdpHost == "" {
				rdpHost = "localhost"
			}
			if err := validateSSHProxyHost(rdpHost); err != nil {
				logger.WithError(err).WithField("host", logutil.Sanitize(payload.Host)).Warn("Invalid RDP proxy host")
				globalWsConnMu.RLock()
				wsConn := globalWsConn
				globalWsConnMu.RUnlock()
				if wsConn != nil {
					sendRDPProxyError(wsConn, payload.SessionID, fmt.Sprintf("Invalid host: %v", err))
				}
				continue
			}
			port := payload.Port
			if port < 1 || port > 65535 {
				port = 3389
			}
			logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"session_id": payload.SessionID,
				"host":       rdpHost,
				"port":       port,
			})).Info("rdp_proxy received")
			out <- wsMsg{
				kind:              "rdp_proxy",
				rdpProxySessionID: payload.SessionID,
				rdpProxyHost:      rdpHost,
				rdpProxyPort:      port,
			}
		case "rdp_proxy_input":
			if payload.SessionID == "" {
				logger.Warn("rdp_proxy_input missing session_id")
				continue
			}
			out <- wsMsg{
				kind:              "rdp_proxy_input",
				rdpProxySessionID: payload.SessionID,
				rdpProxyData:      payload.Data,
			}
		case "rdp_proxy_disconnect":
			if payload.SessionID == "" {
				logger.Warn("rdp_proxy_disconnect missing session_id")
				continue
			}
			out <- wsMsg{
				kind:              "rdp_proxy_disconnect",
				rdpProxySessionID: payload.SessionID,
			}
		default:
			if payload.Type != "" && payload.Type != "connected" {
				logger.WithField("type", logutil.Sanitize(payload.Type)).Warn("Unknown WebSocket message type")
			}
		}
	}
}

// dryRunOutputIndicatesError returns true if the output contains dependency or
// resolution error messages. Used to distinguish "declined" (exit 1, success)
// from actual dependency/validation failures (exit 1, failure).
func dryRunOutputIndicatesError(output string) bool {
	lower := strings.ToLower(output)
	errorPatterns := []string{
		"error:", "error ", "unable to find", "unable to resolve", "no match for",
		"problem:", "transaction check error", "cannot install", "could not find",
		"failed to synchronize", "dependency resolution", "conflict",
		"no packages available to install", "pkg: no packages available",
		"cannot find package",
		// pacman-specific
		"error: failed to prepare transaction", "could not satisfy dependencies",
		"failed to commit transaction", "error: target not found",
		"unresolvable package conflicts",
	}
	for _, p := range errorPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isDryRunExit1Success returns true if err is exit code 1 with output and the
// output does not indicate a dependency/validation error. dnf/yum --assumeno
// and pkg -n return exit 1 when they "decline" the operation (expected for
// dry-run); we treat that as success. But if the output contains error messages
// (e.g. "Unable to resolve", "Problem:"), we treat it as failure.
func isDryRunExit1Success(err error, output string) bool {
	if output == "" {
		return false
	}
	if dryRunOutputIndicatesError(output) {
		return false
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

const freeBSDBasePackageName = "freebsd-base"

func getFreeBSDUpdateBinaryPath() (string, error) {
	if path, err := exec.LookPath("freebsd-update"); err == nil {
		return path, nil
	}
	for _, p := range []string{"/usr/sbin/freebsd-update"} {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("freebsd-update not found")
}

func freeBSDUpdateOutputHasPendingUpdates(output string) bool {
	if output == "" {
		return false
	}
	if strings.Contains(output, "No updates needed") || strings.Contains(output, "No updates are available") {
		return false
	}
	return strings.Contains(output, "will be updated") || strings.Contains(output, "will be installed")
}

// isOpenBSDSyspatchID reports whether name refers to a syspatch entry.
// This covers both the canonical single-entry name "syspatch" and any full
// patch ID in the form "NNN_component" (e.g. "015_tmppath") for forward compatibility.
func isOpenBSDSyspatchID(name string) bool {
	if name == "syspatch" {
		return true
	}
	// Patch IDs from syspatch -c have the form "NNN_component"
	if len(name) < 5 || name[3] != '_' {
		return false
	}
	return name[0] >= '0' && name[0] <= '9' &&
		name[1] >= '0' && name[1] <= '9' &&
		name[2] >= '0' && name[2] <= '9'
}

func splitFreeBSDPatchTargets(packageNames []string) ([]string, bool) {
	filtered := make([]string, 0, len(packageNames))
	includeBase := false
	for _, name := range packageNames {
		if strings.EqualFold(strings.TrimSpace(name), freeBSDBasePackageName) {
			includeBase = true
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered, includeBase
}

// runPatch runs package manager update and upgrade (patch_all) or install (patch_package).
// Supports apt-get (Debian/Ubuntu), dnf, yum (RHEL-based), pkg (FreeBSD), pacman (Arch),
// and windows (WinGet for applications + WUA COM API for OS updates).
// formatCmd returns a shell-style "$ command args..." line for display in output.
func formatCmd(name string, args ...string) string {
	parts := append([]string{name}, args...)
	return "$ " + strings.Join(parts, " ") + "\n"
}

// streamSink accumulates stdout+stderr bytes from streaming patch-run commands,
// appends them to a fullOutput builder, and periodically POSTs stage="progress"
// chunks to the server so the UI can render live output.
//
// Flushes happen on: (a) buffer reaching 1 KiB, (b) 250ms since last flush,
// (c) explicit Flush() call (e.g. at command boundaries).
type streamSink struct {
	client     *client.Client
	patchRunID string

	mu         sync.Mutex
	full       *strings.Builder
	pending    strings.Builder
	lastFlush  time.Time
	flushEvery time.Duration
	flushBytes int
}

func newStreamSink(httpClient *client.Client, patchRunID string, full *strings.Builder) *streamSink {
	return &streamSink{
		client:     httpClient,
		patchRunID: patchRunID,
		full:       full,
		lastFlush:  time.Now(),
		flushEvery: 250 * time.Millisecond,
		flushBytes: 1024,
	}
}

// Write records bytes into both the fullOutput builder and the pending flush
// buffer. It triggers a flush when thresholds are reached.
func (s *streamSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.full.Write(p)
	s.pending.Write(p)
	shouldFlush := s.pending.Len() >= s.flushBytes || time.Since(s.lastFlush) >= s.flushEvery
	s.mu.Unlock()
	if shouldFlush {
		s.Flush()
	}
	return len(p), nil
}

// WriteString is a convenience wrapper for appending a string.
func (s *streamSink) WriteString(str string) {
	_, _ = s.Write([]byte(str))
}

// Flush sends any buffered bytes to the server as a stage="progress" chunk.
// Uses a background context so a cancelled parent ctx does not prevent the
// final chunk from being sent.
func (s *streamSink) Flush() {
	s.mu.Lock()
	if s.pending.Len() == 0 {
		s.lastFlush = time.Now()
		s.mu.Unlock()
		return
	}
	chunk := s.pending.String()
	s.pending.Reset()
	s.lastFlush = time.Now()
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.client.SendPatchOutput(ctx, s.patchRunID, "progress", chunk, ""); err != nil {
		logger.WithError(err).WithField("patch_run_id", logutil.Sanitize(s.patchRunID)).Debug("Failed to send patch progress chunk")
	}
}

// runStreamingPatchStep executes a command, streaming its stdout+stderr into
// the provided sink. On context cancellation it sends SIGINT and allows
// WaitDelay for the process to clean up (rollbacks etc.) before forcing a kill.
func runStreamingPatchStep(ctx context.Context, sink *streamSink, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// os.Interrupt maps to SIGINT on Unix; on Windows the runtime emulates
		// a best-effort interrupt. Subsequent WaitDelay will SIGKILL if needed.
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 30 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	var wg sync.WaitGroup
	copyPipe := func(rc io.ReadCloser) {
		defer wg.Done()
		br := bufio.NewReader(rc)
		buf := make([]byte, 4096)
		for {
			n, readErr := br.Read(buf)
			if n > 0 {
				_, _ = sink.Write(buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}
	wg.Add(2)
	go copyPipe(stdout)
	go copyPipe(stderr)
	wg.Wait()

	waitErr := cmd.Wait()
	sink.Flush()
	return waitErr
}

// patchRunTrailer returns a short human-readable trailer the agent appends to
// the tail of a patch run's shell output so the live terminal view has a
// clear "this is the end" marker regardless of what the underlying package
// manager printed last. The same text is included in the authoritative
// fullOutput sent with the terminal stage, so users see the trailer whether
// they're reading the live WebSocket buffer or the persisted shell_output.
func patchRunTrailer(wasStopped bool, stepErr error, dryRun bool) string {
	ts := time.Now().UTC().Format(time.RFC3339)
	switch {
	case wasStopped:
		return fmt.Sprintf("\n--- Patch run stopped by user at %s ---\n", ts)
	case stepErr != nil:
		return fmt.Sprintf("\n--- Patch run failed at %s ---\n", ts)
	case dryRun:
		return fmt.Sprintf("\n--- Dry run completed at %s ---\n", ts)
	default:
		return fmt.Sprintf("\n--- Patch run completed at %s ---\n", ts)
	}
}

// When dryRun is true, simulates and sends dry_run_completed instead of completed.
func runPatch(patchRunID, patchType string, packageNames []string, dryRun bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Register cancel fn so the server can request an interrupt via "patch_run_stop".
	patchRunCancels.Store(patchRunID, cancel)
	defer patchRunCancels.Delete(patchRunID)

	httpClient := client.New(cfgManager, logger)
	packageMgr := packages.New(logger, packages.CacheRefreshConfig{
		Mode:   cfgManager.GetPackageCacheRefreshMode(),
		MaxAge: cfgManager.GetPackageCacheRefreshMaxAge(),
	})
	pkgManager := packageMgr.DetectPackageManager()

	if pkgManager == "windows" {
		return runPatchWindows(ctx, httpClient, patchRunID, patchType, packageNames, dryRun)
	}

	if pkgManager != "apt" && pkgManager != "dnf" && pkgManager != "yum" && pkgManager != "pkg" && pkgManager != "pacman" && pkgManager != "pkg_info" {
		errMsg := fmt.Sprintf("package manager %q not supported for patching (apt, dnf, yum, pkg, pacman required)", pkgManager)
		_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	var env []string
	var upgradeBin string
	var freeBSDUpdateBin string
	freeBSDPkgTargets := packageNames
	includeFreeBSDBase := pkgManager == "pkg" && patchType == "patch_all"
	switch pkgManager {
	case "apt":
		if _, err := exec.LookPath("apt-get"); err != nil {
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", "apt-get not found: not a Debian/Ubuntu system or apt not installed")
			return fmt.Errorf("apt-get not found: %w", err)
		}
		env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		upgradeBin = "apt-get"
	case "pkg":
		freeBSDPkgTargets, includeFreeBSDBase = splitFreeBSDPatchTargets(packageNames)
		upgradeBin = packages.GetPkgBinaryPath()
		env = append(os.Environ(), "ASSUME_ALWAYS_YES=YES", "PAGER=cat")
		if includeFreeBSDBase {
			var err error
			freeBSDUpdateBin, err = getFreeBSDUpdateBinaryPath()
			if err != nil {
				_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", "freebsd-update not found: cannot patch FreeBSD base system")
				return fmt.Errorf("freebsd-update not found: %w", err)
			}
		}
	case "pkg_info":
		// OpenBSD: pkg_add -u for binary packages, syspatch for base patches
		if _, err := exec.LookPath("pkg_add"); err != nil {
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", "pkg_add not found: not an OpenBSD system")
			return fmt.Errorf("pkg_add not found: %w", err)
		}
		upgradeBin = "pkg_add"
	case "pacman":
		if _, err := exec.LookPath("pacman"); err != nil {
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", "pacman not found: Arch Linux package manager not installed")
			return fmt.Errorf("pacman not found: %w", err)
		}
		upgradeBin = "pacman"
	default: // dnf, yum
		if _, err := exec.LookPath(pkgManager); err != nil {
			errMsg := fmt.Sprintf("%s not found: RHEL/Fedora package manager not installed", pkgManager)
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", errMsg)
			return fmt.Errorf("%s not found: %w", pkgManager, err)
		}
		upgradeBin = pkgManager
	}

	if err := httpClient.SendPatchOutput(ctx, patchRunID, "started", "", ""); err != nil {
		logger.WithError(err).Warn("Failed to send patch started to server")
	}

	var fullOutput strings.Builder
	fullOutput.Grow(8192)
	sink := newStreamSink(httpClient, patchRunID, &fullOutput)

	// runStep streams a single package-manager command's output and returns
	// (terminalError, shouldAbort). If isDryRunStep is true, exit-1 from tools
	// that use it to signal "changes pending" is accepted as success.
	runStep := func(isDryRunStep bool, errTag, errFmt, name string, args ...string) (error, bool) {
		sink.WriteString(formatCmd(name, args...))
		sink.Flush()
		err := runStreamingPatchStep(ctx, sink, env, name, args...)
		if err == nil {
			return nil, false
		}
		if isDryRunStep && isDryRunExit1Success(err, fullOutput.String()) {
			return nil, false
		}
		logger.WithError(err).Warn(errTag + " failed")
		sink.WriteString(fmt.Sprintf("\n[%s error] %s\n", errTag, err.Error()))
		sink.Flush()
		return fmt.Errorf(errFmt, err), true
	}

	// runSyspatch runs syspatch and handles exit code 2, which means syspatch
	// updated itself and must be re-run to install remaining patches.
	// Returns (error, shouldAbort).
	runSyspatch := func(args ...string) (error, bool) {
		sink.WriteString(formatCmd("syspatch", args...))
		sink.Flush()
		err := runStreamingPatchStep(ctx, sink, env, "syspatch", args...)
		if err == nil {
			return nil, false
		}
		// Exit code 2: syspatch updated itself — re-run to install remaining patches
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			sink.WriteString("\n[syspatch] Updated itself, re-running to install remaining patches...\n")
			sink.Flush()
			sink.WriteString(formatCmd("syspatch", args...))
			sink.Flush()
			err2 := runStreamingPatchStep(ctx, sink, env, "syspatch", args...)
			if err2 == nil {
				return nil, false
			}
			logger.WithError(err2).Warn("syspatch (re-run) failed")
			sink.WriteString(fmt.Sprintf("\n[syspatch error] %s\n", err2.Error()))
			sink.Flush()
			return fmt.Errorf("syspatch failed: %w", err2), true
		}
		logger.WithError(err).Warn("syspatch failed")
		sink.WriteString(fmt.Sprintf("\n[syspatch error] %s\n", err.Error()))
		sink.Flush()
		return fmt.Errorf("syspatch failed: %w", err), true
	}

	var stepErr error

	if includeFreeBSDBase {
		fetchStart := fullOutput.Len()
		if err, abort := runStep(false, "freebsd-update fetch", "freebsd-update fetch failed: %w", freeBSDUpdateBin, "fetch", "--not-running-from-cron"); abort {
			stepErr = err
		}
		fetchOutput := ""
		if fullOutput.Len() > fetchStart {
			fetchOutput = fullOutput.String()[fetchStart:]
		}
		if stepErr == nil && !dryRun && freeBSDUpdateOutputHasPendingUpdates(fetchOutput) {
			if err, abort := runStep(false, "freebsd-update install", "freebsd-update install failed: %w", freeBSDUpdateBin, "install"); abort {
				stepErr = err
			}
		}
	}

	needsPkgTransaction := pkgManager != "pkg" || patchType == "patch_all" || len(freeBSDPkgTargets) > 0
	if stepErr == nil && needsPkgTransaction {
		// Update package cache
		switch pkgManager {
		case "apt":
			if err, abort := runStep(false, "apt-get update", "apt-get update failed: %w", "apt-get", "update", "-qq"); abort {
				stepErr = err
			}
		case "pkg":
			if err, abort := runStep(false, "pkg update", "pkg update failed: %w", upgradeBin, "update"); abort {
				stepErr = err
			}
		case "pkg_info":
			// OpenBSD: no separate cache update step — pkg_add fetches on demand
		case "pacman":
			if err, abort := runStep(false, "pacman refresh", "pacman -Sy failed: %w", "pacman", "-Sy", "--noconfirm"); abort {
				stepErr = err
			}
		default:
			if err, abort := runStep(false, upgradeBin+" makecache", upgradeBin+" makecache failed: %w", upgradeBin, "makecache", "-q"); abort {
				stepErr = err
			}
		}
	}

	if stepErr == nil {
		if patchType == "patch_all" {
			switch pkgManager {
			case "apt":
				if dryRun {
					if err, abort := runStep(false, "apt-get -s upgrade", "apt-get -s upgrade failed: %w", "apt-get", "-s", "upgrade"); abort {
						stepErr = err
					}
				} else {
					if err, abort := runStep(false, "apt-get upgrade", "apt-get upgrade failed: %w", "apt-get", "upgrade", "-y"); abort {
						stepErr = err
					}
				}
			case "pkg":
				if dryRun {
					if err, abort := runStep(true, "pkg upgrade -n", "pkg upgrade -n failed: %w", upgradeBin, "upgrade", "-n"); abort {
						stepErr = err
					}
				} else {
					if err, abort := runStep(false, "pkg upgrade", "pkg upgrade failed: %w", upgradeBin, "upgrade", "-y"); abort {
						stepErr = err
					}
				}
			case "pkg_info":
				// OpenBSD patch_all: run syspatch for base patches, then pkg_add -u for all binary packages
				if dryRun {
					if err, abort := runStep(true, "syspatch -c", "syspatch -c failed: %w", "syspatch", "-c"); abort {
						stepErr = err
					}
					if stepErr == nil {
						if err, abort := runStep(false, "pkg_add -u -n", "pkg_add -u -n failed: %w", "pkg_add", "-u", "-n"); abort {
							stepErr = err
						}
					}
				} else {
					if err, abort := runSyspatch(); abort {
						stepErr = err
					}
					if stepErr == nil {
						if err, abort := runStep(false, "pkg_add -u", "pkg_add -u failed: %w", "pkg_add", "-u"); abort {
							stepErr = err
						}
					}
				}
			case "pacman":
				if dryRun {
					if err, abort := runStep(true, "pacman -Syu -p", "pacman -Syu -p failed: %w", "pacman", "-Syu", "-p"); abort {
						stepErr = err
					}
				} else {
					if err, abort := runStep(false, "pacman -Syu", "pacman -Syu failed: %w", "pacman", "-Syu", "--noconfirm"); abort {
						stepErr = err
					}
				}
			default: // dnf, yum
				if dryRun {
					if err, abort := runStep(true, upgradeBin+" upgrade --assumeno", upgradeBin+" upgrade --assumeno failed: %w", upgradeBin, "upgrade", "--assumeno"); abort {
						stepErr = err
					}
				} else {
					if err, abort := runStep(false, upgradeBin+" upgrade", upgradeBin+" upgrade failed: %w", upgradeBin, "upgrade", "-y"); abort {
						stepErr = err
					}
				}
			}
		} else {
			if len(packageNames) == 0 {
				sink.Flush()
				_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", fullOutput.String(), "package_names required for patch_package")
				return fmt.Errorf("package_names required for patch_package")
			}
			switch pkgManager {
			case "apt":
				if dryRun {
					args := append([]string{"-s", "install"}, packageNames...)
					if err, abort := runStep(false, "apt-get -s install", "apt-get -s install failed: %w", "apt-get", args...); abort {
						stepErr = err
					}
				} else {
					args := append([]string{"install", "-y"}, packageNames...)
					if err, abort := runStep(false, "apt-get install", "apt-get install failed: %w", "apt-get", args...); abort {
						stepErr = err
					}
				}
			case "pkg":
				if len(freeBSDPkgTargets) > 0 {
					if dryRun {
						args := append([]string{"install", "-n"}, freeBSDPkgTargets...)
						if err, abort := runStep(true, "pkg install -n", "pkg install -n failed: %w", upgradeBin, args...); abort {
							stepErr = err
						}
					} else {
						args := append([]string{"install", "-y"}, freeBSDPkgTargets...)
						if err, abort := runStep(false, "pkg install", "pkg install failed: %w", upgradeBin, args...); abort {
							stepErr = err
						}
					}
				}
			case "pacman":
				if dryRun {
					args := append([]string{"-S", "-p"}, packageNames...)
					if err, abort := runStep(true, "pacman -S -p", "pacman -S -p failed: %w", "pacman", args...); abort {
						stepErr = err
					}
				} else {
					args := append([]string{"-S", "--noconfirm"}, packageNames...)
					if err, abort := runStep(false, "pacman -S", "pacman -S failed: %w", "pacman", args...); abort {
						stepErr = err
					}
				}
			case "pkg_info":
				// OpenBSD patch_package: syspatch for base patches, pkg_add -u for binary packages.
				// Syspatch entries have Names that are patch IDs: "NNN_component" (e.g. "015_tmppath").
				openBSDSyspatches := []string{}
				openBSDPkgs := []string{}
				for _, name := range packageNames {
					if isOpenBSDSyspatchID(name) {
						openBSDSyspatches = append(openBSDSyspatches, name)
					} else {
						openBSDPkgs = append(openBSDPkgs, name)
					}
				}
				if len(openBSDSyspatches) > 0 {
					if dryRun {
						if err, abort := runStep(true, "syspatch -c", "syspatch -c failed: %w", "syspatch", "-c"); abort {
							stepErr = err
						}
					} else {
						if err, abort := runSyspatch(); abort {
							stepErr = err
						}
					}
				}
				if stepErr == nil && len(openBSDPkgs) > 0 {
					if dryRun {
						args := append([]string{"-u", "-n"}, openBSDPkgs...)
						if err, abort := runStep(false, "pkg_add -u -n", "pkg_add -u -n failed: %w", "pkg_add", args...); abort {
							stepErr = err
						}
					} else {
						args := append([]string{"-u"}, openBSDPkgs...)
						if err, abort := runStep(false, "pkg_add -u", "pkg_add -u failed: %w", "pkg_add", args...); abort {
							stepErr = err
						}
					}
				}
			default: // dnf, yum
				if dryRun {
					args := append([]string{"install", "--assumeno"}, packageNames...)
					if err, abort := runStep(true, upgradeBin+" install --assumeno", upgradeBin+" install --assumeno failed: %w", upgradeBin, args...); abort {
						stepErr = err
					}
				} else {
					args := append([]string{"install", "-y"}, packageNames...)
					if err, abort := runStep(false, upgradeBin+" install", upgradeBin+" install failed: %w", upgradeBin, args...); abort {
						stepErr = err
					}
				}
			}
		}
	}

	// Drain any remaining buffered output before deciding the final stage.
	sink.Flush()

	_, wasStopped := patchRunStopped.LoadAndDelete(patchRunID)

	// Append a short human-readable trailer so users watching the live
	// terminal can tell at a glance that the run has finished instead of
	// guessing whether the last package-manager line is really the end.
	// The trailer is streamed as a final progress chunk AND included in the
	// authoritative shell_output blob we send with the terminal stage, so
	// the same text is present whether the frontend is showing the live
	// buffer or the persisted one.
	trailer := patchRunTrailer(wasStopped, stepErr, dryRun)
	sink.WriteString(trailer)
	sink.Flush()

	// Use a background context for the final status send so a cancelled
	// ctx (from stop) still allows the cancellation record to reach the server.
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer finalCancel()

	switch {
	case wasStopped:
		if err := httpClient.SendPatchOutput(finalCtx, patchRunID, "cancelled", fullOutput.String(), "stopped by user"); err != nil {
			logger.WithError(err).Warn("Failed to send patch cancelled output to server")
		}
	case stepErr != nil:
		if err := httpClient.SendPatchOutput(finalCtx, patchRunID, "failed", fullOutput.String(), stepErr.Error()); err != nil {
			logger.WithError(err).Warn("Failed to send patch failed output to server")
		}
		return stepErr
	default:
		stage := "completed"
		if dryRun {
			stage = "dry_run_completed"
		}
		if err := httpClient.SendPatchOutput(finalCtx, patchRunID, stage, fullOutput.String(), ""); err != nil {
			logger.WithError(err).Warn("Failed to send patch output to server")
			return err
		}
	}

	// Post-patch inventory report: runs after success AND after user-triggered
	// stop (a cancelled run may leave packages in a partially-changed state).
	if !dryRun && (wasStopped || stepErr == nil) {
		logger.Info("Sending post-patch report to refresh package lists...")
		reportDone := make(chan error, 1)
		go func() { reportDone <- sendReport(false) }()
		select {
		case err := <-reportDone:
			if err != nil {
				logger.WithError(err).Warn("Post-patch report failed")
			} else {
				logger.Info("Post-patch report sent successfully")
			}
		case <-time.After(2 * time.Minute):
			logger.Warn("Post-patch report timed out after 2 minutes; will retry on next scheduled report")
		}
	}

	if wasStopped {
		return fmt.Errorf("patch run stopped by user")
	}
	return nil
}

// runPatchWindows handles patching on Windows hosts.
// For patch_all: installs all approved WUA updates (by GUID from server) + upgrades all WinGet apps.
// For patch_package: routes by package name - "KB..." prefix -> WUA, otherwise -> WinGet upgrade.
func runPatchWindows(ctx context.Context, httpClient *client.Client, patchRunID, patchType string, packageNames []string, dryRun bool) error {
	patcher := packages.NewWindowsPatcher()
	var fullOutput strings.Builder

	if err := httpClient.SendPatchOutput(ctx, patchRunID, "started", "", ""); err != nil {
		logger.WithError(err).Warn("Failed to send patch started to server")
	}

	if patchType == "patch_all" {
		// Step 1: WUA - install approved OS/KB updates
		guids, err := httpClient.GetApprovedWindowsUpdateGUIDs(ctx)
		if err != nil {
			logger.WithError(err).Warn("Could not fetch approved Windows Update GUIDs; skipping WUA step")
		}
		if len(guids) > 0 {
			fmt.Fprintf(&fullOutput, "[Windows Update] Installing %d approved update(s)...\n", len(guids))
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "progress", fullOutput.String(), "")
			for _, guid := range guids {
				out, err := patcher.InstallWindowsUpdate(ctx, guid)
				fmt.Fprintf(&fullOutput, "  [%s] %s\n", guid, out)
				success := err == nil && !packages.IsSuperseded(out)
				result := client.WindowsUpdateResult{GUID: guid, Success: success}
				if err != nil {
					result.Error = err.Error()
				}
				_ = httpClient.SendWindowsUpdateResult(ctx, patchRunID, result)
				_ = httpClient.SendPatchOutput(ctx, patchRunID, "progress", fullOutput.String(), "")
			}
		}

		// Step 2: WinGet - upgrade all applications
		fullOutput.WriteString("\n[WinGet] Upgrading applications...\n")
		_ = httpClient.SendPatchOutput(ctx, patchRunID, "progress", fullOutput.String(), "")
		wingetOut, wingetErr := patcher.WinGetUpgradeAll(ctx, dryRun)
		fullOutput.WriteString(wingetOut)
		fullOutput.WriteString("\n")
		if wingetErr != nil {
			logger.WithError(wingetErr).Warn("winget upgrade --all had errors (non-fatal)")
		}

		// Step 3: report reboot status
		needsReboot := packages.RebootRequired()
		_ = httpClient.SendWindowsRebootStatus(ctx, patchRunID, needsReboot)
		if needsReboot {
			fullOutput.WriteString("\n[Reboot Required] A system restart is needed to complete the update installation.\n")
		}
	} else {
		// patch_package: each name is either a KB/GUID (WUA) or a WinGet package ID
		if len(packageNames) == 0 {
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "failed", "", "package_names required for patch_package")
			return fmt.Errorf("package_names required for patch_package")
		}
		for _, name := range packageNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// Treat as WUA GUID if it looks like a UUID (36 chars with dashes), or KB prefix
			isWUA := isWindowsUpdateIdentifier(name)
			if isWUA {
				fmt.Fprintf(&fullOutput, "[Windows Update] Installing %s...\n", name)
				out, err := patcher.InstallWindowsUpdate(ctx, name)
				fullOutput.WriteString(out + "\n")
				success := err == nil && !packages.IsSuperseded(out)
				result := client.WindowsUpdateResult{GUID: name, Success: success}
				if err != nil {
					result.Error = err.Error()
				}
				_ = httpClient.SendWindowsUpdateResult(ctx, patchRunID, result)
			} else {
				fmt.Fprintf(&fullOutput, "[WinGet] Upgrading %s...\n", name)
				out, err := patcher.WinGetUpgradePackage(ctx, name, dryRun)
				fullOutput.WriteString(out + "\n")
				if err != nil {
					logger.WithError(err).WithField("package", name).Warn("winget upgrade failed (non-fatal)")
				}
			}
			_ = httpClient.SendPatchOutput(ctx, patchRunID, "progress", fullOutput.String(), "")
		}

		needsReboot := packages.RebootRequired()
		_ = httpClient.SendWindowsRebootStatus(ctx, patchRunID, needsReboot)
		if needsReboot {
			fullOutput.WriteString("\n[Reboot Required] A system restart is needed.\n")
		}
	}

	_, wasStopped := patchRunStopped.LoadAndDelete(patchRunID)

	// Use a background context for the final status send so a cancelled
	// ctx still allows the final record to reach the server.
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer finalCancel()

	stage := "completed"
	if wasStopped {
		stage = "cancelled"
	} else if dryRun {
		stage = "dry_run_completed"
	}
	errMsg := ""
	if wasStopped {
		errMsg = "stopped by user"
	}

	// Human-readable trailer so the browser's live terminal has a clear
	// "this is the end" marker. Streamed as a progress chunk first so it
	// reaches the WS hub, then folded into the authoritative terminal blob.
	trailer := patchRunTrailer(wasStopped, nil, dryRun)
	fullOutput.WriteString(trailer)
	_ = httpClient.SendPatchOutput(ctx, patchRunID, "progress", trailer, "")

	if err := httpClient.SendPatchOutput(finalCtx, patchRunID, stage, fullOutput.String(), errMsg); err != nil {
		logger.WithError(err).Warn("Failed to send Windows patch output to server")
		return err
	}

	if !dryRun {
		logger.Info("Sending post-patch report to refresh package lists...")
		reportDone := make(chan error, 1)
		go func() { reportDone <- sendReport(false) }()
		select {
		case err := <-reportDone:
			if err != nil {
				logger.WithError(err).Warn("Post-patch report failed")
			} else {
				logger.Info("Post-patch report sent successfully")
			}
		case <-time.After(2 * time.Minute):
			logger.Warn("Post-patch report timed out after 2 minutes; will retry on next scheduled report")
		}
	}

	if wasStopped {
		return fmt.Errorf("patch run stopped by user")
	}
	return nil
}

// isWindowsUpdateIdentifier returns true if the name looks like a WUA GUID (UUID format) or KB article ID.
func isWindowsUpdateIdentifier(name string) bool {
	if strings.HasPrefix(strings.ToUpper(name), "KB") {
		return true
	}
	// UUID format: 8-4-4-4-12 hex digits
	if len(name) == 36 && name[8] == '-' && name[13] == '-' && name[18] == '-' && name[23] == '-' {
		return true
	}
	return false
}

// applyConfig applies a full config update from the server and restarts the service.
func applyConfig(cfg map[string]interface{}) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	logger.Info("Applying configuration from server...")

	// Apply docker
	if v, ok := cfg["docker"]; ok {
		if b, ok := v.(bool); ok {
			if err := cfgManager.SetIntegrationEnabled("docker", b); err != nil {
				return fmt.Errorf("set docker: %w", err)
			}
			logger.WithField("enabled", b).Info("Docker integration updated")
		}
	}

	// Apply compliance (can be bool, string "on-demand", or nested map)
	complianceVal := cfg["compliance"]
	if complianceVal != nil {
		var mode config.ComplianceMode
		modeVal := complianceVal
		if nm, ok := complianceVal.(map[string]interface{}); ok {
			modeVal = nm["enabled"]
		}
		switch val := modeVal.(type) {
		case bool:
			if val {
				mode = config.ComplianceEnabled
			} else {
				mode = config.ComplianceDisabled
			}
		case string:
			switch val {
			case "on-demand", "on_demand":
				mode = config.ComplianceOnDemand
			case "true":
				mode = config.ComplianceEnabled
			case "false":
				mode = config.ComplianceDisabled
			default:
				mode = config.ComplianceOnDemand
			}
		default:
			logger.WithField("compliance", logutil.Sanitize(fmt.Sprintf("%v", complianceVal))).Warn("Unknown compliance value type, skipping")
		}
		if mode != "" {
			if err := cfgManager.SetComplianceMode(mode); err != nil {
				return fmt.Errorf("set compliance mode: %w", err)
			}
			logger.WithField("mode", logutil.Sanitize(string(mode))).Info("Compliance mode updated")
		}
	}

	// Apply compliance scanner toggles (flat keys or nested under compliance)
	openscap := cfgManager.GetComplianceOpenscapEnabled()
	dockerBench := cfgManager.GetComplianceDockerBenchEnabled()
	if v, ok := cfg["compliance_openscap_enabled"]; ok {
		if b, ok := v.(bool); ok {
			openscap = b
		}
	} else if nm, ok := cfg["compliance"].(map[string]interface{}); ok {
		if v, ok := nm["openscap_enabled"]; ok {
			if b, ok := v.(bool); ok {
				openscap = b
			}
		}
	}
	if v, ok := cfg["compliance_docker_bench_enabled"]; ok {
		if b, ok := v.(bool); ok {
			dockerBench = b
		}
	} else if nm, ok := cfg["compliance"].(map[string]interface{}); ok {
		if v, ok := nm["docker_bench_enabled"]; ok {
			if b, ok := v.(bool); ok {
				dockerBench = b
			}
		}
	}
	if err := cfgManager.SetComplianceScanners(openscap, dockerBench); err != nil {
		return fmt.Errorf("set compliance scanners: %w", err)
	}
	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{"openscap": openscap, "docker_bench": dockerBench})).Info("Compliance scanner toggles updated")

	logger.Info("Config updated, restarting patchmon-agent service...")
	return restartService("", "")
}

// toggleIntegration toggles an integration on or off and restarts the service
func toggleIntegration(integrationName string, enabled bool) error {
	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"integration": integrationName,
		"enabled":     enabled,
	})).Info("Toggling integration")

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

			events := make([]models.InstallEvent, 0, 8)
			addEvent := func(step, status, message string) {
				events = append(events, models.InstallEvent{
					Step:      step,
					Status:    status,
					Message:   message,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}
			sendEvt := func(oStatus, msg string, si *models.ComplianceScannerDetails) {
				_ = httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
					Integration:   "compliance",
					Enabled:       true,
					Status:        oStatus,
					Message:       msg,
					Components:    components,
					InstallEvents: events,
					ScannerInfo:   si,
				})
			}

			// Step: Detect OS
			addEvent("detect_os", "in_progress", "Detecting operating system...")
			sendEvt(overallStatus, "Detecting operating system...", nil)

			openscapScanner := compliance.NewOpenSCAPScanner(logger)
			osInfo := openscapScanner.GetOSInfo()
			osDesc := fmt.Sprintf("%s %s (%s)", osInfo.Name, osInfo.Version, osInfo.Family)
			if osInfo.Name == "" {
				osDesc = "unknown OS"
			}
			events[len(events)-1] = models.InstallEvent{Step: "detect_os", Status: "done", Message: fmt.Sprintf("Detected %s", osDesc), Timestamp: events[len(events)-1].Timestamp}

			// Step: Install OpenSCAP
			addEvent("install_openscap", "in_progress", "Installing OpenSCAP packages...")
			sendEvt(overallStatus, "Installing OpenSCAP packages...", nil)

			if err := openscapScanner.EnsureInstalled(); err != nil {
				logger.WithError(err).Warn("Failed to install OpenSCAP (will try again on next scan)")
				components["openscap"] = "failed"
				events[len(events)-1] = models.InstallEvent{Step: "install_openscap", Status: "failed", Message: fmt.Sprintf("OpenSCAP installation failed: %s", err.Error()), Timestamp: events[len(events)-1].Timestamp}
			} else {
				logger.Info("OpenSCAP installed successfully")
				components["openscap"] = "ready"
				events[len(events)-1] = models.InstallEvent{Step: "install_openscap", Status: "done", Message: "OpenSCAP packages installed successfully", Timestamp: events[len(events)-1].Timestamp}
			}

			// Step: Docker Bench
			dockerIntegrationEnabled := cfgManager.IsIntegrationEnabled("docker")
			if dockerIntegrationEnabled {
				addEvent("docker_bench", "in_progress", "Pre-pulling Docker Bench image...")
				sendEvt(overallStatus, "Pre-pulling Docker Bench image...", nil)

				dockerBenchScanner := compliance.NewDockerBenchScanner(logger)
				if dockerBenchScanner.IsAvailable() {
					if err := dockerBenchScanner.EnsureInstalled(); err != nil {
						logger.WithError(err).Warn("Failed to pre-pull Docker Bench image (will pull on first scan)")
						components["docker-bench"] = "failed"
						events[len(events)-1] = models.InstallEvent{Step: "docker_bench", Status: "failed", Message: fmt.Sprintf("Docker Bench image pull failed: %s", err.Error()), Timestamp: events[len(events)-1].Timestamp}
					} else {
						logger.Info("Docker Bench image pulled successfully")
						components["docker-bench"] = "ready"
						events[len(events)-1] = models.InstallEvent{Step: "docker_bench", Status: "done", Message: "Docker Bench image pulled successfully", Timestamp: events[len(events)-1].Timestamp}
					}
				} else {
					components["docker-bench"] = "unavailable"
					events[len(events)-1] = models.InstallEvent{Step: "docker_bench", Status: "skipped", Message: "Docker not available on this host", Timestamp: events[len(events)-1].Timestamp}
				}

				oscapDockerScanner := compliance.NewOscapDockerScanner(logger)
				if !oscapDockerScanner.IsAvailable() {
					if err := oscapDockerScanner.EnsureInstalled(); err != nil {
						errMsg := err.Error()
						if strings.Contains(errMsg, "not available") || strings.Contains(errMsg, "not supported") {
							logger.WithError(err).Info("oscap-docker not available on this platform")
							components["oscap-docker"] = "unavailable"
						} else {
							logger.WithError(err).Warn("Failed to install oscap-docker")
							components["oscap-docker"] = "failed"
						}
					} else {
						logger.Info("oscap-docker installed successfully")
						components["oscap-docker"] = "ready"
					}
				} else {
					components["oscap-docker"] = "ready"
				}
			} else {
				logger.Debug("Docker integration not enabled, skipping Docker Bench and oscap-docker setup")
				addEvent("docker_bench", "skipped", "Docker integration not enabled, skipping Docker Bench setup")
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
			addEvent("complete", func() string {
				if allReady {
					return "done"
				}
				return "failed"
			}(), statusMessage)

			scannerDetails := openscapScanner.GetScannerDetails()
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

			sendEvt(overallStatus, statusMessage, scannerDetails)
			return nil
		}

		logger.Info("Compliance disabled - removing tools...")
		overallStatus = "removing"

		// Send initial "removing" status
		if err := httpClient.SendIntegrationSetupStatus(ctx, &models.IntegrationSetupStatus{
			Integration: "compliance",
			Enabled:     false,
			Status:      overallStatus,
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
		logger.WithField("output", logutil.Sanitize(string(output))).Debug("Service restart command completed")
		logger.Info("Service restarted successfully")
		return nil
	} else if runtime.GOOS == "freebsd" {
		// FreeBSD / pfSense: use rc.d helper script (same approach as version_update.go)
		logger.Debug("Detected FreeBSD, scheduling service restart via helper script")
		if err := os.MkdirAll("/etc/patchmon", 0700); err != nil {
			logger.WithError(err).Warn("Failed to create /etc/patchmon directory, will try anyway")
		}
		helperScript := `#!/bin/sh
sleep 2
# Prefer service, fallback to rc.d script (pfSense, minimal env)
if [ -x /usr/sbin/service ]; then
    /usr/sbin/service patchmon_agent restart 2>/dev/null || /usr/sbin/service patchmon_agent start 2>/dev/null
else
    /usr/local/etc/rc.d/patchmon_agent restart 2>/dev/null || /usr/local/etc/rc.d/patchmon_agent start 2>/dev/null
fi
rm -f "$0"
`
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			randomBytes = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
		}
		helperPath := filepath.Join("/etc/patchmon", fmt.Sprintf("restart-%s.sh", hex.EncodeToString(randomBytes)))
		dirInfo, err := os.Lstat("/etc/patchmon")
		if err == nil && dirInfo.Mode()&os.ModeSymlink != 0 {
			logger.Warn("Security: /etc/patchmon is a symlink, refusing to create helper script")
			os.Exit(0)
		}
		file, err := os.OpenFile(helperPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			logger.WithError(err).Warn("Failed to create restart helper script, exiting to let daemon -r respawn")
			os.Exit(0)
		}
		if _, err := file.WriteString(helperScript); err != nil {
			_ = file.Close()
			_ = os.Remove(helperPath)
			os.Exit(0)
		}
		_ = file.Close()
		fileInfo, err := os.Lstat(helperPath)
		if err != nil || fileInfo.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(helperPath)
			os.Exit(0)
		}
		var cmd *exec.Cmd
		if _, nohupErr := exec.LookPath("nohup"); nohupErr == nil {
			cmd = exec.Command("nohup", helperPath)
		} else {
			cmd = exec.Command("/bin/sh", helperPath)
		}
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.SysProcAttr = sysProcAttrForDetach()
		if err := cmd.Start(); err != nil {
			_ = os.Remove(helperPath)
			logger.WithError(err).Warn("Failed to start restart helper, exiting to let daemon -r respawn")
			os.Exit(0)
		}
		logger.Info("Scheduled service restart via helper script (FreeBSD), exiting now...")
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
		return nil
	} else if _, err := exec.LookPath("rc-service"); err == nil {
		// OpenRC is available (Alpine Linux)
		// Since we're running inside the service, we can't stop ourselves directly
		// Instead, we'll create a helper script that runs after we exit
		// Note: The OpenRC service file uses supervisor=supervise-daemon which will
		// automatically restart the agent if the helper script approach fails.
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
			// supervise-daemon will restart the agent automatically
			os.Exit(0)
		}

		// SECURITY: Use O_EXCL to atomically create file (fail if exists - prevents race conditions)
		file, err := os.OpenFile(helperPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			logger.WithError(err).Warn("Failed to create restart helper script, supervise-daemon will handle restart")
			// Fall through to exit approach - supervise-daemon will restart
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
				// Fall through to exit approach - supervise-daemon will restart
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
					// supervise-daemon will restart the agent automatically
					os.Exit(0)
				}

				// Execute the helper script in background (detached from current process)
				// Try nohup first, fall back to direct /bin/sh with session detachment
				var cmd *exec.Cmd
				if _, nohupErr := exec.LookPath("nohup"); nohupErr == nil {
					cmd = exec.Command("nohup", helperPath)
				} else {
					logger.Debug("nohup not available, using direct /bin/sh execution with session detachment")
					cmd = exec.Command("/bin/sh", helperPath)
				}
				cmd.Stdout = nil
				cmd.Stderr = nil
				// Create a new session to fully detach the child process (Linux only)
				cmd.SysProcAttr = sysProcAttrForDetach()
				if err := cmd.Start(); err != nil {
					logger.WithError(err).Warn("Failed to start restart helper script, supervise-daemon will handle restart")
					// Clean up script
					if removeErr := os.Remove(helperPath); removeErr != nil {
						logger.WithError(removeErr).Debug("Failed to remove helper script")
					}
					// Fall through to exit approach - supervise-daemon will restart
				} else {
					logger.Info("Scheduled service restart via helper script, exiting now...")
					// Give the helper script a moment to start
					time.Sleep(500 * time.Millisecond)
					// Exit gracefully - the helper script will restart the service
					os.Exit(0)
				}
			}
		}

		// Fallback: If helper script approach failed, just exit
		// OpenRC with supervisor=supervise-daemon will automatically restart the agent after respawn_delay
		logger.Info("Exiting to allow OpenRC supervise-daemon to restart service with updated config...")
		os.Exit(0)
		// os.Exit never returns, but we need this for code flow
		return nil
	}

	// Fallback: No known init system detected (crontab-based or bare process)
	// We MUST create a helper script to restart the agent, because:
	// - There is no service manager watching the process
	// - The @reboot crontab entry only runs on boot, not on process exit
	// - Simply sending pkill -HUP would kill the agent with nothing to restart it
	logger.Warn("No known init system detected, creating restart helper script for safe restart...")

	// SECURITY: Ensure /etc/patchmon directory exists with restrictive permissions
	if err := os.MkdirAll("/etc/patchmon", 0700); err != nil {
		logger.WithError(err).Warn("Failed to create /etc/patchmon directory")
	}

	helperScript := `#!/bin/sh
# Wait a moment for the current process to exit
sleep 3
# Kill any remaining patchmon-agent processes
pkill -f 'patchmon-agent serve' 2>/dev/null || true
sleep 1
# Start the new binary in the background
/usr/local/bin/patchmon-agent serve </dev/null >/dev/null 2>&1 &
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

	// SECURITY: Verify the directory is not a symlink
	dirInfo, err := os.Lstat("/etc/patchmon")
	if err == nil && dirInfo.Mode()&os.ModeSymlink != 0 {
		logger.Warn("Security: /etc/patchmon is a symlink, refusing to create helper script")
		logger.Error("Cannot safely restart agent - manual restart required: /usr/local/bin/patchmon-agent serve &")
		return fmt.Errorf("no init system and /etc/patchmon is a symlink - cannot safely restart")
	}

	// SECURITY: Use O_EXCL to atomically create file
	file, err := os.OpenFile(helperPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		logger.WithError(err).Error("Failed to create restart helper script")
		logger.Error("Manual restart required: /usr/local/bin/patchmon-agent serve &")
		return fmt.Errorf("no init system and failed to create helper script: %w", err)
	}

	if _, err := file.WriteString(helperScript); err != nil {
		logger.WithError(err).Error("Failed to write restart helper script")
		if closeErr := file.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close file after write error")
		}
		if removeErr := os.Remove(helperPath); removeErr != nil {
			logger.WithError(removeErr).Warn("Failed to remove helper script after write error")
		}
		logger.Error("Manual restart required: /usr/local/bin/patchmon-agent serve &")
		return fmt.Errorf("no init system and failed to write helper script: %w", err)
	}

	if err := file.Close(); err != nil {
		logger.WithError(err).Warn("Failed to close restart helper script file")
	}

	// SECURITY: Verify the file we're about to execute is the one we created
	fileInfo, err := os.Lstat(helperPath)
	if err != nil || fileInfo.Mode()&os.ModeSymlink != 0 {
		logger.Warn("Security: helper script may have been tampered with, refusing to execute")
		if removeErr := os.Remove(helperPath); removeErr != nil {
			logger.WithError(removeErr).Warn("Failed to remove tampered helper script")
		}
		logger.Error("Manual restart required: /usr/local/bin/patchmon-agent serve &")
		return fmt.Errorf("no init system and helper script security check failed")
	}

	// Execute the helper script in background (detached from current process)
	var cmd *exec.Cmd
	if _, nohupErr := exec.LookPath("nohup"); nohupErr == nil {
		cmd = exec.CommandContext(ctx, "nohup", "/bin/sh", helperPath)
	} else {
		logger.Debug("nohup not available, using direct /bin/sh execution with session detachment")
		cmd = exec.CommandContext(ctx, "/bin/sh", helperPath)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = sysProcAttrForDetach()

	if err := cmd.Start(); err != nil {
		logger.WithError(err).Error("Failed to start restart helper script")
		if removeErr := os.Remove(helperPath); removeErr != nil {
			logger.WithError(removeErr).Debug("Failed to remove helper script")
		}
		logger.Error("Manual restart required: /usr/local/bin/patchmon-agent serve &")
		return fmt.Errorf("no init system and failed to start helper script: %w", err)
	}

	logger.Info("Scheduled agent restart via helper script (no init system), exiting now...")
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
	return nil // Unreachable, but satisfies function signature
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

// runComplianceScanWithOptions runs an on-demand compliance scan with options and sends results to server.
// ctx can be cancelled from the server (e.g. user clicks Cancel) to abort the scan.
// Run scan now works for both on-demand and scheduled compliance modes.
func runComplianceScanWithOptions(ctx context.Context, options *models.ComplianceScanOptions) error {
	profileName := options.ProfileID
	if profileName == "" {
		profileName = "default"
	}

	// Run scan now works for both on-demand and scheduled compliance modes.
	// Only reject if compliance is disabled.
	if !cfgManager.IsIntegrationEnabled("compliance") {
		sendComplianceProgress("failed", profileName, "Compliance scanning is disabled", 0, "compliance integration is not enabled")
		return fmt.Errorf("compliance integration is not enabled")
	}

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"profile_id":         options.ProfileID,
		"enable_remediation": options.EnableRemediation,
	})).Info("Starting on-demand compliance scan")

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

	// Run the scan with options (25 min max; ctx can cancel earlier)
	scanCtx, timeoutCancel := context.WithTimeout(ctx, 25*time.Minute)
	defer timeoutCancel()

	integrationData, err := complianceInteg.CollectWithOptions(scanCtx, options)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			sendComplianceProgress("cancelled", profileName, "Scan cancelled", 0, "")
			return err
		}
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
		AgentVersion:   pkgversion.Version,
		ScanType:       "on-demand",
	}

	// Debug: log what we're about to send
	for i, scan := range payload.Scans {
		statusCounts := map[string]int{}
		for _, r := range scan.Results {
			statusCounts[r.Status]++
		}
		logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
			"scan_index":      i,
			"profile_name":    scan.ProfileName,
			"profile_type":    scan.ProfileType,
			"total_results":   len(scan.Results),
			"result_statuses": statusCounts,
			"scan_passed":     scan.Passed,
			"scan_failed":     scan.Failed,
			"scan_warnings":   scan.Warnings,
			"scan_skipped":    scan.Skipped,
		})).Info("DEBUG: Compliance payload scan details before sending")
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
	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"image_name":      imageName,
		"container_name":  containerName,
		"scan_all_images": scanAllImages,
	})).Info("Starting Docker image CVE scan")

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
		AgentVersion:   pkgversion.Version,
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

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"scans_received": response.ScansReceived,
		"images_scanned": len(scans),
		"cves_found":     totalCVEs,
	})).Info("Docker image CVE scan results sent to server")

	return nil
}

// validateSSHProxyHost validates SSH proxy host to prevent injection
func validateSSHProxyHost(host string) error {
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

// sendSSHProxyMessage sends a message to backend via WebSocket
func sendSSHProxyMessage(conn *websocket.Conn, msgType string, sessionID string, data interface{}) {
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
	if err := writeWebSocketTextMessage(conn, msgJSON); err != nil {
		logger.WithError(err).Error("Failed to send SSH proxy message")
	}
}

func sendSSHProxyError(conn *websocket.Conn, sessionID string, message string) {
	sendSSHProxyMessage(conn, "ssh_proxy_error", sessionID, message)
}

func sendSSHProxyData(conn *websocket.Conn, sessionID string, data string) {
	sendSSHProxyMessage(conn, "ssh_proxy_data", sessionID, data)
}

func sendSSHProxyConnected(conn *websocket.Conn, sessionID string) {
	sendSSHProxyMessage(conn, "ssh_proxy_connected", sessionID, nil)
}

func sendSSHProxyClosed(conn *websocket.Conn, sessionID string) {
	sendSSHProxyMessage(conn, "ssh_proxy_closed", sessionID, nil)
}

// handleSSHProxy establishes SSH connection and manages proxy session
func handleSSHProxy(m wsMsg, conn *websocket.Conn) {
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

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"session_id": sessionID,
		"host":       host,
		"port":       port,
		"username":   username,
	})).Info("Establishing SSH proxy connection")

	// Create SSH client config
	config := &ssh.ClientConfig{
		User:            username,
		HostKeyCallback: agentHostKeyCallback(),
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
			sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to parse private key: %v", err))
			return
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if m.sshProxyPassword != "" {
		// Use password authentication
		config.Auth = []ssh.AuthMethod{ssh.Password(m.sshProxyPassword)}
	} else {
		sendSSHProxyError(conn, sessionID, "No authentication method provided (password or private key required)")
		return
	}

	// Connect to SSH server
	address := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		logger.WithError(err).Error("Failed to connect to SSH server")
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	// Create session
	session, err := client.NewSession()
	if err != nil {
		if closeErr := client.Close(); closeErr != nil {
			logger.WithError(closeErr).Warn("Failed to close SSH client after session creation error")
		}
		logger.WithError(err).Error("Failed to create SSH session")
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to create session: %v", err))
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
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to request PTY: %v", err))
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
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to get stdin: %v", err))
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
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to get stdout: %v", err))
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
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to get stderr: %v", err))
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
		sendSSHProxyError(conn, sessionID, fmt.Sprintf("Failed to start shell: %v", err))
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
	sendSSHProxyConnected(conn, sessionID)

	// Forward stdout to WebSocket
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := stdout.Read(buffer)
			if n > 0 {
				sendSSHProxyData(conn, sessionID, string(buffer[:n]))
			}
			if err != nil {
				if err != io.EOF {
					logger.WithError(err).Error("Error reading from SSH stdout")
				}
				break
			}
		}
		// Clean up on stdout close
		handleSSHProxyDisconnect(wsMsg{sshProxySessionID: sessionID}, conn)
	}()

	// Forward stderr to WebSocket
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := stderr.Read(buffer)
			if n > 0 {
				sendSSHProxyData(conn, sessionID, string(buffer[:n]))
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
		handleSSHProxyDisconnect(wsMsg{sshProxySessionID: sessionID}, conn)
	}()
}

// handleSSHProxyInput sends input to SSH session
func handleSSHProxyInput(m wsMsg, _ *websocket.Conn) {
	sshProxySessionsMu.RLock()
	proxySession, exists := sshProxySessions[m.sshProxySessionID]
	sshProxySessionsMu.RUnlock()

	if !exists {
		logger.WithField("session_id", logutil.Sanitize(m.sshProxySessionID)).Warn("SSH proxy session not found for input")
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

// handleSSHProxyResize resizes SSH terminal
func handleSSHProxyResize(m wsMsg, _ *websocket.Conn) {
	sshProxySessionsMu.RLock()
	proxySession, exists := sshProxySessions[m.sshProxySessionID]
	sshProxySessionsMu.RUnlock()

	if !exists {
		logger.WithField("session_id", logutil.Sanitize(m.sshProxySessionID)).Warn("SSH proxy session not found for resize")
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

// handleSSHProxyDisconnect closes SSH session
func handleSSHProxyDisconnect(m wsMsg, conn *websocket.Conn) {
	sshProxySessionsMu.Lock()
	proxySession, exists := sshProxySessions[m.sshProxySessionID]
	if exists {
		delete(sshProxySessions, m.sshProxySessionID)
	}
	sshProxySessionsMu.Unlock()

	if !exists {
		return
	}

	logger.WithField("session_id", logutil.Sanitize(m.sshProxySessionID)).Info("Closing SSH proxy session")

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
	sendSSHProxyClosed(conn, m.sshProxySessionID)
}

// RDP proxy session management (raw TCP stream to localhost:3389)
type rdpProxySession struct {
	tcpConn   net.Conn
	conn      *websocket.Conn
	sessionID string
	mu        sync.Mutex
}

var rdpProxySessions = make(map[string]*rdpProxySession)
var rdpProxySessionsMu sync.RWMutex

func sendRDPProxyMessage(conn *websocket.Conn, msgType string, sessionID string, data interface{}) {
	msg := map[string]interface{}{
		"type":       msgType,
		"session_id": sessionID,
	}
	if data != nil {
		msg["data"] = data
	}
	if msgType == "rdp_proxy_error" {
		if errMsg, ok := data.(string); ok {
			msg["message"] = errMsg
		}
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		logger.WithError(err).Error("Failed to marshal RDP proxy message")
		return
	}
	if err := writeWebSocketTextMessage(conn, msgJSON); err != nil {
		logger.WithError(err).Error("Failed to send RDP proxy message")
	}
}

func sendRDPProxyError(conn *websocket.Conn, sessionID string, message string) {
	sendRDPProxyMessage(conn, "rdp_proxy_error", sessionID, message)
}

func sendRDPProxyData(conn *websocket.Conn, sessionID string, data string) {
	sendRDPProxyMessage(conn, "rdp_proxy_data", sessionID, data)
}

func sendRDPProxyConnected(conn *websocket.Conn, sessionID string) {
	sendRDPProxyMessage(conn, "rdp_proxy_connected", sessionID, nil)
}

func sendRDPProxyClosed(conn *websocket.Conn, sessionID string) {
	sendRDPProxyMessage(conn, "rdp_proxy_closed", sessionID, nil)
}

func handleRDPProxy(m wsMsg, conn *websocket.Conn) {
	sessionID := m.rdpProxySessionID
	host := m.rdpProxyHost
	if host == "" {
		host = "localhost"
	}
	port := m.rdpProxyPort
	if port <= 0 {
		port = 3389
	}

	logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
		"session_id": sessionID,
		"host":       host,
		"port":       port,
	})).Info("Establishing RDP proxy connection")

	// Dial localhost:3389. Kept at 8s so the server's 12s handshake budget has
	// room for the WebSocket round trip; a slow-to-accept TermService still gets
	// through, and an actually-closed port fails fast with a specific error
	// (rdp_port_unreachable) instead of a generic agent-timeout.
	address := net.JoinHostPort(host, strconv.Itoa(port))
	tcpConn, err := net.DialTimeout("tcp", address, 8*time.Second)
	if err != nil {
		logger.WithError(err).Error("Failed to connect to RDP server")
		sendRDPProxyError(conn, sessionID, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	proxySession := &rdpProxySession{
		tcpConn:   tcpConn,
		conn:      conn,
		sessionID: sessionID,
	}

	rdpProxySessionsMu.Lock()
	rdpProxySessions[sessionID] = proxySession
	rdpProxySessionsMu.Unlock()

	sendRDPProxyConnected(conn, sessionID)

	// Forward TCP -> WebSocket (base64)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := tcpConn.Read(buf)
			if n > 0 {
				sendRDPProxyData(conn, sessionID, base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if err != nil {
				if err != io.EOF {
					logger.WithError(err).Debug("RDP proxy TCP read error")
				}
				break
			}
		}
		handleRDPProxyDisconnect(wsMsg{rdpProxySessionID: sessionID}, conn)
	}()

	// Wait for disconnect
	go func() {
		// Keep session alive until explicitly disconnected
		rdpProxySessionsMu.RLock()
		_, exists := rdpProxySessions[sessionID]
		rdpProxySessionsMu.RUnlock()
		for exists {
			time.Sleep(1 * time.Second)
			rdpProxySessionsMu.RLock()
			_, exists = rdpProxySessions[sessionID]
			rdpProxySessionsMu.RUnlock()
		}
	}()
}

func handleRDPProxyInput(m wsMsg, _ *websocket.Conn) {
	rdpProxySessionsMu.RLock()
	proxySession, exists := rdpProxySessions[m.rdpProxySessionID]
	rdpProxySessionsMu.RUnlock()

	if !exists {
		logger.WithField("session_id", logutil.Sanitize(m.rdpProxySessionID)).Warn("RDP proxy session not found for input")
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(m.rdpProxyData)
	if err != nil {
		logger.WithError(err).Warn("Failed to decode RDP proxy input")
		return
	}

	proxySession.mu.Lock()
	defer proxySession.mu.Unlock()

	if proxySession.tcpConn != nil {
		if _, err := proxySession.tcpConn.Write(decoded); err != nil {
			logger.WithError(err).Error("Failed to write to RDP TCP connection")
		}
	}
}

func handleRDPProxyDisconnect(m wsMsg, conn *websocket.Conn) {
	sessionID := m.rdpProxySessionID

	rdpProxySessionsMu.Lock()
	proxySession, exists := rdpProxySessions[sessionID]
	if exists {
		delete(rdpProxySessions, sessionID)
	}
	rdpProxySessionsMu.Unlock()

	if !exists {
		logger.WithField("session_id", logutil.Sanitize(sessionID)).Debug("RDP proxy session already closed")
		return
	}

	logger.WithField("session_id", logutil.Sanitize(sessionID)).Info("Closing RDP proxy session")

	if proxySession.tcpConn != nil {
		if err := proxySession.tcpConn.Close(); err != nil {
			logger.WithError(err).Warn("Failed to close RDP proxy TCP connection")
		}
	}

	sendRDPProxyClosed(conn, sessionID)
}
