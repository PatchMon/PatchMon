package system

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/sirupsen/logrus"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"
)

// OSReleaseInfo holds parsed information from /etc/os-release
type OSReleaseInfo struct {
	Name            string
	PrettyName      string
	Version         string
	VersionID       string
	ID              string
	IDLike          string
	VersionCodename string
}

// Detector handles system information detection
type Detector struct {
	logger *logrus.Logger
}

// New creates a new system detector
func New(logger *logrus.Logger) *Detector {
	return &Detector{
		logger: logger,
	}
}

// parseOSRelease parses /etc/os-release file and returns OS information
func (d *Detector) parseOSRelease() (*OSReleaseInfo, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/os-release: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Log error but don't fail the function
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	info := &OSReleaseInfo{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := strings.Trim(parts[1], "\"'")

		switch key {
		case "NAME":
			info.Name = value
		case "PRETTY_NAME":
			info.PrettyName = value
		case "VERSION":
			info.Version = value
		case "VERSION_ID":
			info.VersionID = value
		case "ID":
			info.ID = value
		case "ID_LIKE":
			info.IDLike = value
		case "VERSION_CODENAME":
			info.VersionCodename = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan /etc/os-release: %w", err)
	}

	return info, nil
}

// DetectOS detects the operating system and version using /etc/os-release
func (d *Detector) DetectOS() (osType, osVersion string, err error) {
	// Try to parse /etc/os-release first
	osReleaseInfo, err := d.parseOSRelease()
	if err != nil {
		d.logger.WithError(err).Warn("Failed to parse /etc/os-release, falling back to gopsutil")

		// Fallback to gopsutil
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		info, err := host.InfoWithContext(ctx)
		if err != nil {
			d.logger.WithError(err).Warn("Failed to get host info")
			return "", "", err
		}

		osType = info.Platform
		osVersion = info.PlatformVersion

		return osType, osVersion, nil
	}

	// Use NAME for OS type (e.g., "Pop!_OS", "Debian GNU/Linux", "Rocky Linux")
	osType = osReleaseInfo.Name
	if osType == "" {
		osType = "Unknown"
	}

	// Use VERSION for OS version (e.g., "22.04 LTS", "12 (bookworm)", "10.0 (Red Quartz)")
	osVersion = osReleaseInfo.Version
	if osVersion == "" {
		osVersion = "Unknown"
	}

	d.logger.WithFields(logrus.Fields{
		"name":          osReleaseInfo.Name,
		"version":       osReleaseInfo.Version,
		"version_id":    osReleaseInfo.VersionID,
		"id":            osReleaseInfo.ID,
		"id_like":       osReleaseInfo.IDLike,
		"final_type":    osType,
		"final_version": osVersion,
	}).Debug("Parsed OS release information")

	return osType, osVersion, nil
}

// GetSystemInfo gets additional system information
func (d *Detector) GetSystemInfo() models.SystemInfo {
	d.logger.Debug("Beginning system information collection")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := models.SystemInfo{
		KernelVersion: d.GetKernelVersion(),
		SELinuxStatus: d.getSELinuxStatus(),
		SystemUptime:  d.getSystemUptime(ctx),
		LoadAverage:   d.getLoadAverage(ctx),
	}

	d.logger.WithFields(logrus.Fields{
		"kernel":  info.KernelVersion,
		"selinux": info.SELinuxStatus,
		"uptime":  info.SystemUptime,
	}).Debug("Collected kernel, SELinux, and uptime information")

	return info
}

// GetArchitecture returns the system architecture
func (d *Detector) GetArchitecture() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get architecture")
		return constants.ArchUnknown
	}

	return info.KernelArch
}

// GetHostname returns the system hostname
func (d *Detector) GetHostname() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get hostname")
		// Fallback to os.Hostname
		return os.Hostname()
	}

	return info.Hostname, nil
}

// GetIPAddress gets the primary IP address using network interfaces
func (d *Detector) GetIPAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get network interfaces")
		return ""
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
					return ipnet.IP.String()
				}
			}
		}
	}

	return ""
}

// GetKernelVersion gets the kernel version
func (d *Detector) GetKernelVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get kernel version")
		return constants.ErrUnknownValue
	}

	return info.KernelVersion
}

// getSELinuxStatus gets SELinux status using file reading
func (d *Detector) getSELinuxStatus() string {
	// Try getenforce command first
	if cmd := exec.Command("getenforce"); cmd != nil {
		if output, err := cmd.Output(); err == nil {
			status := strings.ToLower(strings.TrimSpace(string(output)))
			// Map "enforcing" to "enabled" for server validation
			if status == constants.SELinuxEnforcing {
				return constants.SELinuxEnabled
			}
			if status == constants.SELinuxPermissive {
				return constants.SELinuxPermissive
			}
			return status
		}
	}

	// Fallback to reading config file
	if data, err := os.ReadFile("/etc/selinux/config"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if value, found := strings.CutPrefix(line, "SELINUX="); found {
				status := strings.ToLower(strings.Trim(value, "\"'"))
				// Map "enforcing" to "enabled" for server validation
				if status == constants.SELinuxEnforcing {
					return constants.SELinuxEnabled
				}
				if status == constants.SELinuxPermissive {
					return constants.SELinuxPermissive
				}
				return status
			}
		}
	}

	return constants.SELinuxDisabled
}

// getSystemUptime gets system uptime
func (d *Detector) getSystemUptime(ctx context.Context) string {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get uptime")
		return "Unknown"
	}

	uptime := time.Duration(info.Uptime) * time.Second

	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d days, %d hours, %d minutes", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%d hours, %d minutes", hours, minutes)
	} else {
		return fmt.Sprintf("%d minutes", minutes)
	}
}

// getLoadAverage gets system load average
func (d *Detector) getLoadAverage(ctx context.Context) []float64 {
	loadAvg, err := load.AvgWithContext(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get load average")
		return []float64{0, 0, 0}
	}

	return []float64{loadAvg.Load1, loadAvg.Load5, loadAvg.Load15}
}

// GetMachineID returns the system's machine ID using gopsutil
func (d *Detector) GetMachineID() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use gopsutil's HostID which reads from standard locations
	// (/etc/machine-id, /var/lib/dbus/machine-id, etc.)
	hostID, err := host.HostIDWithContext(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get host ID, using hostname as fallback")
		// Fallback to hostname if we can't get machine ID
		if hostname, err := os.Hostname(); err == nil {
			return hostname
		}
		return "unknown"
	}

	return hostID
}
