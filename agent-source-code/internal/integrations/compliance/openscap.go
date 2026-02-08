package compliance

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

const (
	oscapBinary    = "oscap"
	scapContentDir = "/usr/share/xml/scap/ssg/content"
	osReleasePath  = "/etc/os-release"
)

// Profile mappings for different OS families
var profileMappings = map[string]map[string]string{
	"level1_server": {
		"ubuntu":   "xccdf_org.ssgproject.content_profile_cis_level1_server",
		"debian":   "xccdf_org.ssgproject.content_profile_cis_level1_server",
		"rhel":     "xccdf_org.ssgproject.content_profile_cis",
		"centos":   "xccdf_org.ssgproject.content_profile_cis",
		"rocky":    "xccdf_org.ssgproject.content_profile_cis",
		"alma":     "xccdf_org.ssgproject.content_profile_cis",
		"fedora":   "xccdf_org.ssgproject.content_profile_cis",
		"sles":     "xccdf_org.ssgproject.content_profile_cis",
		"opensuse": "xccdf_org.ssgproject.content_profile_cis",
	},
	"level2_server": {
		"ubuntu": "xccdf_org.ssgproject.content_profile_cis_level2_server",
		"debian": "xccdf_org.ssgproject.content_profile_cis_level2_server",
		"rhel":   "xccdf_org.ssgproject.content_profile_cis_server_l1",
		"centos": "xccdf_org.ssgproject.content_profile_cis_server_l1",
		"rocky":  "xccdf_org.ssgproject.content_profile_cis_server_l1",
		"alma":   "xccdf_org.ssgproject.content_profile_cis_server_l1",
	},
}

// OpenSCAPScanner handles OpenSCAP compliance scanning
type OpenSCAPScanner struct {
	logger    *logrus.Logger
	osInfo    models.ComplianceOSInfo
	idLike    string // Stores ID_LIKE from /etc/os-release for base distribution detection
	available bool
	version   string
}

// NewOpenSCAPScanner creates a new OpenSCAP scanner
func NewOpenSCAPScanner(logger *logrus.Logger) *OpenSCAPScanner {
	s := &OpenSCAPScanner{
		logger: logger,
	}
	s.osInfo = s.detectOS()
	s.checkAvailability()
	return s
}

// IsAvailable returns whether OpenSCAP is available
func (s *OpenSCAPScanner) IsAvailable() bool {
	return s.available
}

// GetVersion returns the OpenSCAP version
func (s *OpenSCAPScanner) GetVersion() string {
	return s.version
}

// GetOSInfo returns detected OS information
func (s *OpenSCAPScanner) GetOSInfo() models.ComplianceOSInfo {
	return s.osInfo
}

// GetContentFile returns the path to the content file being used
func (s *OpenSCAPScanner) GetContentFilePath() string {
	return s.getContentFile()
}

// GetContentPackageVersion returns the SSG content version
// First checks for GitHub-installed version, then falls back to package manager
func (s *OpenSCAPScanner) GetContentPackageVersion() string {
	// First check for GitHub-installed version marker
	githubVersion := s.getInstalledSSGVersion()
	if githubVersion != "" {
		return githubVersion
	}

	// Fall back to package manager version
	var cmd *exec.Cmd

	switch s.osInfo.Family {
	case "debian":
		cmd = exec.Command("dpkg-query", "-W", "-f=${Version}", "ssg-base")
	case "rhel":
		cmd = exec.Command("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", "scap-security-guide")
	case "suse":
		cmd = exec.Command("rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", "scap-security-guide")
	default:
		return ""
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// DiscoverProfiles returns all available profiles from the SCAP content file
func (s *OpenSCAPScanner) DiscoverProfiles() []models.ScanProfileInfo {
	contentFile := s.getContentFile()
	if contentFile == "" {
		s.logger.Debug("No content file available, returning default profiles")
		return s.getDefaultProfiles()
	}

	// Run oscap info to get profile list
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, oscapBinary, "info", "--profiles", contentFile)
	output, err := cmd.Output()
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get profiles from oscap info, using defaults")
		return s.getDefaultProfiles()
	}

	profiles := []models.ScanProfileInfo{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse profile line: "xccdf_org.ssgproject.content_profile_cis_level1_server:CIS Ubuntu 22.04 Level 1 Server Benchmark"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 1 {
			continue
		}

		xccdfId := strings.TrimSpace(parts[0])
		name := xccdfId
		if len(parts) == 2 {
			name = strings.TrimSpace(parts[1])
		}

		// Determine category from profile ID
		category := s.categorizeProfile(xccdfId)

		// Create short ID from XCCDF ID
		shortId := s.createShortId(xccdfId)

		profiles = append(profiles, models.ScanProfileInfo{
			ID:       shortId,
			Name:     name,
			Type:     "openscap",
			XCCDFId:  xccdfId,
			Category: category,
		})
	}

	if len(profiles) == 0 {
		return s.getDefaultProfiles()
	}

	s.logger.WithField("count", len(profiles)).Debug("Discovered profiles from SCAP content")
	return profiles
}

// categorizeProfile determines the category of a profile based on its ID
func (s *OpenSCAPScanner) categorizeProfile(xccdfId string) string {
	id := strings.ToLower(xccdfId)
	switch {
	case strings.Contains(id, "cis"):
		return "cis"
	case strings.Contains(id, "stig"):
		return "stig"
	case strings.Contains(id, "pci") || strings.Contains(id, "pci-dss"):
		return "pci-dss"
	case strings.Contains(id, "hipaa"):
		return "hipaa"
	case strings.Contains(id, "anssi"):
		return "anssi"
	case strings.Contains(id, "standard"):
		return "standard"
	default:
		return "other"
	}
}

// createShortId creates a short profile ID from the full XCCDF ID
func (s *OpenSCAPScanner) createShortId(xccdfId string) string {
	// Extract the profile name part: xccdf_org.ssgproject.content_profile_XXX -> XXX
	if strings.Contains(xccdfId, "_profile_") {
		parts := strings.SplitN(xccdfId, "_profile_", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return xccdfId
}

// getDefaultProfiles returns fallback profiles when discovery fails
func (s *OpenSCAPScanner) getDefaultProfiles() []models.ScanProfileInfo {
	return []models.ScanProfileInfo{
		{
			ID:          "level1_server",
			Name:        "CIS Level 1 Server",
			Description: "Basic security hardening for servers",
			Type:        "openscap",
			Category:    "cis",
		},
		{
			ID:          "level2_server",
			Name:        "CIS Level 2 Server",
			Description: "Extended security hardening (more restrictive)",
			Type:        "openscap",
			Category:    "cis",
		},
	}
}

// GetScannerDetails returns comprehensive scanner information
func (s *OpenSCAPScanner) GetScannerDetails() *models.ComplianceScannerDetails {
	contentFile := s.getContentFile()
	contentVersion := s.GetContentPackageVersion()

	// Determine minimum required SSG version for this OS
	// Use base distribution name (from ID_LIKE) for version checks
	baseOSName := s.getContentOSName()
	minVersion := ""
	if baseOSName == "ubuntu" && s.osInfo.Version >= "24.04" {
		minVersion = "0.1.76"
	} else if baseOSName == "ubuntu" && s.osInfo.Version >= "22.04" {
		minVersion = "0.1.60"
	}

	// Check if SSG needs upgrade
	ssgNeedsUpgrade := false
	ssgUpgradeMessage := ""
	if minVersion != "" && contentVersion != "" {
		if compareVersions(contentVersion, minVersion) < 0 {
			ssgNeedsUpgrade = true
			ssgUpgradeMessage = fmt.Sprintf("ssg-base %s is installed, but %s %s requires v%s+ for proper CIS/STIG content.",
				contentVersion, s.osInfo.Name, s.osInfo.Version, minVersion)
		}
	} else if minVersion != "" && contentVersion == "" {
		ssgNeedsUpgrade = true
		ssgUpgradeMessage = fmt.Sprintf("ssg-base is not installed. %s %s requires ssg-base v%s+ for CIS/STIG scanning.",
			s.osInfo.Name, s.osInfo.Version, minVersion)
	}

	// Check for content mismatch
	contentMismatch := false
	mismatchWarning := ""
	if contentFile != "" && s.osInfo.Version != "" {
		osVersion := strings.ReplaceAll(s.osInfo.Version, ".", "")
		baseName := filepath.Base(contentFile)
		if !strings.Contains(baseName, osVersion) {
			contentMismatch = true
			if ssgNeedsUpgrade {
				mismatchWarning = ssgUpgradeMessage
			} else {
				mismatchWarning = fmt.Sprintf("Content file %s may not match OS version %s.", baseName, s.osInfo.Version)
			}
		}
	} else if contentFile == "" && baseOSName == "ubuntu" && s.osInfo.Version >= "24.04" {
		contentMismatch = true
		mismatchWarning = ssgUpgradeMessage
		if mismatchWarning == "" {
			mismatchWarning = "No SCAP content found for Ubuntu 24.04."
		}
	}

	// Discover available profiles dynamically
	profiles := s.DiscoverProfiles()

	// Determine content package source
	contentPackage := fmt.Sprintf("ssg-base %s", contentVersion)
	githubVersion := s.getInstalledSSGVersion()
	if githubVersion != "" {
		contentPackage = fmt.Sprintf("SSG %s (GitHub)", githubVersion)
	}

	return &models.ComplianceScannerDetails{
		OpenSCAPVersion:   s.version,
		OpenSCAPAvailable: s.available,
		ContentFile:       filepath.Base(contentFile),
		ContentPackage:    contentPackage,
		SSGVersion:        contentVersion,
		SSGMinVersion:     minVersion,
		SSGNeedsUpgrade:   ssgNeedsUpgrade,
		SSGUpgradeMessage: ssgUpgradeMessage,
		AvailableProfiles: profiles,
		OSName:            s.osInfo.Name,
		OSVersion:         s.osInfo.Version,
		OSFamily:          s.osInfo.Family,
		ContentMismatch:   contentMismatch,
		MismatchWarning:   mismatchWarning,
	}
}

// compareVersions compares two semantic version strings
// Returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			if _, err := fmt.Sscanf(parts1[i], "%d", &n1); err != nil {
				// If parsing fails, treat as 0
				n1 = 0
			}
		}
		if i < len(parts2) {
			if _, err := fmt.Sscanf(parts2[i], "%d", &n2); err != nil {
				// If parsing fails, treat as 0
				n2 = 0
			}
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}

// EnsureInstalled installs OpenSCAP and SCAP content if not present
// Also upgrades existing packages to ensure latest content is available
func (s *OpenSCAPScanner) EnsureInstalled() error {
	s.logger.Info("Ensuring OpenSCAP is installed with latest SCAP content...")

	// Create context with timeout for package operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Environment for non-interactive apt operations
	nonInteractiveEnv := append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
		"NEEDRESTART_SUSPEND=1",
	)

	switch s.osInfo.Family {
	case "debian":
		// Ubuntu/Debian - always update and upgrade to get latest content
		s.logger.Info("Installing/upgrading OpenSCAP on Debian-based system...")

		// Check if Ubuntu 24.04+ (Noble Numbat) - also check Ubuntu-based distros like Pop!_OS
		baseOSName := s.getContentOSName()
		isUbuntu2404Plus := (s.osInfo.Name == "ubuntu" || baseOSName == "ubuntu") && s.osInfo.Version >= "24.04"
		if isUbuntu2404Plus {
			s.logger.Info("Ubuntu 24.04+ detected: CIS/STIG content requires ssg-base >= 0.1.76 or Canonical's Ubuntu Security Guide (USG)")

			// Check current version and auto-upgrade if needed
			currentVersion := s.GetContentPackageVersion()
			if currentVersion != "" {
				if compareVersions(currentVersion, "0.1.76") < 0 {
					s.logger.Info("ssg-base version is below 0.1.76, attempting to upgrade from GitHub...")
					if upgradeErr := s.UpgradeSSGContent(); upgradeErr != nil {
						s.logger.WithError(upgradeErr).Warn("Failed to auto-upgrade SSG content from GitHub. Manual upgrade recommended.")
					} else {
						s.logger.Info("SSG content successfully upgraded from GitHub")
						// Re-check version after upgrade
						newVersion := s.GetContentPackageVersion()
						if newVersion != "" && compareVersions(newVersion, "0.1.76") >= 0 {
							s.logger.WithField("new_version", newVersion).Info("SSG content upgraded successfully")
						}
					}
				} else {
					s.logger.WithField("version", currentVersion).Debug("SSG content version is sufficient")
				}
			} else {
				// No version detected - try GitHub upgrade
				s.logger.Info("No SSG version detected, attempting to install from GitHub...")
				if upgradeErr := s.UpgradeSSGContent(); upgradeErr != nil {
					s.logger.WithError(upgradeErr).Warn("Failed to install SSG content from GitHub")
				}
			}
		}

		// Update package cache first (with timeout)
		updateCmd := exec.CommandContext(ctx, "apt-get", "update", "-qq")
		updateCmd.Env = nonInteractiveEnv
		if err := updateCmd.Run(); err != nil {
			// Ignore errors on update - non-critical
			_ = err
		}

		// Build package list - openscap-common is required for Ubuntu 24.04+
		packages := []string{"openscap-scanner", "openscap-common"}

		// Try to install SSG content packages (may not be available for newer Ubuntu)
		ssgPackages := []string{"ssg-debderived", "ssg-base"}

		// Install core OpenSCAP packages first
		installArgs := append([]string{"install", "-y", "-qq",
			"-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold"}, packages...)
		installCmd := exec.CommandContext(ctx, "apt-get", installArgs...)
		installCmd.Env = nonInteractiveEnv
		output, err := installCmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				s.logger.Warn("OpenSCAP installation timed out after 5 minutes")
				return fmt.Errorf("installation timed out after 5 minutes")
			}
			s.logger.WithError(err).WithField("output", string(output)).Warn("Failed to install OpenSCAP core packages")
			// Truncate output for error message
			outputStr := string(output)
			if len(outputStr) > 500 {
				outputStr = outputStr[:500] + "... (truncated)"
			}
			return fmt.Errorf("failed to install OpenSCAP: %w - %s", err, outputStr)
		}
		s.logger.Info("OpenSCAP core packages installed successfully")

		// Try to install SSG content packages (best effort - may fail on Ubuntu 24.04+)
		ssgArgs := append([]string{"install", "-y", "-qq",
			"-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold"}, ssgPackages...)
		ssgCmd := exec.CommandContext(ctx, "apt-get", ssgArgs...)
		ssgCmd.Env = nonInteractiveEnv
		ssgOutput, ssgErr := ssgCmd.CombinedOutput()
		if ssgErr != nil {
			s.logger.WithField("output", string(ssgOutput)).Warn("SSG content packages not available or failed to install. CIS scanning may have limited functionality.")
			if isUbuntu2404Plus {
				s.logger.Info("For Ubuntu 24.04+, consider using Canonical's Ubuntu Security Guide (USG) with Ubuntu Pro for official CIS benchmarks.")
			}
		} else {
			s.logger.Info("SSG content packages installed successfully")

			// Explicitly upgrade to ensure we have the latest SCAP content
			upgradeCmd := exec.CommandContext(ctx, "apt-get", "upgrade", "-y", "-qq",
				"-o", "Dpkg::Options::=--force-confdef",
				"-o", "Dpkg::Options::=--force-confold",
				"ssg-base", "ssg-debderived")
			upgradeCmd.Env = nonInteractiveEnv
			upgradeOutput, upgradeErr := upgradeCmd.CombinedOutput()
			if upgradeErr != nil {
				s.logger.WithField("output", string(upgradeOutput)).Debug("Package upgrade returned non-zero (may already be latest)")
			} else {
				s.logger.Info("SCAP content packages upgraded to latest version")
			}
		}

	case "rhel":
		// RHEL/CentOS/Rocky/Alma/Fedora
		s.logger.Info("Installing/upgrading OpenSCAP on RHEL-based system...")
		var installCmd *exec.Cmd
		if _, err := exec.LookPath("dnf"); err == nil {
			installCmd = exec.CommandContext(ctx, "dnf", "install", "-y", "-q", "openscap-scanner", "scap-security-guide")
		} else {
			installCmd = exec.CommandContext(ctx, "yum", "install", "-y", "-q", "openscap-scanner", "scap-security-guide")
		}
		output, err := installCmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				s.logger.Warn("OpenSCAP installation timed out after 5 minutes")
				return fmt.Errorf("installation timed out after 5 minutes")
			}
			s.logger.WithError(err).WithField("output", string(output)).Warn("Failed to install OpenSCAP")
			outputStr := string(output)
			if len(outputStr) > 500 {
				outputStr = outputStr[:500] + "... (truncated)"
			}
			return fmt.Errorf("failed to install OpenSCAP: %w - %s", err, outputStr)
		}

	case "suse":
		// SLES/openSUSE
		s.logger.Info("Installing/upgrading OpenSCAP on SUSE-based system...")
		installCmd := exec.CommandContext(ctx, "zypper", "--non-interactive", "install", "openscap-utils", "scap-security-guide")
		output, err := installCmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				s.logger.Warn("OpenSCAP installation timed out after 5 minutes")
				return fmt.Errorf("installation timed out after 5 minutes")
			}
			s.logger.WithError(err).WithField("output", string(output)).Warn("Failed to install OpenSCAP")
			outputStr := string(output)
			if len(outputStr) > 500 {
				outputStr = outputStr[:500] + "... (truncated)"
			}
			return fmt.Errorf("failed to install OpenSCAP: %w - %s", err, outputStr)
		}

	default:
		return fmt.Errorf("unsupported OS family: %s (OS: %s)", s.osInfo.Family, s.osInfo.Name)
	}

	s.logger.Info("OpenSCAP installed/upgraded successfully")

	// Re-check availability after installation
	s.checkAvailability()
	if !s.available {
		return fmt.Errorf("OpenSCAP installed but still not available - content files may be missing")
	}

	// Check for content version mismatch
	s.checkContentCompatibility()

	return nil
}

// checkContentCompatibility checks if the SCAP content is compatible with the OS version
func (s *OpenSCAPScanner) checkContentCompatibility() {
	contentFile := s.getContentFile()
	if contentFile == "" {
		s.logger.Warn("No SCAP content file found - compliance scans will not work correctly")
		return
	}

	// Extract version from content file name (e.g., ssg-ubuntu2204-ds.xml -> 22.04)
	baseName := filepath.Base(contentFile)

	// Log detected content file
	s.logger.WithFields(logrus.Fields{
		"os_name":      s.osInfo.Name,
		"os_version":   s.osInfo.Version,
		"content_file": baseName,
	}).Debug("Checking SCAP content compatibility")

	// Check if content file matches OS version
	osVersion := strings.ReplaceAll(s.osInfo.Version, ".", "")
	expectedPattern := fmt.Sprintf("ssg-%s%s", s.osInfo.Name, osVersion)

	if !strings.Contains(baseName, osVersion) && !strings.HasPrefix(baseName, expectedPattern) {
		s.logger.WithFields(logrus.Fields{
			"os_version":   s.osInfo.Version,
			"content_file": baseName,
		}).Warn("SCAP content may not match OS version - scan results may show many 'notapplicable' rules. Consider updating ssg-base package.")
	}
}

// UpgradeSSGContent upgrades the SCAP Security Guide content from GitHub releases
func (s *OpenSCAPScanner) UpgradeSSGContent() error {
	s.logger.Info("Upgrading SCAP Security Guide content from GitHub...")

	// Download and install from GitHub
	if err := s.installSSGFromGitHub(); err != nil {
		s.logger.WithError(err).Warn("Failed to install SSG from GitHub")
		return err
	}

	// Re-check availability after upgrade
	s.checkAvailability()
	s.checkContentCompatibility()

	// Verify the new version
	newVersion := s.getInstalledSSGVersion()
	s.logger.WithField("version", newVersion).Info("SSG content upgrade completed")

	return nil
}

// installSSGFromGitHub downloads and installs SSG content from GitHub releases
func (s *OpenSCAPScanner) installSSGFromGitHub() error {
	// Latest stable version - update this periodically
	const ssgVersion = "0.1.79"
	const ssgURL = "https://github.com/ComplianceAsCode/content/releases/download/v" + ssgVersion + "/scap-security-guide-" + ssgVersion + ".zip"

	s.logger.WithFields(map[string]interface{}{
		"version": ssgVersion,
		"url":     ssgURL,
	}).Info("Downloading SSG from GitHub...")

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ssg-upgrade-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			// Log cleanup errors but don't fail
			_ = err
		}
	}()

	zipPath := filepath.Join(tmpDir, "ssg.zip")

	// Download the zip file
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := s.downloadFile(ctx, ssgURL, zipPath); err != nil {
		return fmt.Errorf("failed to download SSG: %w", err)
	}

	s.logger.Info("Extracting SSG content...")

	// Extract the zip file
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := s.extractZip(zipPath, extractDir); err != nil {
		return fmt.Errorf("failed to extract SSG: %w", err)
	}

	// Find the content directory in the extracted files
	contentSrcDir := filepath.Join(extractDir, "scap-security-guide-"+ssgVersion)
	if _, err := os.Stat(contentSrcDir); os.IsNotExist(err) {
		// Try without version suffix
		entries, _ := os.ReadDir(extractDir)
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "scap-security-guide") {
				contentSrcDir = filepath.Join(extractDir, e.Name())
				break
			}
		}
	}

	// Ensure target directory exists
	targetDir := scapContentDir
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create content directory: %w", err)
	}

	// Copy all XML files (datastream files) to the target directory
	s.logger.WithField("target", targetDir).Info("Installing SSG content files...")

	xmlFiles, err := filepath.Glob(filepath.Join(contentSrcDir, "*.xml"))
	if err != nil {
		return fmt.Errorf("failed to find XML files: %w", err)
	}

	if len(xmlFiles) == 0 {
		// Try looking in subdirectories
		xmlFiles, _ = filepath.Glob(filepath.Join(contentSrcDir, "*", "*.xml"))
	}

	copiedCount := 0
	for _, src := range xmlFiles {
		baseName := filepath.Base(src)
		// Only copy datastream files (ssg-*-ds.xml)
		if strings.HasPrefix(baseName, "ssg-") && strings.HasSuffix(baseName, "-ds.xml") {
			dst := filepath.Join(targetDir, baseName)
			if err := s.copyFile(src, dst); err != nil {
				s.logger.WithError(err).WithField("file", baseName).Warn("Failed to copy content file")
			} else {
				copiedCount++
			}
		}
	}

	if copiedCount == 0 {
		return fmt.Errorf("no SSG content files were installed")
	}

	s.logger.WithField("files_installed", copiedCount).Info("SSG content files installed successfully")

	// Create a version marker file
	versionFile := filepath.Join(targetDir, ".ssg-version")
	if err := os.WriteFile(versionFile, []byte(ssgVersion+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write version marker: %w", err)
	}

	return nil
}

// downloadFile downloads a file from a URL
func (s *OpenSCAPScanner) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log cleanup errors but don't fail
			_ = err
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			// Log cleanup errors but don't fail
			_ = err
		}
	}()

	_, err = io.Copy(out, resp.Body)
	return err
}

// extractZip extracts a zip file to a directory
func (s *OpenSCAPScanner) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			// Log cleanup errors but don't fail
			_ = err
		}
	}()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			if closeErr := outFile.Close(); closeErr != nil {
				_ = closeErr
			}
			return err
		}

		_, err = io.Copy(outFile, rc)
		if closeErr := outFile.Close(); closeErr != nil {
			_ = closeErr
		}
		if closeErr := rc.Close(); closeErr != nil {
			_ = closeErr
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func (s *OpenSCAPScanner) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			_ = err
		}
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			_ = err
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return out.Chmod(0644)
}

// getInstalledSSGVersion reads the version from the marker file
func (s *OpenSCAPScanner) getInstalledSSGVersion() string {
	versionFile := filepath.Join(scapContentDir, ".ssg-version")
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// checkAvailability checks if OpenSCAP is installed and has content
func (s *OpenSCAPScanner) checkAvailability() {
	// Check if oscap binary exists
	path, err := exec.LookPath(oscapBinary)
	if err != nil {
		s.logger.Debug("OpenSCAP binary not found")
		s.available = false
		return
	}
	s.logger.WithField("path", path).Debug("Found OpenSCAP binary")

	// Get version
	cmd := exec.Command(oscapBinary, "--version")
	output, err := cmd.Output()
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get OpenSCAP version")
		s.available = false
		return
	}

	// Parse version from output
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		s.version = strings.TrimSpace(lines[0])
	}

	// Check if SCAP content exists
	contentFile := s.getContentFile()
	if contentFile == "" {
		s.logger.Debug("No SCAP content files found")
		s.available = false
		return
	}

	s.available = true
	s.logger.WithFields(logrus.Fields{
		"version": s.version,
		"content": contentFile,
	}).Debug("OpenSCAP is available")
}

// detectOS detects the operating system
func (s *OpenSCAPScanner) detectOS() models.ComplianceOSInfo {
	info := models.ComplianceOSInfo{}

	file, err := os.Open(osReleasePath)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to open os-release")
		return info
	}
	defer func() {
		if err := file.Close(); err != nil {
			_ = err
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := strings.Trim(parts[1], "\"")

		switch key {
		case "ID":
			info.Name = value
		case "VERSION_ID":
			info.Version = value
		case "ID_LIKE":
			// Store ID_LIKE for base distribution detection
			s.idLike = value
			// Determine family from ID_LIKE
			if strings.Contains(value, "debian") {
				info.Family = "debian"
			} else if strings.Contains(value, "rhel") || strings.Contains(value, "fedora") {
				info.Family = "rhel"
			} else if strings.Contains(value, "suse") {
				info.Family = "suse"
			}
		}
	}

	// Set family from ID if not set from ID_LIKE
	if info.Family == "" {
		switch info.Name {
		case "ubuntu", "debian":
			info.Family = "debian"
		case "rhel", "centos", "rocky", "alma", "fedora":
			info.Family = "rhel"
		case "sles", "opensuse", "opensuse-leap":
			info.Family = "suse"
		}
	}

	return info
}

// getContentOSName determines the base distribution name for SCAP content file lookup
// Uses ID_LIKE from /etc/os-release to automatically detect Ubuntu/Debian/RHEL-based distributions
func (s *OpenSCAPScanner) getContentOSName() string {
	// Known base distributions that have SCAP content files
	baseDistributions := []string{"ubuntu", "debian", "rhel", "centos", "rocky", "alma", "fedora", "sles", "opensuse"}

	// First, check if the OS name itself is a base distribution
	for _, base := range baseDistributions {
		if s.osInfo.Name == base {
			return s.osInfo.Name
		}
	}

	// If not, check ID_LIKE for base distributions
	// ID_LIKE typically contains space-separated values like "ubuntu debian" or "rhel fedora"
	if s.idLike != "" {
		idLikeParts := strings.Fields(s.idLike)
		for _, part := range idLikeParts {
			for _, base := range baseDistributions {
				if part == base {
					return base
				}
			}
		}
	}

	// Fallback to original OS name if no base distribution found
	return s.osInfo.Name
}

// getContentFile returns the appropriate SCAP content file for this OS
func (s *OpenSCAPScanner) getContentFile() string {
	if s.osInfo.Name == "" {
		return ""
	}

	// Get the base distribution name for content file lookup
	contentOSName := s.getContentOSName()

	// Build possible content file names
	patterns := []string{
		fmt.Sprintf("ssg-%s%s-ds.xml", contentOSName, strings.ReplaceAll(s.osInfo.Version, ".", "")),
		fmt.Sprintf("ssg-%s%s-ds.xml", contentOSName, strings.Split(s.osInfo.Version, ".")[0]),
		fmt.Sprintf("ssg-%s-ds.xml", contentOSName),
	}

	// Check each pattern
	for _, pattern := range patterns {
		path := filepath.Join(scapContentDir, pattern)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try to find any matching file
	matches, err := filepath.Glob(filepath.Join(scapContentDir, fmt.Sprintf("ssg-%s*-ds.xml", contentOSName)))
	if err == nil && len(matches) > 0 {
		return matches[0]
	}

	// If still not found and we normalized to a base distribution, try the original OS name as fallback
	if contentOSName != s.osInfo.Name {
		patterns = []string{
			fmt.Sprintf("ssg-%s%s-ds.xml", s.osInfo.Name, strings.ReplaceAll(s.osInfo.Version, ".", "")),
			fmt.Sprintf("ssg-%s%s-ds.xml", s.osInfo.Name, strings.Split(s.osInfo.Version, ".")[0]),
			fmt.Sprintf("ssg-%s-ds.xml", s.osInfo.Name),
		}
		for _, pattern := range patterns {
			path := filepath.Join(scapContentDir, pattern)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	return ""
}

// GetAvailableProfiles returns available CIS profiles for this system
func (s *OpenSCAPScanner) GetAvailableProfiles() []string {
	profiles := make([]string, 0)

	if !s.available {
		return profiles
	}

	// Get the base distribution name for profile lookup
	profileOSName := s.getContentOSName()

	for profileName, osProfiles := range profileMappings {
		if _, exists := osProfiles[profileOSName]; exists {
			profiles = append(profiles, profileName)
		} else if profileOSName != s.osInfo.Name {
			// Fallback to original OS name
			if _, exists := osProfiles[s.osInfo.Name]; exists {
				profiles = append(profiles, profileName)
			}
		}
	}

	return profiles
}

// getProfileID returns the full profile ID for this OS
func (s *OpenSCAPScanner) getProfileID(profileName string) string {
	// If it's already a full XCCDF profile ID, use it directly
	if strings.HasPrefix(profileName, "xccdf_") {
		return profileName
	}

	// Get the base distribution name for profile lookup
	profileOSName := s.getContentOSName()

	// Otherwise, look up the mapping for this OS
	if osProfiles, exists := profileMappings[profileName]; exists {
		if profileID, exists := osProfiles[profileOSName]; exists {
			return profileID
		}
		// Fallback to original OS name if normalized name didn't work
		if profileOSName != s.osInfo.Name {
			if profileID, exists := osProfiles[s.osInfo.Name]; exists {
				return profileID
			}
		}
	}
	return ""
}

// RunScan executes an OpenSCAP scan (legacy method - calls RunScanWithOptions with defaults)
func (s *OpenSCAPScanner) RunScan(ctx context.Context, profileName string) (*models.ComplianceScan, error) {
	return s.RunScanWithOptions(ctx, &models.ComplianceScanOptions{
		ProfileID: profileName,
	})
}

// RunScanWithOptions executes an OpenSCAP scan with configurable options
func (s *OpenSCAPScanner) RunScanWithOptions(ctx context.Context, options *models.ComplianceScanOptions) (*models.ComplianceScan, error) {
	if !s.available {
		return nil, fmt.Errorf("OpenSCAP is not available")
	}

	startTime := time.Now()

	contentFile := s.getContentFile()
	if contentFile == "" {
		return nil, fmt.Errorf("no SCAP content file found for %s %s", s.osInfo.Name, s.osInfo.Version)
	}

	profileID := s.getProfileID(options.ProfileID)
	if profileID == "" {
		return nil, fmt.Errorf("profile %s not available for %s", options.ProfileID, s.osInfo.Name)
	}

	// Create temp file for results
	resultsFile, err := os.CreateTemp("", "oscap-results-*.xml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	resultsPath := resultsFile.Name()
	if err := resultsFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close results file: %w", err)
	}
	defer func() {
		if err := os.Remove(resultsPath); err != nil && !os.IsNotExist(err) {
			_ = err
		}
	}()

	// Create temp file for OVAL results (contains detailed check data)
	ovalResultsFile, err := os.CreateTemp("", "oscap-oval-*.xml")
	if err != nil {
		return nil, fmt.Errorf("failed to create oval temp file: %w", err)
	}
	ovalResultsPath := ovalResultsFile.Name()
	if err := ovalResultsFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close OVAL results file: %w", err)
	}
	defer func() {
		if err := os.Remove(ovalResultsPath); err != nil && !os.IsNotExist(err) {
			_ = err
		}
	}()

	// Build command arguments
	args := []string{
		"xccdf", "eval",
		"--profile", profileID,
		"--results", resultsPath,
		"--oval-results",         // Generate detailed OVAL results with actual values
		"--check-engine-results", // Force evaluation even if CPE doesn't match perfectly
	}

	// Add optional arguments based on options
	if options.EnableRemediation {
		args = append(args, "--remediate")
		s.logger.Info("Remediation enabled - will attempt to fix failed rules")
	}

	// Add rule filter for single rule remediation
	if options.RuleID != "" {
		args = append(args, "--rule", options.RuleID)
		s.logger.WithField("rule_id", options.RuleID).Info("Filtering scan to single rule")
	}

	if options.FetchRemoteResources {
		args = append(args, "--fetch-remote-resources")
	}

	if options.TailoringFile != "" {
		args = append(args, "--tailoring-file", options.TailoringFile)
	}

	// Add ARF output if requested
	if options.OutputFormat == "arf" {
		arfFile, err := os.CreateTemp("", "oscap-arf-*.xml")
		if err == nil {
			arfPath := arfFile.Name()
			if err := arfFile.Close(); err != nil {
				return nil, fmt.Errorf("failed to close ARF file: %w", err)
			}
			defer func() {
				if err := os.Remove(arfPath); err != nil && !os.IsNotExist(err) {
					_ = err
				}
			}()
			args = append(args, "--results-arf", arfPath)
		}
	}

	// Add content file last
	args = append(args, contentFile)

	s.logger.WithFields(logrus.Fields{
		"profile":     options.ProfileID,
		"profile_id":  profileID,
		"content":     contentFile,
		"remediation": options.EnableRemediation,
	}).Info("Starting OpenSCAP scan (this may take several minutes)...")

	// Run oscap with progress logging
	cmd := exec.CommandContext(ctx, oscapBinary, args...)

	// Start a goroutine to log progress every 30 seconds
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed += 30
				s.logger.WithField("elapsed_seconds", elapsed).Info("OpenSCAP scan still running...")
			}
		}
	}()

	output, err := cmd.CombinedOutput()
	close(done)

	elapsed := time.Since(startTime)
	s.logger.WithFields(logrus.Fields{
		"elapsed_seconds": elapsed.Seconds(),
		"results_path":    resultsPath,
		"output_length":   len(output),
	}).Info("OpenSCAP command completed")

	// Check if results file exists and has content
	if fileInfo, statErr := os.Stat(resultsPath); statErr == nil {
		s.logger.WithFields(logrus.Fields{
			"results_file_size": fileInfo.Size(),
			"results_file_path": resultsPath,
		}).Info("Results file exists")
	} else {
		s.logger.WithError(statErr).Warn("Results file does not exist or cannot be accessed")
	}

	// oscap returns non-zero exit code if there are failures, which is expected
	// We only care about actual execution errors
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("scan cancelled or timed out: %w", ctx.Err())
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 or 2 means there were rule failures - this is normal
			if exitErr.ExitCode() != 2 && exitErr.ExitCode() != 1 {
				// Truncate output for error message (keep first 500 chars)
				outputStr := string(output)
				if len(outputStr) > 500 {
					outputStr = outputStr[:500] + "... (truncated)"
				}
				return nil, fmt.Errorf("oscap execution failed (exit code %d): %s", exitErr.ExitCode(), outputStr)
			}
		} else {
			// Other errors (like signal killed)
			return nil, fmt.Errorf("oscap execution failed: %w", err)
		}
	}

	// Verify results file was written
	if fileInfo, statErr := os.Stat(resultsPath); statErr == nil {
		if fileInfo.Size() == 0 {
			s.logger.Warn("Results file is empty - scan may not have run correctly")
			// Log first part of oscap output for debugging
			outputPreview := string(output)
			if len(outputPreview) > 1000 {
				outputPreview = outputPreview[:1000] + "... (truncated)"
			}
			s.logger.WithField("oscap_output_preview", outputPreview).Debug("OpenSCAP output preview")
		} else {
			s.logger.WithField("results_file_size_bytes", fileInfo.Size()).Debug("Results file has content")
		}
	} else {
		s.logger.WithError(statErr).Error("Results file does not exist after scan completion")
		return nil, fmt.Errorf("results file not found: %w", statErr)
	}

	// Parse results (pass oscap output and content file for metadata)
	scan, err := s.parseResults(resultsPath, contentFile, options.ProfileID, string(output))
	if err != nil {
		return nil, fmt.Errorf("failed to parse results: %w", err)
	}

	// Log summary of parsed results for debugging
	s.logger.WithFields(logrus.Fields{
		"total_rules":    scan.TotalRules,
		"passed":         scan.Passed,
		"failed":         scan.Failed,
		"skipped":        scan.Skipped,
		"not_applicable": scan.NotApplicable,
		"warnings":       scan.Warnings,
	}).Debug("Parsed scan results summary")

	scan.StartedAt = startTime
	now := time.Now()
	scan.CompletedAt = &now
	scan.Status = "completed"
	scan.RemediationApplied = options.EnableRemediation

	return scan, nil
}

// GenerateRemediationScript generates a shell script to fix failed rules
func (s *OpenSCAPScanner) GenerateRemediationScript(ctx context.Context, resultsPath string, outputPath string) error {
	if !s.available {
		return fmt.Errorf("OpenSCAP is not available")
	}

	args := []string{
		"xccdf", "generate", "fix",
		"--template", "urn:xccdf:fix:script:sh",
		"--output", outputPath,
		resultsPath,
	}

	s.logger.WithField("output", outputPath).Debug("Generating remediation script")

	cmd := exec.CommandContext(ctx, oscapBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Truncate output for error message
		outputStr := string(output)
		if len(outputStr) > 500 {
			outputStr = outputStr[:500] + "... (truncated)"
		}
		return fmt.Errorf("failed to generate remediation script: %w - %s", err, outputStr)
	}

	s.logger.WithField("output", outputPath).Info("Remediation script generated")
	return nil
}

// RunOfflineRemediation applies fixes from a previous scan result
func (s *OpenSCAPScanner) RunOfflineRemediation(ctx context.Context, resultsPath string) error {
	if !s.available {
		return fmt.Errorf("OpenSCAP is not available")
	}

	contentFile := s.getContentFile()
	if contentFile == "" {
		return fmt.Errorf("no SCAP content file found")
	}

	args := []string{
		"xccdf", "remediate",
		"--results", resultsPath,
		contentFile,
	}

	s.logger.WithField("results", resultsPath).Info("Running offline remediation")

	cmd := exec.CommandContext(ctx, oscapBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Non-zero exit is expected if some remediations fail
			if exitErr.ExitCode() > 2 {
				// Truncate output for error message
				outputStr := string(output)
				if len(outputStr) > 500 {
					outputStr = outputStr[:500] + "... (truncated)"
				}
				return fmt.Errorf("remediation failed (exit code %d): %s", exitErr.ExitCode(), outputStr)
			}
		} else {
			return fmt.Errorf("remediation execution failed: %w", err)
		}
	}

	s.logger.Info("Offline remediation completed")
	return nil
}

// XCCDF result structures for parsing
type xccdfTestResult struct {
	XMLName xml.Name          `xml:"TestResult"`
	Rules   []xccdfRuleResult `xml:"rule-result"`
}

type xccdfRuleResult struct {
	IDRef  string `xml:"idref,attr"`
	Result string `xml:"result"`
}

// ruleMetadata holds extracted rule information from the benchmark
type ruleMetadata struct {
	Title       string
	Description string
	Rationale   string
	Severity    string
	Remediation string
	Section     string
}

// parseResults parses the XCCDF results file and extracts rich metadata from the benchmark
func (s *OpenSCAPScanner) parseResults(resultsPath string, contentFile string, profileName string, oscapOutput string) (*models.ComplianceScan, error) {
	data, err := os.ReadFile(resultsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read results: %w", err)
	}

	resultsContent := string(data)

	// Extract TestResult section (simplified parsing)
	scan := &models.ComplianceScan{
		ProfileName: profileName,
		ProfileType: "openscap",
		Results:     make([]models.ComplianceResult, 0),
	}

	// Extract rule metadata from the BENCHMARK file (not results file)
	// The benchmark file (ssg-*-ds.xml) contains Rule definitions with title, description, etc.
	benchmarkContent := ""
	if contentFile != "" {
		if benchmarkData, err := os.ReadFile(contentFile); err == nil {
			benchmarkContent = string(benchmarkData)
			s.logger.WithField("content_file", contentFile).Debug("Loaded benchmark file for metadata extraction")
		} else {
			s.logger.WithError(err).Warn("Failed to read benchmark file for metadata")
		}
	}

	// Try results file first (might have embedded benchmark), then fall back to benchmark file
	s.logger.WithFields(map[string]interface{}{
		"results_content_len":   len(resultsContent),
		"benchmark_content_len": len(benchmarkContent),
	}).Info("Starting metadata extraction")

	ruleMetadataMap := s.extractRuleMetadata(resultsContent)
	s.logger.WithField("rules_from_results", len(ruleMetadataMap)).Info("Extracted metadata from results file")

	if len(ruleMetadataMap) == 0 && benchmarkContent != "" {
		s.logger.Info("No metadata in results file, extracting from benchmark datastream")
		ruleMetadataMap = s.extractRuleMetadata(benchmarkContent)
		s.logger.WithField("rules_from_benchmark", len(ruleMetadataMap)).Info("Extracted metadata from benchmark file")
	}

	// Parse oscap output for rule-specific failure details
	// oscap output format: "Title	rule_id	result"
	// For failures, additional detail lines follow
	ruleOutputMap := s.parseOscapOutput(oscapOutput)

	// Parse rule results with optional message element
	// Pattern captures: idref, full rule-result block content
	ruleResultPattern := regexp.MustCompile(`<rule-result[^>]*idref="([^"]+)"[^>]*>([\s\S]*?)</rule-result>`)
	resultPattern := regexp.MustCompile(`<result>([^<]+)</result>`)
	messagePattern := regexp.MustCompile(`<message[^>]*>([^<]+)</message>`)

	matches := ruleResultPattern.FindAllStringSubmatch(resultsContent, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			ruleID := match[1]
			ruleResultContent := match[2]

			// Extract result status
			resultMatch := resultPattern.FindStringSubmatch(ruleResultContent)
			if len(resultMatch) < 2 {
				continue
			}
			result := strings.TrimSpace(resultMatch[1])
			status := s.mapResult(result)

			// Extract message if present (contains specific check output for failures)
			var finding string
			messageMatch := messagePattern.FindStringSubmatch(ruleResultContent)
			if len(messageMatch) >= 2 {
				finding = strings.TrimSpace(messageMatch[1])
			}

			// If no finding from XML, try to get from oscap output
			if finding == "" && status == "fail" {
				if outputInfo, ok := ruleOutputMap[ruleID]; ok {
					finding = outputInfo
				}
			}

			// Update counters
			switch status {
			case "pass":
				scan.Passed++
			case "fail":
				scan.Failed++
			case "warn":
				scan.Warnings++
			case "skip":
				scan.Skipped++
			case "notapplicable":
				scan.NotApplicable++
			}
			scan.TotalRules++

			// Get metadata from embedded benchmark
			metadata := ruleMetadataMap[ruleID]

			// Use extracted title or fall back to generated one
			title := metadata.Title
			if title == "" {
				title = s.extractTitle(ruleID)
			}

			// Extract actual/expected from finding if possible
			actual, expected := s.parseActualExpected(finding, metadata.Description)

			scan.Results = append(scan.Results, models.ComplianceResult{
				RuleID:      ruleID,
				Title:       title,
				Status:      status,
				Finding:     finding,
				Actual:      actual,
				Expected:    expected,
				Description: metadata.Description,
				Severity:    metadata.Severity,
				Remediation: metadata.Remediation,
				Section:     metadata.Section,
			})

			// Debug logging for result assembly (only for failed rules to reduce noise)
			if status == "fail" {
				s.logger.WithFields(map[string]interface{}{
					"rule_id":         ruleID,
					"title":           title,
					"status":          status,
					"has_description": len(metadata.Description) > 0,
					"desc_len":        len(metadata.Description),
					"has_remediation": len(metadata.Remediation) > 0,
					"severity":        metadata.Severity,
				}).Debug("Assembled failed rule result")
			}
		}
	}

	// Check if all rules are notapplicable/skip - this usually indicates a CPE/platform mismatch
	if scan.TotalRules > 0 && scan.Passed == 0 && scan.Failed == 0 && (scan.NotApplicable+scan.Skipped) == scan.TotalRules {
		baseOSName := s.getContentOSName()
		warningMsg := fmt.Sprintf("All rules marked as notapplicable/skip - CPE/platform mismatch detected. System '%s' does not match benchmark target platform '%s'. OpenSCAP requires exact CPE matching to evaluate rules. For Ubuntu-based distributions like Pop!_OS, consider: 1) Using Ubuntu directly, 2) Using Canonical's Ubuntu Security Guide (USG) with Ubuntu Pro, or 3) Accepting that compliance scanning has limited functionality on derivative distributions.", s.osInfo.Name, baseOSName)
		s.logger.Warn(warningMsg)

		// Set error message in scan so UI can display it
		if scan.Error == "" {
			scan.Error = "CPE/platform mismatch: System does not match benchmark target platform. All rules were marked as not applicable. This is expected behavior for Ubuntu-based distributions that aren't exactly Ubuntu (e.g., Pop!_OS)."
		}
	}

	// Calculate score
	if scan.TotalRules > 0 {
		applicable := scan.TotalRules - scan.NotApplicable - scan.Skipped
		if applicable > 0 {
			scan.Score = float64(scan.Passed) / float64(applicable) * 100
		}
	}

	return scan, nil
}

// parseOscapOutput extracts rule-specific information from oscap stdout
func (s *OpenSCAPScanner) parseOscapOutput(output string) map[string]string {
	ruleInfo := make(map[string]string)

	// oscap output contains lines like:
	// "Title\trule_id\tresult"
	// For failed rules, we want to capture any additional context
	lines := strings.Split(output, "\n")

	var currentRuleID string
	var currentDetails []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is a rule result line (contains rule ID pattern)
		if strings.Contains(line, "xccdf_org.ssgproject.content_rule_") {
			// Save previous rule's details if any
			if currentRuleID != "" && len(currentDetails) > 0 {
				ruleInfo[currentRuleID] = strings.Join(currentDetails, "; ")
			}

			// Extract rule ID from line
			rulePattern := regexp.MustCompile(`(xccdf_org\.ssgproject\.content_rule_[^\s\t]+)`)
			if match := rulePattern.FindStringSubmatch(line); len(match) >= 2 {
				currentRuleID = match[1]
				currentDetails = nil

				// Check if line contains failure indicator and additional info
				if strings.Contains(strings.ToLower(line), "fail") {
					// Look for any additional info after the status
					parts := strings.Split(line, "\t")
					if len(parts) > 3 {
						currentDetails = append(currentDetails, strings.Join(parts[3:], " "))
					}
				}
			}
		} else if currentRuleID != "" && !strings.HasPrefix(line, "Title") {
			// This might be additional detail for the current rule
			// Capture lines that look like check output (often start with paths or values)
			if strings.HasPrefix(line, "/") || strings.Contains(line, "=") || strings.Contains(line, ":") {
				currentDetails = append(currentDetails, line)
			}
		}
	}

	// Save last rule's details
	if currentRuleID != "" && len(currentDetails) > 0 {
		ruleInfo[currentRuleID] = strings.Join(currentDetails, "; ")
	}

	return ruleInfo
}

// parseActualExpected attempts to extract actual and expected values from finding text
func (s *OpenSCAPScanner) parseActualExpected(finding string, description string) (actual, expected string) {
	if finding == "" {
		return "", ""
	}

	// Common patterns in XCCDF findings:
	// "expected X but found Y"
	// "value is X, should be Y"
	// "X is set to Y"

	// Pattern: "expected ... but found ..."
	pattern1 := regexp.MustCompile(`(?i)expected\s+['"]?([^'"]+?)['"]?\s+but\s+found\s+['"]?([^'"]+?)['"]?`)
	if match := pattern1.FindStringSubmatch(finding); len(match) >= 3 {
		return match[2], match[1] // actual, expected
	}

	// Pattern: "found ... expected ..."
	pattern2 := regexp.MustCompile(`(?i)found\s+['"]?([^'"]+?)['"]?\s+expected\s+['"]?([^'"]+?)['"]?`)
	if match := pattern2.FindStringSubmatch(finding); len(match) >= 3 {
		return match[1], match[2]
	}

	// Pattern: "is set to X" (actual value)
	pattern3 := regexp.MustCompile(`(?i)is\s+set\s+to\s+['"]?([^'"]+?)['"]?`)
	if match := pattern3.FindStringSubmatch(finding); len(match) >= 2 {
		actual = match[1]
	}

	// Pattern: "should be X" (expected value)
	pattern4 := regexp.MustCompile(`(?i)should\s+be\s+['"]?([^'"]+?)['"]?`)
	if match := pattern4.FindStringSubmatch(finding); len(match) >= 2 {
		expected = match[1]
	}

	// Pattern: "value X" or "= X"
	pattern5 := regexp.MustCompile(`(?:value|=)\s*['"]?(\S+)['"]?`)
	if actual == "" {
		if match := pattern5.FindStringSubmatch(finding); len(match) >= 2 {
			actual = match[1]
		}
	}

	return actual, expected
}

// extractRuleMetadata extracts rule definitions from the embedded benchmark in XCCDF results
func (s *OpenSCAPScanner) extractRuleMetadata(content string) map[string]ruleMetadata {
	metadata := make(map[string]ruleMetadata)

	// Extract Rule elements using a more robust approach:
	// 1. Find all Rule opening tags and their positions
	// 2. Find the corresponding closing tag (handling nesting)
	// 3. Extract attributes and content separately

	// Pattern to match Rule opening tags with any attributes
	// Namespace prefix can be like "xccdf-1.2:" so we need to include dots and hyphens
	ruleOpenPattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?Rule\s+([^>]*)>`)
	idPattern := regexp.MustCompile(`id="([^"]+)"`)
	severityAttrPattern := regexp.MustCompile(`severity="([^"]*)"`)

	// Patterns for child elements (handle any namespace prefix including dots like xccdf-1.2:)
	titlePattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?title[^>]*>([^<]+)</([a-zA-Z0-9._-]*:)?title>`)
	descPattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?description[^>]*>([\s\S]*?)</([a-zA-Z0-9._-]*:)?description>`)
	rationalePattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?rationale[^>]*>([\s\S]*?)</([a-zA-Z0-9._-]*:)?rationale>`)
	// For fix elements, prefer shell script remediation (system="urn:xccdf:fix:script:sh")
	fixShPattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?fix[^>]*system="urn:xccdf:fix:script:sh"[^>]*>([\s\S]*?)</([a-zA-Z0-9._-]*:)?fix>`)
	fixPattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?fix[^>]*>([\s\S]*?)</([a-zA-Z0-9._-]*:)?fix>`)
	fixTextPattern := regexp.MustCompile(`<([a-zA-Z0-9._-]*:)?fixtext[^>]*>([\s\S]*?)</([a-zA-Z0-9._-]*:)?fixtext>`)

	// Find all Rule opening tags
	openMatches := ruleOpenPattern.FindAllStringSubmatchIndex(content, -1)

	for _, openMatch := range openMatches {
		if len(openMatch) < 6 {
			continue
		}

		tagStart := openMatch[0]
		tagEnd := openMatch[1]
		nsPrefix := ""
		if openMatch[2] >= 0 && openMatch[3] > openMatch[2] {
			nsPrefix = content[openMatch[2]:openMatch[3]]
		}
		attributes := content[openMatch[4]:openMatch[5]]

		// Extract id from attributes
		idMatch := idPattern.FindStringSubmatch(attributes)
		if len(idMatch) < 2 {
			continue
		}
		ruleID := idMatch[1]

		// Find the closing tag for this Rule element
		// Build the closing tag pattern based on namespace prefix
		closingTag := "</" + nsPrefix + "Rule>"
		openingTag := "<" + nsPrefix + "Rule"

		// Find closing tag, accounting for potential nested Rule elements
		ruleContent := ""
		depth := 1
		searchStart := tagEnd
		for depth > 0 && searchStart < len(content) {
			nextOpen := strings.Index(content[searchStart:], openingTag)
			nextClose := strings.Index(content[searchStart:], closingTag)

			if nextClose == -1 {
				// No closing tag found
				break
			}

			if nextOpen != -1 && nextOpen < nextClose {
				// Found another opening tag before closing
				depth++
				searchStart = searchStart + nextOpen + len(openingTag)
			} else {
				// Found closing tag
				depth--
				if depth == 0 {
					ruleContent = content[tagEnd : searchStart+nextClose]
				}
				searchStart = searchStart + nextClose + len(closingTag)
			}
		}

		// If nesting approach failed, try simpler non-greedy match
		if ruleContent == "" {
			// Look for closing tag within reasonable distance (500KB limit per rule)
			endIdx := tagStart + 500000
			if endIdx > len(content) {
				endIdx = len(content)
			}
			searchContent := content[tagEnd:endIdx]
			closeIdx := strings.Index(searchContent, closingTag)
			if closeIdx != -1 {
				ruleContent = searchContent[:closeIdx]
			}
		}

		if ruleContent == "" {
			s.logger.WithField("rule_id", ruleID).Debug("Could not find Rule content")
			continue
		}

		meta := ruleMetadata{}

		// Extract severity from attributes
		if sevMatch := severityAttrPattern.FindStringSubmatch(attributes); len(sevMatch) >= 2 {
			meta.Severity = sevMatch[1]
		}

		// Extract title - use the inner text (group 2)
		if titleMatch := titlePattern.FindStringSubmatch(ruleContent); len(titleMatch) >= 3 {
			meta.Title = s.cleanXMLText(titleMatch[2])
		}

		// Extract description - use the inner text (group 2)
		if descMatch := descPattern.FindStringSubmatch(ruleContent); len(descMatch) >= 3 {
			meta.Description = s.cleanXMLText(descMatch[2])
		}

		// Extract rationale (append to description if present)
		if ratMatch := rationalePattern.FindStringSubmatch(ruleContent); len(ratMatch) >= 3 {
			rationale := s.cleanXMLText(ratMatch[2])
			if rationale != "" {
				if meta.Description != "" {
					meta.Description = meta.Description + "\n\nRationale: " + rationale
				} else {
					meta.Description = "Rationale: " + rationale
				}
			}
		}

		// Extract fix/remediation - prefer shell script fix, then any fix, then fixtext
		if fixShMatch := fixShPattern.FindStringSubmatch(ruleContent); len(fixShMatch) >= 3 {
			meta.Remediation = s.cleanXMLText(fixShMatch[2])
		}
		if meta.Remediation == "" {
			if fixMatch := fixPattern.FindStringSubmatch(ruleContent); len(fixMatch) >= 3 {
				meta.Remediation = s.cleanXMLText(fixMatch[2])
			}
		}
		if meta.Remediation == "" {
			if fixTextMatch := fixTextPattern.FindStringSubmatch(ruleContent); len(fixTextMatch) >= 3 {
				meta.Remediation = s.cleanXMLText(fixTextMatch[2])
			}
		}

		// Extract section from rule ID (e.g., "1.1.1" from rule naming)
		meta.Section = s.extractSection(ruleID)

		metadata[ruleID] = meta

		// Debug logging for metadata extraction verification
		s.logger.WithFields(map[string]interface{}{
			"rule_id":         ruleID,
			"title":           meta.Title,
			"title_len":       len(meta.Title),
			"desc_len":        len(meta.Description),
			"desc_preview":    truncateString(meta.Description, 100),
			"remediation_len": len(meta.Remediation),
			"severity":        meta.Severity,
			"section":         meta.Section,
		}).Debug("Extracted rule metadata")
	}

	// Count rules with actual content for debugging
	withTitle := 0
	withDesc := 0
	withRemediation := 0
	for _, m := range metadata {
		if m.Title != "" {
			withTitle++
		}
		if m.Description != "" {
			withDesc++
		}
		if m.Remediation != "" {
			withRemediation++
		}
	}

	s.logger.WithFields(map[string]interface{}{
		"total_rules":      len(metadata),
		"with_title":       withTitle,
		"with_description": withDesc,
		"with_remediation": withRemediation,
	}).Info("Extracted rule metadata summary")

	return metadata
}

// cleanXMLText removes HTML/XML tags and cleans up whitespace
func (s *OpenSCAPScanner) cleanXMLText(text string) string {
	// Remove HTML tags
	htmlPattern := regexp.MustCompile(`<[^>]+>`)
	text = htmlPattern.ReplaceAllString(text, " ")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#xA;", "\n")
	text = strings.ReplaceAll(text, "&#10;", "\n")

	// Clean up whitespace
	whitespacePattern := regexp.MustCompile(`\s+`)
	text = whitespacePattern.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// truncateString truncates a string to maxLen characters for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// extractSection attempts to extract a section number from the rule ID
func (s *OpenSCAPScanner) extractSection(ruleID string) string {
	// Look for patterns like "1_1_1" or "1.1.1" in the rule ID
	sectionPattern := regexp.MustCompile(`(\d+[_\.]\d+(?:[_\.]\d+)*)`)
	if match := sectionPattern.FindString(ruleID); match != "" {
		// Convert underscores to dots for display
		return strings.ReplaceAll(match, "_", ".")
	}
	return ""
}

// mapResult maps XCCDF result to our status
func (s *OpenSCAPScanner) mapResult(result string) string {
	switch strings.ToLower(result) {
	case "pass":
		return "pass"
	case "fail":
		return "fail"
	case "error":
		return "fail"
	case "informational":
		return "warn"
	case "notselected", "notchecked":
		return "skip"
	case "notapplicable":
		return "notapplicable"
	default:
		return "skip"
	}
}

// extractTitle extracts a readable title from a rule ID
func (s *OpenSCAPScanner) extractTitle(ruleID string) string {
	// Remove prefix and convert underscores to spaces
	title := strings.TrimPrefix(ruleID, "xccdf_org.ssgproject.content_rule_")
	title = strings.ReplaceAll(title, "_", " ")

	// Capitalize first letter
	if len(title) > 0 {
		title = strings.ToUpper(title[:1]) + title[1:]
	}

	return title
}

// Cleanup removes OpenSCAP and related packages
// Note: This is optional - packages can be left installed if desired
func (s *OpenSCAPScanner) Cleanup() error {
	if !s.available {
		s.logger.Debug("OpenSCAP not installed, nothing to clean up")
		return nil
	}

	s.logger.Info("Removing OpenSCAP packages...")

	// Create context with timeout for package operations
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Environment for non-interactive apt operations
	nonInteractiveEnv := append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
		"NEEDRESTART_SUSPEND=1",
	)

	var removeCmd *exec.Cmd

	switch s.osInfo.Family {
	case "debian":
		removeCmd = exec.CommandContext(ctx, "apt-get", "remove", "-y", "-qq",
			"-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold",
			"openscap-scanner", "ssg-debderived", "ssg-base")
		removeCmd.Env = nonInteractiveEnv
	case "rhel":
		if _, err := exec.LookPath("dnf"); err == nil {
			removeCmd = exec.CommandContext(ctx, "dnf", "remove", "-y", "-q", "openscap-scanner", "scap-security-guide")
		} else {
			removeCmd = exec.CommandContext(ctx, "yum", "remove", "-y", "-q", "openscap-scanner", "scap-security-guide")
		}
	case "suse":
		removeCmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "remove", "openscap-utils", "scap-security-guide")
	default:
		s.logger.Debug("Unknown OS family, skipping package removal")
		return nil
	}

	output, err := removeCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			s.logger.Warn("OpenSCAP removal timed out after 3 minutes")
			return fmt.Errorf("removal timed out after 3 minutes")
		}
		s.logger.WithError(err).WithField("output", string(output)).Warn("Failed to remove OpenSCAP packages")
		// Don't return error - cleanup is best-effort
		return nil
	}

	s.logger.Info("OpenSCAP packages removed successfully")
	s.available = false
	s.version = ""

	return nil
}
