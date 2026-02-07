package compliance

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

const (
	oscapDockerBinary = "oscap-docker"
)

// OscapDockerScanner handles Docker image/container vulnerability scanning using oscap-docker
type OscapDockerScanner struct {
	logger    *logrus.Logger
	available bool
}

// NewOscapDockerScanner creates a new oscap-docker scanner
func NewOscapDockerScanner(logger *logrus.Logger) *OscapDockerScanner {
	s := &OscapDockerScanner{
		logger: logger,
	}
	s.checkAvailability()
	return s
}

// IsAvailable returns whether oscap-docker is available
func (s *OscapDockerScanner) IsAvailable() bool {
	return s.available
}

// checkAvailability checks if oscap-docker tool is available
func (s *OscapDockerScanner) checkAvailability() {
	// Check if oscap-docker binary exists
	path, err := exec.LookPath(oscapDockerBinary)
	if err != nil {
		s.logger.Debug("oscap-docker binary not found")
		s.available = false
		return
	}

	s.logger.WithField("path", path).Debug("oscap-docker binary found")

	// Check if docker is also available (required for oscap-docker)
	_, err = exec.LookPath("docker")
	if err != nil {
		s.logger.Debug("Docker binary not found - oscap-docker requires Docker")
		s.available = false
		return
	}

	// Check if Docker daemon is running
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		s.logger.Debug("Docker daemon not responding - oscap-docker requires Docker")
		s.available = false
		return
	}

	s.available = true
	s.logger.Debug("oscap-docker is available for container image scanning")
}

// ScanImage scans a Docker image for CVEs using oscap-docker
func (s *OscapDockerScanner) ScanImage(ctx context.Context, imageName string) (*models.ComplianceScan, error) {
	if !s.available {
		return nil, fmt.Errorf("oscap-docker is not available")
	}

	if imageName == "" {
		return nil, fmt.Errorf("image name is required")
	}

	startTime := time.Now()

	s.logger.WithField("image", imageName).Info("Scanning Docker image for CVEs...")

	// Run oscap-docker image-cve
	// This will:
	// 1. Attach to the Docker image
	// 2. Determine OS variant/version
	// 3. Download applicable CVE stream (OVAL data)
	// 4. Run vulnerability scan
	cmd := exec.CommandContext(ctx, oscapDockerBinary, "image-cve", imageName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("scan cancelled: %w", ctx.Err())
		}
		// oscap-docker exits non-zero when vulnerabilities are found
		// Check if we got any output to parse
		if len(output) == 0 {
			return nil, fmt.Errorf("oscap-docker failed: %w", err)
		}
		s.logger.WithError(err).Debug("oscap-docker exited with error, parsing output for results")
	}

	// Parse the output
	scan := s.parseImageCveOutput(string(output), imageName)
	scan.StartedAt = startTime
	now := time.Now()
	scan.CompletedAt = &now
	scan.Status = "completed"

	s.logger.WithFields(logrus.Fields{
		"image":           imageName,
		"vulnerabilities": scan.Failed,
		"total_cves":      scan.TotalRules,
	}).Info("Docker image CVE scan completed")

	return scan, nil
}

// ScanContainer scans a running container for CVEs
func (s *OscapDockerScanner) ScanContainer(ctx context.Context, containerName string) (*models.ComplianceScan, error) {
	if !s.available {
		return nil, fmt.Errorf("oscap-docker is not available")
	}

	if containerName == "" {
		return nil, fmt.Errorf("container name is required")
	}

	startTime := time.Now()

	s.logger.WithField("container", containerName).Info("Scanning Docker container for CVEs...")

	// Run oscap-docker container-cve
	cmd := exec.CommandContext(ctx, oscapDockerBinary, "container-cve", containerName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("scan cancelled: %w", ctx.Err())
		}
		if len(output) == 0 {
			return nil, fmt.Errorf("oscap-docker failed: %w", err)
		}
		s.logger.WithError(err).Debug("oscap-docker exited with error, parsing output for results")
	}

	// Parse the output
	scan := s.parseContainerCveOutput(string(output), containerName)
	scan.StartedAt = startTime
	now := time.Now()
	scan.CompletedAt = &now
	scan.Status = "completed"

	s.logger.WithFields(logrus.Fields{
		"container":       containerName,
		"vulnerabilities": scan.Failed,
		"total_cves":      scan.TotalRules,
	}).Info("Docker container CVE scan completed")

	return scan, nil
}

// ScanAllImages scans all Docker images on the system
func (s *OscapDockerScanner) ScanAllImages(ctx context.Context) ([]*models.ComplianceScan, error) {
	if !s.available {
		return nil, fmt.Errorf("oscap-docker is not available")
	}

	// Get list of all images
	cmd := exec.CommandContext(ctx, "docker", "images", "--format", "{{.Repository}}:{{.Tag}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list Docker images: %w", err)
	}

	var scans []*models.ComplianceScan
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		imageName := strings.TrimSpace(scanner.Text())
		if imageName == "" || imageName == "<none>:<none>" {
			continue
		}

		scan, err := s.ScanImage(ctx, imageName)
		if err != nil {
			s.logger.WithError(err).WithField("image", imageName).Warn("Failed to scan image, skipping")
			continue
		}
		scans = append(scans, scan)
	}

	return scans, nil
}

// parseImageCveOutput parses oscap-docker image-cve output
func (s *OscapDockerScanner) parseImageCveOutput(output string, imageName string) *models.ComplianceScan {
	scan := &models.ComplianceScan{
		ProfileName: fmt.Sprintf("Docker Image CVE Scan: %s", imageName),
		ProfileType: "oscap-docker",
		Results:     make([]models.ComplianceResult, 0),
	}

	// Parse CVE results
	// oscap-docker output format varies, but typically includes lines like:
	// CVE-2021-44228 - Critical - Description...
	// Or in OVAL format with true/false results

	// Pattern for CVE lines
	cvePattern := regexp.MustCompile(`(CVE-\d{4}-\d+)`)
	severityPattern := regexp.MustCompile(`(?i)(critical|high|important|medium|moderate|low)`)

	lines := strings.Split(output, "\n")
	seenCVEs := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for CVE identifiers
		cveMatches := cvePattern.FindStringSubmatch(line)
		if len(cveMatches) > 0 {
			cveID := cveMatches[1]

			// Skip duplicates
			if seenCVEs[cveID] {
				continue
			}
			seenCVEs[cveID] = true

			// Determine severity
			severity := "medium" // default
			severityMatch := severityPattern.FindStringSubmatch(line)
			if len(severityMatch) > 0 {
				severity = strings.ToLower(severityMatch[1])
				// Normalize severity names
				switch severity {
				case "important":
					severity = "high"
				case "moderate":
					severity = "medium"
				}
			}

			scan.Results = append(scan.Results, models.ComplianceResult{
				RuleID:   cveID,
				Title:    line,
				Status:   "fail", // CVEs found are failures
				Severity: severity,
				Section:  "Container Vulnerabilities",
			})
			scan.Failed++
			scan.TotalRules++
		}
	}

	// If no CVEs found, mark as passed
	if scan.TotalRules == 0 {
		scan.Passed = 1
		scan.TotalRules = 1
		scan.Score = 100.0
		scan.Results = append(scan.Results, models.ComplianceResult{
			RuleID:  "no-cves",
			Title:   "No known CVEs found in image",
			Status:  "pass",
			Section: "Container Vulnerabilities",
		})
	} else {
		// Calculate score based on severity
		// Critical = 10 points, High = 5 points, Medium = 2 points, Low = 1 point
		totalPenalty := 0
		for _, result := range scan.Results {
			switch result.Severity {
			case "critical":
				totalPenalty += 10
			case "high":
				totalPenalty += 5
			case "medium":
				totalPenalty += 2
			case "low":
				totalPenalty += 1
			}
		}
		// Score decreases with more/worse vulnerabilities
		// Max penalty of 100 points
		if totalPenalty > 100 {
			totalPenalty = 100
		}
		scan.Score = float64(100 - totalPenalty)
		if scan.Score < 0 {
			scan.Score = 0
		}
	}

	return scan
}

// parseContainerCveOutput parses oscap-docker container-cve output
func (s *OscapDockerScanner) parseContainerCveOutput(output string, containerName string) *models.ComplianceScan {
	// Reuse image parsing logic - output format is similar
	scan := s.parseImageCveOutput(output, containerName)
	scan.ProfileName = fmt.Sprintf("Docker Container CVE Scan: %s", containerName)
	return scan
}

// GetVersion returns the oscap-docker version
func (s *OscapDockerScanner) GetVersion() string {
	if !s.available {
		return ""
	}

	cmd := exec.Command(oscapDockerBinary, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// EnsureInstalled checks if oscap-docker is installed and attempts to install if not
func (s *OscapDockerScanner) EnsureInstalled() error {
	// Re-check availability
	s.checkAvailability()

	if s.available {
		s.logger.Debug("oscap-docker is already available")
		return nil
	}

	s.logger.Info("Attempting to install oscap-docker...")

	// Detect package manager and install
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Try different package managers with appropriate packages
	if _, err := exec.LookPath("apt-get"); err == nil {
		// Debian/Ubuntu - oscap-docker requires the 'atomic' package which is NOT available on Ubuntu
		// oscap-docker is primarily a Red Hat/Fedora tool that depends on atomic
		// See: https://answers.launchpad.net/ubuntu/+source/openscap/+question/242354
		s.logger.Warn("oscap-docker is not supported on Ubuntu/Debian - it requires the 'atomic' package which is only available on RHEL/Fedora")
		return fmt.Errorf("oscap-docker is not available on Ubuntu/Debian (requires 'atomic' package)")
	} else if _, err := exec.LookPath("dnf"); err == nil {
		// RHEL 8+/Fedora - oscap-docker is available via openscap-containers
		s.logger.Info("Installing openscap-containers for RHEL/Fedora...")
		installCmd := exec.CommandContext(ctx, "dnf", "install", "-y", "openscap-containers")
		output, err := installCmd.CombinedOutput()
		if err != nil {
			s.logger.WithError(err).WithField("output", string(output)).Warn("Failed to install openscap-containers")
			return fmt.Errorf("failed to install openscap-containers: %w", err)
		}
	} else if _, err := exec.LookPath("yum"); err == nil {
		// RHEL 7/CentOS 7
		s.logger.Info("Installing openscap-containers for CentOS/RHEL 7...")
		installCmd := exec.CommandContext(ctx, "yum", "install", "-y", "openscap-containers")
		output, err := installCmd.CombinedOutput()
		if err != nil {
			s.logger.WithError(err).WithField("output", string(output)).Warn("Failed to install openscap-containers")
			return fmt.Errorf("failed to install openscap-containers: %w", err)
		}
	} else if _, err := exec.LookPath("apk"); err == nil {
		// Alpine - oscap-docker is not available
		s.logger.Warn("oscap-docker is not available on Alpine Linux")
		return fmt.Errorf("oscap-docker is not available on Alpine Linux")
	} else {
		return fmt.Errorf("no supported package manager found")
	}

	// Re-check availability after install
	s.checkAvailability()
	if !s.available {
		s.logger.Warn("oscap-docker binary not found after installation - it may not be available for this OS version")
		return fmt.Errorf("oscap-docker still not available after installation attempt")
	}

	s.logger.Info("oscap-docker installed successfully")
	return nil
}
