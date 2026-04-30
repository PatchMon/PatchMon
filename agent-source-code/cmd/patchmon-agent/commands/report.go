package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"patchmon-agent/internal/client"
	"patchmon-agent/internal/hardware"
	"patchmon-agent/internal/hashing"
	"patchmon-agent/internal/integrations"
	"patchmon-agent/internal/integrations/compliance"
	"patchmon-agent/internal/integrations/docker"
	"patchmon-agent/internal/network"
	"patchmon-agent/internal/packages"
	"patchmon-agent/internal/pkgversion"
	"patchmon-agent/internal/repositories"
	"patchmon-agent/internal/system"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var reportJSON bool

// reportCmd represents the report command
var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report system and package information to server",
	Long:  "Collect and report system, package, and repository information to the PatchMon server.",
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		return sendReport(reportJSON)
	},
}

func init() {
	reportCmd.Flags().BoolVar(&reportJSON, "json", false, "Output the JSON report payload to stdout instead of sending to server")
}

// collectedReport holds the result of one full local data collection. Used
// by both sendReport (legacy full-report flow) and runCheckIn (hash-gated
// periodic check-in).
type collectedReport struct {
	Payload  *models.ReportPayload
	StartAt  time.Time
	HasFatal bool
}

// collectReportData runs every collector exactly the way sendReport does,
// then assembles a ReportPayload. Returns a fatal error only if a *required*
// collector failed (os, hostname, packages); soft failures (repos, network)
// are logged and replaced with empty slices, matching the legacy behaviour.
//
// Both sendReport and runCheckIn call this. Keeping the assembly in one
// place means hash computation in runCheckIn can use the exact same data
// the agent would have shipped — drift between the periodic-tick
// canonicalisation and the follow-up partial /hosts/update payload is
// therefore impossible.
func collectReportData() (*collectedReport, error) {
	startTime := time.Now()
	runtime.GC()

	systemDetector := system.New(logger)
	packageMgr := packages.New(logger, packages.CacheRefreshConfig{
		Mode:   cfgManager.GetPackageCacheRefreshMode(),
		MaxAge: cfgManager.GetPackageCacheRefreshMaxAge(),
	})
	repoMgr := repositories.New(logger)
	hardwareMgr := hardware.New(logger)
	networkMgr := network.New(logger)

	var (
		osType, osVersion             string
		osErr                         error
		hostname                      string
		hostnameErr                   error
		architecture                  string
		systemInfo                    models.SystemInfo
		ipAddress                     string
		hardwareInfo                  models.HardwareInfo
		networkInfo                   models.NetworkInfo
		needsReboot                   bool
		rebootReason                  string
		installedKernel               string
		packageList                   []models.Package
		pkgErr                        error
		repoList                      []models.Repository
		repoErr                       error
		machineID, detectedPackageMgr string
	)

	var (
		panicMu    sync.Mutex
		taskPanics = make(map[string]any)
	)

	var wg sync.WaitGroup
	runTask := func(name string, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicMu.Lock()
					taskPanics[name] = r
					panicMu.Unlock()
					logger.WithFields(logrus.Fields{"task": name, "panic": r}).Error("Collector panicked")
				}
			}()
			fn()
		}()
	}

	runTask("os", func() { osType, osVersion, osErr = systemDetector.DetectOS() })
	runTask("hostname", func() { hostname, hostnameErr = systemDetector.GetHostname() })
	runTask("architecture", func() { architecture = systemDetector.GetArchitecture() })
	runTask("systemInfo", func() { systemInfo = systemDetector.GetSystemInfo() })
	runTask("ip", func() { ipAddress = systemDetector.GetIPAddress() })
	runTask("hardware", func() { hardwareInfo = hardwareMgr.GetHardwareInfo() })
	runTask("network", func() {
		networkInfo = networkMgr.GetNetworkInfo()
		if networkInfo.DNSServers == nil {
			networkInfo.DNSServers = []string{}
		}
	})
	runTask("reboot", func() { needsReboot, rebootReason = systemDetector.CheckRebootRequired() })
	runTask("kernel", func() { installedKernel = systemDetector.GetLatestInstalledKernel() })
	runTask("machineID", func() { machineID = systemDetector.GetMachineID() })
	runTask("packageMgr", func() { detectedPackageMgr = packageMgr.DetectPackageManager() })
	runTask("packages", func() { packageList, pkgErr = packageMgr.GetPackages() })
	runTask("repos", func() { repoList, repoErr = repoMgr.GetRepositories() })

	wg.Wait()

	for _, name := range []string{"os", "hostname", "packages"} {
		if p, ok := taskPanics[name]; ok {
			return nil, fmt.Errorf("%s collector panicked: %v", name, p)
		}
	}
	if osErr != nil {
		return nil, fmt.Errorf("failed to detect OS: %w", osErr)
	}
	if hostnameErr != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", hostnameErr)
	}
	if pkgErr != nil {
		return nil, fmt.Errorf("failed to get packages: %w", pkgErr)
	}
	if repoErr != nil {
		logger.WithError(repoErr).Warn("Failed to get repositories")
		repoList = []models.Repository{}
	}

	if packageList == nil {
		packageList = []models.Package{}
	}
	if repoList == nil {
		repoList = []models.Repository{}
	}

	executionTime := time.Since(startTime).Seconds()
	payload := &models.ReportPayload{
		Packages:               packageList,
		Repositories:           repoList,
		OSType:                 osType,
		OSVersion:              osVersion,
		Hostname:               hostname,
		IP:                     ipAddress,
		Architecture:           architecture,
		AgentVersion:           pkgversion.Version,
		MachineID:              machineID,
		KernelVersion:          systemInfo.KernelVersion,
		InstalledKernelVersion: installedKernel,
		SELinuxStatus:          systemInfo.SELinuxStatus,
		SystemUptime:           systemInfo.SystemUptime,
		LoadAverage:            systemInfo.LoadAverage,
		CPUModel:               hardwareInfo.CPUModel,
		CPUCores:               hardwareInfo.CPUCores,
		RAMInstalled:           hardwareInfo.RAMInstalled,
		SwapSize:               hardwareInfo.SwapSize,
		DiskDetails:            hardwareInfo.DiskDetails,
		GatewayIP:              networkInfo.GatewayIP,
		DNSServers:             networkInfo.DNSServers,
		NetworkInterfaces:      networkInfo.NetworkInterfaces,
		ExecutionTime:          executionTime,
		NeedsReboot:            needsReboot,
		RebootReason:           rebootReason,
		PackageManager:         detectedPackageMgr,
	}
	return &collectedReport{Payload: payload, StartAt: startTime}, nil
}

// computeReportHashes computes the four "main report" canonical hashes from
// the collected payload. Errors are propagated; an empty hash is never
// returned silently. Caller wires the returned hashes into the agent's
// outbound payload (so the server can stamp them on the host row) and into
// the outbound PingHashes (for the next steady-state hash compare).
func computeReportHashes(p *models.ReportPayload) (models.ReportHashes, error) {
	pkgs, err := hashing.PackagesHash(p.Packages)
	if err != nil {
		return models.ReportHashes{}, fmt.Errorf("packages hash: %w", err)
	}
	repos, err := hashing.ReposHash(p.Repositories)
	if err != nil {
		return models.ReportHashes{}, fmt.Errorf("repos hash: %w", err)
	}
	ifaces, err := hashing.InterfacesHash(p.NetworkInterfaces)
	if err != nil {
		return models.ReportHashes{}, fmt.Errorf("interfaces hash: %w", err)
	}
	return models.ReportHashes{
		PackagesHash:   pkgs,
		ReposHash:      repos,
		InterfacesHash: ifaces,
		HostnameHash:   hashing.HostnameHash(p.Hostname),
	}, nil
}

func sendReport(outputJSON bool) error {
	logger.Debug("Starting report process")

	// Load API credentials only if we're sending the report (not just outputting JSON)
	if !outputJSON {
		logger.Debug("Loading API credentials")
		if err := cfgManager.LoadCredentials(); err != nil {
			logger.WithError(err).Debug("Failed to load credentials")
			return err
		}
	}

	collected, err := collectReportData()
	if err != nil {
		return err
	}
	payload := collected.Payload

	// Stamp the canonical hashes on every full report so the server's host
	// row gets the up-to-date values. Failure here is logged but not fatal —
	// the payload still goes through; the worst case is the next ping
	// asks for the same content again.
	if hashes, err := computeReportHashes(payload); err != nil {
		logger.WithError(err).Warn("failed to compute canonical report hashes (continuing without)")
	} else {
		payload.Hashes = hashes
	}

	logger.WithFields(logrus.Fields{"osType": payload.OSType, "osVersion": payload.OSVersion}).Info("Detected OS")
	logger.WithFields(logrus.Fields{
		"needs_reboot":     payload.NeedsReboot,
		"reason":           payload.RebootReason,
		"installed_kernel": payload.InstalledKernelVersion,
		"running_kernel":   payload.KernelVersion,
	}).Info("Reboot status check completed")

	logger.WithField("count", len(payload.Packages)).Info("Found packages")
	logger.WithField("count", len(payload.Repositories)).Info("Found repositories")
	logger.WithField("execution_time_seconds", payload.ExecutionTime).Debug("Data collection completed")

	// If --report-json flag is set, output JSON and exit
	if outputJSON {
		jsonData, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if _, err := fmt.Fprintf(os.Stdout, "%s\n", jsonData); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
		return nil
	}

	// Send report
	logger.Info("Sending report to PatchMon server...")
	httpClient := client.New(cfgManager, logger)
	ctx := context.Background()
	response, err := httpClient.SendUpdate(ctx, payload)
	if err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}

	logger.Info("Report sent successfully")
	logger.WithField("count", response.PackagesProcessed).Info("Processed packages")

	// Handle agent auto-update (server-initiated)
	if response.AutoUpdate != nil && response.AutoUpdate.ShouldUpdate {
		logger.WithFields(logrus.Fields{
			"current": response.AutoUpdate.CurrentVersion,
			"latest":  response.AutoUpdate.LatestVersion,
			"message": response.AutoUpdate.Message,
		}).Info("PatchMon agent update detected")

		logger.Info("Automatically updating PatchMon agent to latest version...")
		if err := updateAgent(); err != nil {
			logger.WithError(err).Warn("PatchMon agent update failed, but data was sent successfully")
		} else {
			logger.Info("PatchMon agent update completed successfully")
			// updateAgent() will exit the process after restart, so we won't reach here
			// But if it does return, skip the update check to prevent loops
			return nil
		}
	} else {
		// Proactive update check after report (with timeout to prevent hanging)
		// Use a WaitGroup to ensure the goroutine completes before function returns
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Create a context with timeout to prevent indefinite hanging
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			// Add a delay to prevent immediate checks after service restart
			// This gives the new process time to fully initialize
			select {
			case <-time.After(5 * time.Second):
				// Continue with update check
			case <-ctx.Done():
				logger.Debug("Update check cancelled due to timeout")
				return
			}

			logger.Info("Checking for agent updates...")
			versionInfo, err := getServerVersionInfo()
			if err != nil {
				logger.WithError(err).Warn("Failed to check for updates after report (non-critical)")
				return
			}
			if versionInfo.HasUpdate {
				logger.WithFields(logrus.Fields{
					"current": versionInfo.CurrentVersion,
					"latest":  versionInfo.LatestVersion,
				}).Info("Update available, automatically updating...")

				if err := updateAgent(); err != nil {
					logger.WithError(err).Warn("PatchMon agent update failed, but data was sent successfully")
				} else {
					logger.Info("PatchMon agent update completed successfully")
					// updateAgent() will exit after restart, so this won't be reached
				}
			} else if versionInfo.AutoUpdateDisabled && versionInfo.LatestVersion != versionInfo.CurrentVersion {
				// Update is available but auto-update is disabled
				logger.WithFields(logrus.Fields{
					"current": versionInfo.CurrentVersion,
					"latest":  versionInfo.LatestVersion,
					"reason":  versionInfo.AutoUpdateDisabledReason,
				}).Info("New update available but auto-update is disabled")
			} else {
				logger.WithField("version", versionInfo.CurrentVersion).Info("Agent is up to date")
			}
		}()
		// Wait for the update check to complete (with the internal timeout)
		wg.Wait()
	}

	// Collect and send integration data (Docker, etc.) separately
	// This ensures failures in integrations don't affect core system reporting
	sendIntegrationData()

	logger.Debug("Report process completed")
	return nil
}

// sendIntegrationData collects and sends data from integrations (Docker, etc.)
func sendIntegrationData() {
	logger.Debug("Starting integration data collection")

	// Create integration manager
	integrationMgr := integrations.NewManager(logger)

	// Set enabled checker to respect config.yml settings
	// Load config first to check integration status
	if err := cfgManager.LoadConfig(); err != nil {
		logger.WithError(err).Debug("Failed to load config for integration check")
	}
	integrationMgr.SetEnabledChecker(func(name string) bool {
		return cfgManager.IsIntegrationEnabled(name)
	})

	// Register available integrations
	integrationMgr.Register(docker.New(logger))

	// Future: integrationMgr.Register(proxmox.New(logger))
	// Future: integrationMgr.Register(kubernetes.New(logger))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	integrationData := integrationMgr.CollectAll(ctx)

	if len(integrationData) == 0 {
		logger.Debug("No integration data to send")
		return
	}

	// Get system info for integration payloads
	systemDetector := system.New(logger)
	hostname, _ := systemDetector.GetHostname()
	machineID := systemDetector.GetMachineID()

	// Create HTTP client
	httpClient := client.New(cfgManager, logger)

	// Send Docker data if available
	if dockerData, exists := integrationData["docker"]; exists && dockerData.Error == "" {
		sendDockerData(httpClient, dockerData, hostname, machineID)
	}

	// Future: Send other integration data here
}

// sendDockerData sends Docker integration data to server
func sendDockerData(httpClient *client.Client, integrationData *models.IntegrationData, hostname, machineID string) {
	// Extract Docker data from integration data
	dockerData, ok := integrationData.Data.(*models.DockerData)
	if !ok {
		logger.Warn("Failed to extract Docker data from integration")
		return
	}

	payload := &models.DockerPayload{
		DockerData:   *dockerData,
		Hostname:     hostname,
		MachineID:    machineID,
		AgentVersion: pkgversion.Version,
	}
	// Stamp the canonical docker hash so the server can store it and
	// hash-gate the docker section on the next ping. Failure is logged
	// but non-fatal — the upload still goes through; the worst case is
	// the next ping requests docker again.
	if dh, err := hashing.DockerHash(dockerData); err != nil {
		logger.WithError(err).Debug("docker hash computation failed; uploading without hash")
	} else {
		payload.DockerHash = dh
		setLastDockerHash(dh)
	}

	logger.WithFields(logrus.Fields{
		"containers": len(dockerData.Containers),
		"images":     len(dockerData.Images),
		"volumes":    len(dockerData.Volumes),
		"networks":   len(dockerData.Networks),
		"updates":    len(dockerData.Updates),
	}).Info("Sending Docker data to server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := httpClient.SendDockerData(ctx, payload)
	if err != nil {
		logger.WithError(err).Warn("Failed to send Docker data (will retry on next report)")
		return
	}

	logger.WithFields(logrus.Fields{
		"containers": response.ContainersReceived,
		"images":     response.ImagesReceived,
		"volumes":    response.VolumesReceived,
		"networks":   response.NetworksReceived,
		"updates":    response.UpdatesFound,
	}).Info("Docker data sent successfully")
}

// sendComplianceData sends compliance scan data to server
func sendComplianceData(httpClient *client.Client, integrationData *models.IntegrationData, hostname, machineID, scanType string) {
	// Extract Compliance data from integration data
	complianceData, ok := integrationData.Data.(*models.ComplianceData)
	if !ok {
		logger.Warn("Failed to extract compliance data from integration")
		return
	}

	if len(complianceData.Scans) == 0 {
		logger.Debug("No compliance scans to send")
		return
	}

	payload := &models.CompliancePayload{
		ComplianceData: *complianceData,
		Hostname:       hostname,
		MachineID:      machineID,
		AgentVersion:   pkgversion.Version,
		ScanType:       scanType,
	}
	// Stamp the canonical compliance hash so the server can hash-gate
	// the compliance section on the next ping.
	if ch, err := hashing.ComplianceHash(complianceData); err != nil {
		logger.WithError(err).Debug("compliance hash computation failed; uploading without hash")
	} else {
		payload.ComplianceHash = ch
		setLastComplianceHash(ch)
	}

	totalRules := 0
	for _, scan := range complianceData.Scans {
		totalRules += scan.TotalRules
	}

	logger.WithFields(logrus.Fields{
		"scans":       len(complianceData.Scans),
		"total_rules": totalRules,
	}).Info("Sending compliance data to server...")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // Longer timeout for compliance
	defer cancel()

	response, err := httpClient.SendComplianceData(ctx, payload)
	if err != nil {
		logger.WithError(err).Warn("Failed to send compliance data (will retry on next report)")
		return
	}

	logger.WithFields(logrus.Fields{
		"scans_received": response.ScansReceived,
		"message":        response.Message,
	}).Info("Compliance data sent successfully")
}

// runCheckIn is the per-tick hash-gated check-in. It runs all collectors,
// computes per-section hashes, pings the server, and on a non-empty
// requestFull response fires a partial /hosts/update for just the stale
// sections. Docker and compliance staleness routes through their existing
// dedicated endpoints — runCheckIn never bundles those into /hosts/update.
//
// On any failure runCheckIn falls back to legacy full-report behaviour so a
// hashing bug or a transient ping failure cannot dark out the agent.
func runCheckIn(ctx context.Context) error {
	logger.Debug("Starting hash-gated check-in")
	if err := cfgManager.LoadCredentials(); err != nil {
		return err
	}
	collected, err := collectReportData()
	if err != nil {
		// Hard collector failure (os/hostname/packages panic). Fall through
		// to sendReport-style error propagation — the caller will log and
		// retry on the next tick.
		return err
	}
	payload := collected.Payload

	hashes, hashErr := computeReportHashes(payload)
	if hashErr != nil {
		// Hashing failed — degrade to a full report so the host still gets
		// updated. The next ping with empty hashes will force a full anyway,
		// but doing it inline avoids an extra network round-trip.
		logger.WithError(hashErr).Warn("hash computation failed; falling back to full report")
		return sendReport(false)
	}
	payload.Hashes = hashes

	// Docker / compliance hashes are computed from cached integration data
	// (the agent does not re-collect docker or run a scan just to hash). If
	// the agent has no cache yet (cold start) the hash stays empty and the
	// server will request a fresh upload.
	dockerHash := lastDockerHash()
	complianceHash := lastComplianceHash()

	pingReq := &models.PingRequest{
		AgentVersion: pkgversion.Version,
		Hashes: models.PingHashes{
			PackagesHash:   hashes.PackagesHash,
			ReposHash:      hashes.ReposHash,
			InterfacesHash: hashes.InterfacesHash,
			HostnameHash:   hashes.HostnameHash,
			DockerHash:     dockerHash,
			ComplianceHash: complianceHash,
		},
		Metrics: models.PingMetrics{
			CPUCores:     intPtr(payload.CPUCores),
			CPUModel:     strPtrIfNonEmpty(payload.CPUModel),
			RAMInstalled: float64Ptr(payload.RAMInstalled),
			SwapSize:     float64Ptr(payload.SwapSize),
			DiskDetails:  payload.DiskDetails,
			SystemUptime: strPtrIfNonEmpty(payload.SystemUptime),
			LoadAverage:  payload.LoadAverage,
			NeedsReboot:  boolPtr(payload.NeedsReboot),
			RebootReason: strPtrIfNonEmpty(payload.RebootReason),
		},
	}

	httpClient := client.New(cfgManager, logger)
	resp, err := httpClient.Ping(ctx, pingReq)
	if err != nil {
		// Server unreachable / wrong creds — caller logs and retries.
		return fmt.Errorf("check-in ping failed: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"requestFull": resp.RequestFull,
	}).Debug("Check-in completed")

	if len(resp.RequestFull) == 0 {
		// Steady state: no work to do. ~1 KB ping is the entire round-trip.
		return nil
	}

	// Partition the request into main-report sections and integration
	// sections. Main-report sections go in one /hosts/update; integrations
	// go through their dedicated endpoints.
	var mainSections []string
	var sendDockerNow, sendComplianceNow bool
	for _, s := range resp.RequestFull {
		switch s {
		case models.SectionPackages, models.SectionRepos, models.SectionInterfaces, models.SectionHostname:
			mainSections = append(mainSections, s)
		case models.SectionDocker:
			sendDockerNow = true
		case models.SectionCompliance:
			sendComplianceNow = true
		}
	}

	if len(mainSections) > 0 {
		if err := sendPartialReport(ctx, httpClient, payload, mainSections); err != nil {
			logger.WithError(err).Warn("partial report failed; will retry on next tick")
		}
	}

	if sendDockerNow && cfgManager.IsIntegrationEnabled("docker") {
		systemDetector := system.New(logger)
		hostname, _ := systemDetector.GetHostname()
		machineID := systemDetector.GetMachineID()
		integrationMgr := integrations.NewManager(logger)
		integrationMgr.SetEnabledChecker(func(name string) bool { return cfgManager.IsIntegrationEnabled(name) })
		integrationMgr.Register(docker.New(logger))
		dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		integData := integrationMgr.CollectAll(dctx)
		cancel()
		if dd, ok := integData["docker"]; ok && dd.Error == "" {
			sendDockerData(httpClient, dd, hostname, machineID)
		}
	}

	if sendComplianceNow && cfgManager.IsIntegrationEnabled("compliance") {
		// Per user decision: run a fresh on-demand scan when the server
		// asks for compliance. The scheduled-scan path is intentionally
		// reused — same compliance integration, same payload shape, just
		// triggered by a ping response instead of a cron tick.
		go runScheduledComplianceScan()
	}

	return nil
}

// sendPartialReport ships a /hosts/update payload constrained to the listed
// sections. Empty strings in unrelated payload fields are NOT clobbered —
// the server's COALESCE-guarded UPDATE preserves the previous value for any
// column the partial report doesn't touch.
func sendPartialReport(ctx context.Context, httpClient *client.Client, full *models.ReportPayload, sections []string) error {
	wantPackages, wantRepos, wantInterfaces, wantHostname := false, false, false, false
	for _, s := range sections {
		switch s {
		case models.SectionPackages:
			wantPackages = true
		case models.SectionRepos:
			wantRepos = true
		case models.SectionInterfaces:
			wantInterfaces = true
		case models.SectionHostname:
			wantHostname = true
		}
	}

	partial := &models.ReportPayload{
		// Always-included identity fields. The server's UpdateHostFromReport
		// COALESCEs these with prior values so omitting them is safe; we
		// include them to keep the payload self-describing.
		AgentVersion: full.AgentVersion,
		MachineID:    full.MachineID,
		OSType:       full.OSType,
		OSVersion:    full.OSVersion,
		Architecture: full.Architecture,

		// Section payload fields, gated on what was requested.
		Sections: sections,
		Hashes: models.ReportHashes{
			PackagesHash:   full.Hashes.PackagesHash,
			ReposHash:      full.Hashes.ReposHash,
			InterfacesHash: full.Hashes.InterfacesHash,
			HostnameHash:   full.Hashes.HostnameHash,
		},
	}
	if wantPackages {
		partial.Packages = full.Packages
		partial.PackageManager = full.PackageManager
	} else {
		partial.Packages = nil
	}
	if wantRepos {
		partial.Repositories = full.Repositories
	} else {
		partial.Repositories = nil
	}
	if wantInterfaces {
		partial.IP = full.IP
		partial.GatewayIP = full.GatewayIP
		partial.DNSServers = full.DNSServers
		partial.NetworkInterfaces = full.NetworkInterfaces
	}
	if wantHostname {
		partial.Hostname = full.Hostname
	}

	logger.WithFields(logrus.Fields{
		"sections": sections,
	}).Info("Sending hash-gated partial report")
	if _, err := httpClient.SendUpdate(ctx, partial); err != nil {
		return fmt.Errorf("send partial report: %w", err)
	}
	return nil
}

// In-process hash cache for docker and compliance. The agent computes a
// canonical hash whenever it uploads docker or compliance data, and ships
// the cached value on subsequent pings so the server can hash-gate. If the
// cache is empty (e.g. agent just started, integration never ran), the hash
// is empty and the server treats the section as stale on the next ping —
// which is the correct cold-start behaviour. The cache is intentionally
// process-lifetime only; restarts trigger one fresh upload, no persistent
// state needed.
var (
	lastHashMu                    sync.Mutex
	cachedDockerHash, cachedCompH string
)

func lastDockerHash() string {
	lastHashMu.Lock()
	defer lastHashMu.Unlock()
	return cachedDockerHash
}

func lastComplianceHash() string {
	lastHashMu.Lock()
	defer lastHashMu.Unlock()
	return cachedCompH
}

func setLastDockerHash(h string) {
	lastHashMu.Lock()
	defer lastHashMu.Unlock()
	cachedDockerHash = h
}

func setLastComplianceHash(h string) {
	lastHashMu.Lock()
	defer lastHashMu.Unlock()
	cachedCompH = h
}

// Tiny helpers used by runCheckIn to lift Go zero-values into pointers when
// building the PingMetrics. Keeping these inline lets the body of runCheckIn
// stay readable.
func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
func float64Ptr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
func boolPtr(v bool) *bool { return &v }
func strPtrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func runScheduledComplianceScan() {
	if !cfgManager.IsIntegrationEnabled("compliance") || cfgManager.IsComplianceOnDemandOnly() {
		logger.Debug("Skipping scheduled compliance scan (not in enabled mode)")
		return
	}

	if !complianceScanRunning.CompareAndSwap(false, true) {
		complianceScanCancelMu.Lock()
		source := complianceScanSource
		complianceScanCancelMu.Unlock()
		logger.WithField("running_source", source).Debug("Skipping scheduled compliance scan (scan already running)")
		return
	}

	complianceScanCancelMu.Lock()
	complianceScanSource = "scheduled"
	complianceScanCancelMu.Unlock()

	defer func() {
		complianceScanCancelMu.Lock()
		complianceScanSource = ""
		complianceScanCancelMu.Unlock()
		complianceScanRunning.Store(false)
	}()

	startTime := time.Now()
	logger.Info("Starting scheduled compliance scan")

	if err := cfgManager.LoadConfig(); err != nil {
		logger.WithError(err).Debug("Failed to load config for scheduled compliance scan")
	}

	complianceInteg := compliance.New(logger)
	complianceInteg.SetDockerIntegrationEnabled(cfgManager.IsIntegrationEnabled("docker"))
	complianceInteg.SetScannerOptionsGetter(func() (bool, bool) {
		return cfgManager.GetComplianceOpenscapEnabled(), cfgManager.GetComplianceDockerBenchEnabled()
	})

	if !complianceInteg.IsAvailable() {
		logger.Debug("Compliance scanning not available on this system, skipping scheduled scan")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	complianceScanCancelMu.Lock()
	complianceScanCancel = cancel
	complianceScanCancelMu.Unlock()
	defer func() {
		complianceScanCancelMu.Lock()
		complianceScanCancel = nil
		complianceScanCancelMu.Unlock()
	}()

	integrationData, err := complianceInteg.Collect(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("Scheduled compliance scan was cancelled")
		} else {
			logger.WithError(err).Warn("Scheduled compliance scan failed")
		}
		return
	}

	if integrationData == nil || integrationData.Error != "" {
		if integrationData != nil {
			logger.WithField("error", integrationData.Error).Warn("Scheduled compliance scan returned error")
		}
		return
	}

	systemDetector := system.New(logger)
	hostname, _ := systemDetector.GetHostname()
	machineID := systemDetector.GetMachineID()

	httpClient := client.New(cfgManager, logger)
	sendComplianceData(httpClient, integrationData, hostname, machineID, "scheduled")

	logger.WithField("elapsed_ms", time.Since(startTime).Milliseconds()).Info("Scheduled compliance scan completed")
}
