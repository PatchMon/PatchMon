//go:build linux

package system

import (
	"bytes"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicksPerSecond mirrors the kernel's USER_HZ, the unit /proc/<pid>/stat
// reports times in. It is 100 on every architecture the agent builds for.
const clockTicksPerSecond = 100

// virtualisedUptimePath is the mount point lxcfs (and equivalents) bind a
// container-scoped uptime file over.
const virtualisedUptimePath = "/proc/uptime"

// maxPlausibleUptime bounds what /proc/uptime is allowed to claim. Anything
// beyond it means the file is not reporting seconds since boot at all, which a
// broken lxcfs can produce.
const maxPlausibleUptime = 100 * 365 * 24 * time.Hour

// procSources locates the files the container-aware uptime probe reads. Held in
// a struct so tests can point it at a fixture tree.
type procSources struct {
	uptime           string
	mountinfo        string
	pid1Stat         string
	pid1Environ      string
	pid1Cgroup       string
	systemdContainer string
	dockerEnv        string
}

var defaultProcSources = procSources{
	uptime:           "/proc/uptime",
	mountinfo:        "/proc/self/mountinfo",
	pid1Stat:         "/proc/1/stat",
	pid1Environ:      "/proc/1/environ",
	pid1Cgroup:       "/proc/1/cgroup",
	systemdContainer: "/run/systemd/container",
	dockerEnv:        "/.dockerenv",
}

// cgroupContainerMarkers are the runtime names that appear in PID 1's cgroup
// paths inside a container on cgroup v1 hosts. On the host itself PID 1 sits in
// the root or init scope, so none of these match.
var cgroupContainerMarkers = []string{"/docker/", "/docker-", "/lxc/", "/lxc-", "/kubepods"}

// containerUptime returns how long the container the agent runs in has been up.
//
// gopsutil takes Linux uptime from the sysinfo syscall, which is not namespaced
// and which lxcfs cannot intercept, so inside an LXC container it reports the
// Proxmox host's uptime instead of the container's. ok is false when the agent
// is not containerised, or when the container's own uptime cannot be
// established, and callers should keep the figure gopsutil gives them.
func containerUptime() (time.Duration, bool) {
	return defaultProcSources.containerUptime()
}

func (p procSources) containerUptime() (time.Duration, bool) {
	procUptime, err := p.readProcUptime()
	if err != nil {
		return 0, false
	}

	uptimeVirtualised, lxcfsProcMount := p.mountinfoSignals()

	// lxcfs bind-mounts a virtualised /proc/uptime that already reports the
	// container's own uptime, so take it as-is.
	if uptimeVirtualised {
		return procUptime, true
	}

	if !p.inContainer(lxcfsProcMount) {
		return 0, false
	}

	// Without lxcfs, /proc/uptime is the host's. PID 1 in our namespace is the
	// container's init, and its start time is measured from the host's boot, so
	// the difference is the container's uptime.
	start, err := p.readPID1StartTime()
	if err != nil {
		return 0, false
	}

	uptime := procUptime - start
	if uptime <= 0 {
		return 0, false
	}

	return uptime, true
}

// readProcUptime reads the first field of /proc/uptime, the seconds elapsed
// since boot.
func (p procSources) readProcUptime() (time.Duration, error) {
	data, err := os.ReadFile(p.uptime)
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errors.New("empty proc uptime")
	}

	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	// ParseFloat accepts NaN and Inf, neither of which survives the conversion
	// to a Duration in any useful form.
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, errors.New("non-finite proc uptime")
	}

	uptime := time.Duration(seconds * float64(time.Second))
	if uptime <= 0 || uptime > maxPlausibleUptime {
		return 0, errors.New("implausible proc uptime")
	}

	return uptime, nil
}

// mountinfoSignals scans the mount table once for the two things the probe
// cares about: whether anything is mounted over /proc/uptime, and whether lxcfs
// is providing a virtualised procfs or sysfs file. Only the latter means we are
// inside an LXC container: Proxmox runs lxcfs on the host too, but mounts it at
// /var/lib/lxcfs, outside /proc and /sys.
func (p procSources) mountinfoSignals() (uptimeVirtualised, lxcfsProcMount bool) {
	data, err := os.ReadFile(p.mountinfo)
	if err != nil {
		return false, false
	}

	for line := range strings.Lines(string(data)) {
		// Mount point is the fifth field of a mountinfo record.
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mountPoint := fields[4]

		if mountPoint == virtualisedUptimePath {
			uptimeVirtualised = true
		}

		if strings.Contains(line, "lxcfs") &&
			(strings.HasPrefix(mountPoint, "/proc/") || strings.HasPrefix(mountPoint, "/sys/")) {
			lxcfsProcMount = true
		}
	}

	return uptimeVirtualised, lxcfsProcMount
}

// inContainer reports whether the agent is running inside a container.
// lxcfsProcMount comes from mountinfoSignals so the mount table is read once.
func (p procSources) inContainer(lxcfsProcMount bool) bool {
	if lxcfsProcMount {
		return true
	}

	// systemd records the container type it detected at boot.
	if data, err := os.ReadFile(p.systemdContainer); err == nil && len(bytes.TrimSpace(data)) > 0 {
		return true
	}

	// LXC, systemd-nspawn and podman all export container= to init.
	if data, err := os.ReadFile(p.pid1Environ); err == nil {
		for _, entry := range bytes.Split(data, []byte{0}) {
			if bytes.HasPrefix(entry, []byte("container=")) {
				return true
			}
		}
	}

	if _, err := os.Stat(p.dockerEnv); err == nil {
		return true
	}

	if data, err := os.ReadFile(p.pid1Cgroup); err == nil {
		for _, marker := range cgroupContainerMarkers {
			if bytes.Contains(data, []byte(marker)) {
				return true
			}
		}
	}

	return false
}

// readPID1StartTime returns how long after the host booted PID 1 started.
func (p procSources) readPID1StartTime() (time.Duration, error) {
	data, err := os.ReadFile(p.pid1Stat)
	if err != nil {
		return 0, err
	}

	// The second field is the parenthesised executable name and may itself
	// contain spaces and brackets, so anchor the scan on the final ')'.
	end := bytes.LastIndexByte(data, ')')
	if end < 0 || end+2 >= len(data) {
		return 0, errors.New("malformed proc stat")
	}

	// starttime is field 22; the two fields before the ')' are already consumed.
	const startTimeIndex = 19

	fields := strings.Fields(string(data[end+2:]))
	if len(fields) <= startTimeIndex {
		return 0, errors.New("proc stat too short")
	}

	ticks, err := strconv.ParseInt(fields[startTimeIndex], 10, 64)
	if err != nil {
		return 0, err
	}
	if ticks < 0 {
		return 0, errors.New("negative start time in proc stat")
	}

	return time.Duration(ticks) * (time.Second / clockTicksPerSecond), nil
}
