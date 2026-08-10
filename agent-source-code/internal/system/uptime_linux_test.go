//go:build linux

package system

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pid1Stat builds a /proc/1/stat line with the given starttime in clock ticks.
// The comm field deliberately contains a space and brackets, matching processes
// such as "(sd-pam)" that break naive field splitting.
func pid1Stat(t *testing.T, startTicks int64) string {
	t.Helper()

	fields := []string{"1", "(my proc)", "S"}
	for i := 4; i <= 52; i++ {
		if i == 22 {
			fields = append(fields, strconv.FormatInt(startTicks, 10))
			continue
		}
		fields = append(fields, "0")
	}

	return strings.Join(fields, " ") + "\n"
}

// writeProcTree lays out a fake procfs and returns sources pointing at it. Only
// the files named in the map exist; the rest are absent, as they would be on a
// host that does not run the corresponding runtime.
func writeProcTree(t *testing.T, files map[string]string) procSources {
	t.Helper()

	dir := t.TempDir()
	sources := procSources{
		uptime:           filepath.Join(dir, "uptime"),
		mountinfo:        filepath.Join(dir, "mountinfo"),
		pid1Stat:         filepath.Join(dir, "pid1stat"),
		pid1Environ:      filepath.Join(dir, "pid1environ"),
		pid1Cgroup:       filepath.Join(dir, "pid1cgroup"),
		systemdContainer: filepath.Join(dir, "systemd-container"),
		dockerEnv:        filepath.Join(dir, "dockerenv"),
	}

	byName := map[string]string{
		"uptime":            sources.uptime,
		"mountinfo":         sources.mountinfo,
		"pid1stat":          sources.pid1Stat,
		"pid1environ":       sources.pid1Environ,
		"pid1cgroup":        sources.pid1Cgroup,
		"systemd-container": sources.systemdContainer,
		"dockerenv":         sources.dockerEnv,
	}

	for name, content := range files {
		path, ok := byName[name]
		require.Truef(t, ok, "unknown fixture file %q", name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	return sources
}

func TestContainerUptimeBareHost(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			// A plain host: no container markers anywhere.
			name: "no markers",
			files: map[string]string{
				"mountinfo":  "23 28 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:14 - proc proc rw\n",
				"pid1cgroup": "0::/init.scope\n",
			},
		},
		{
			// Proxmox runs lxcfs on the hypervisor host as well, mounted at
			// /var/lib/lxcfs. Only lxcfs over /proc or /sys means we are inside
			// a container.
			name: "proxmox host running lxcfs",
			files: map[string]string{
				"mountinfo": "23 28 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:14 - proc proc rw\n" +
					"48 25 0:44 / /var/lib/lxcfs rw,nosuid,nodev,relatime shared:29 - fuse.lxcfs lxcfs rw,user_id=0,group_id=0\n",
				"pid1cgroup": "0::/init.scope\n",
			},
		},
		{
			// The cgroup markers are anchored so a host's own docker service
			// unit does not read as a container.
			name: "docker host",
			files: map[string]string{
				"pid1cgroup": "0::/init.scope\n",
				"mountinfo":  "23 28 0:21 / /proc rw,relatime shared:14 - proc proc rw\n",
			},
		},
		{
			// A mount whose source path is /proc/uptime, rather than its mount
			// point, does not virtualise anything.
			name: "near miss mount points",
			files: map[string]string{
				"mountinfo": "31 23 0:33 /proc/uptime /mnt/uptime rw,relatime shared:16 - ext4 /dev/sda1 rw\n" +
					"32 23 0:34 / /proc/uptime2 rw,relatime shared:17 - tmpfs tmpfs rw\n",
				"pid1cgroup": "0::/init.scope\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"uptime":   "864000.00 1000.00\n",
				"pid1stat": pid1Stat(t, 250),
			}
			for name, content := range tt.files {
				files[name] = content
			}

			sources := writeProcTree(t, files)

			_, lxcfsProcMount := sources.mountinfoSignals()
			assert.False(t, sources.inContainer(lxcfsProcMount))

			// Not containerised, so the caller keeps gopsutil's figure.
			_, ok := sources.containerUptime()
			assert.False(t, ok)
		})
	}
}

func TestContainerUptimeLXCWithoutLxcfs(t *testing.T) {
	// Host up 10 days, container init started at day 7: the container has been
	// up 3 days, not 10.
	sources := writeProcTree(t, map[string]string{
		"uptime":            "864000.00 1000.00\n",
		"mountinfo":         "23 28 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:14 - proc proc rw\n",
		"pid1stat":          pid1Stat(t, 604800*clockTicksPerSecond),
		"systemd-container": "lxc\n",
	})

	uptime, ok := sources.containerUptime()
	require.True(t, ok)
	assert.Equal(t, 72*time.Hour, uptime)
}

func TestContainerUptimeLXCWithLxcfs(t *testing.T) {
	// lxcfs virtualises /proc/uptime, so it already reports the container's own
	// uptime and PID 1's host-relative start time must not be subtracted.
	sources := writeProcTree(t, map[string]string{
		"uptime": "259200.00 1000.00\n",
		"mountinfo": "23 28 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:14 - proc proc rw\n" +
			"31 23 0:33 /uptime /proc/uptime rw,nosuid,nodev,relatime shared:16 - fuse.lxcfs lxcfs rw,user_id=0,group_id=0\n",
		"pid1stat":          pid1Stat(t, 604800*clockTicksPerSecond),
		"systemd-container": "lxc\n",
	})

	uptime, ok := sources.containerUptime()
	require.True(t, ok)
	assert.Equal(t, 72*time.Hour, uptime)
}

func TestContainerUptimeDetectionSignals(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "pid 1 environ",
			files: map[string]string{
				"pid1environ": "HOME=/\x00container=lxc\x00TERM=linux\x00",
			},
		},
		{
			name: "docker env file",
			files: map[string]string{
				"dockerenv": "",
			},
		},
		{
			name: "cgroup v1 path",
			files: map[string]string{
				"pid1cgroup": "11:devices:/docker/6a1b2c3d4e5f\n1:name=systemd:/docker/6a1b2c3d4e5f\n",
			},
		},
		{
			name: "lxcfs virtualising another proc file",
			files: map[string]string{
				"mountinfo": "31 23 0:33 /meminfo /proc/meminfo rw,relatime shared:16 - fuse.lxcfs lxcfs rw\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"uptime":   "864000.00 1000.00\n",
				"pid1stat": pid1Stat(t, 604800*clockTicksPerSecond),
			}
			for name, content := range tt.files {
				files[name] = content
			}

			sources := writeProcTree(t, files)

			_, lxcfsProcMount := sources.mountinfoSignals()
			assert.True(t, sources.inContainer(lxcfsProcMount))

			uptime, ok := sources.containerUptime()
			require.True(t, ok)
			assert.Equal(t, 72*time.Hour, uptime)
		})
	}
}

func TestContainerUptimeRejectsImplausibleValues(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "pid 1 started after the host booted",
			files: map[string]string{
				"uptime":            "3600.00 1000.00\n",
				"pid1stat":          pid1Stat(t, 7200*clockTicksPerSecond),
				"systemd-container": "lxc\n",
			},
		},
		{
			name: "pid 1 stat unreadable",
			files: map[string]string{
				"uptime":            "3600.00 1000.00\n",
				"systemd-container": "lxc\n",
			},
		},
		{
			name: "proc uptime unreadable",
			files: map[string]string{
				"systemd-container": "lxc\n",
			},
		},
		{
			name: "proc uptime empty",
			files: map[string]string{
				"uptime":            "\n",
				"pid1stat":          pid1Stat(t, 100),
				"systemd-container": "lxc\n",
			},
		},
		{
			name: "proc stat truncated",
			files: map[string]string{
				"uptime":            "3600.00 1000.00\n",
				"pid1stat":          "1 (systemd) S 0 1 1\n",
				"systemd-container": "lxc\n",
			},
		},
		{
			// A broken lxcfs can emit values that parse but mean nothing. They
			// must not become a boot time centuries in the past.
			name: "proc uptime not a number",
			files: map[string]string{
				"uptime":    "NaN 1000.00\n",
				"mountinfo": "31 23 0:33 /uptime /proc/uptime rw,relatime shared:16 - fuse.lxcfs lxcfs rw\n",
			},
		},
		{
			name: "proc uptime infinite",
			files: map[string]string{
				"uptime":    "inf 1000.00\n",
				"mountinfo": "31 23 0:33 /uptime /proc/uptime rw,relatime shared:16 - fuse.lxcfs lxcfs rw\n",
			},
		},
		{
			name: "proc uptime beyond any plausible value",
			files: map[string]string{
				"uptime":    "1e30 1000.00\n",
				"mountinfo": "31 23 0:33 /uptime /proc/uptime rw,relatime shared:16 - fuse.lxcfs lxcfs rw\n",
			},
		},
		{
			name: "proc uptime negative",
			files: map[string]string{
				"uptime":    "-3600.00 1000.00\n",
				"mountinfo": "31 23 0:33 /uptime /proc/uptime rw,relatime shared:16 - fuse.lxcfs lxcfs rw\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := writeProcTree(t, tt.files)

			_, ok := sources.containerUptime()
			assert.False(t, ok)
		})
	}
}

func TestReadPID1StartTime(t *testing.T) {
	sources := writeProcTree(t, map[string]string{
		"pid1stat": pid1Stat(t, 12345),
	})

	start, err := sources.readPID1StartTime()
	require.NoError(t, err)
	assert.Equal(t, 123450*time.Millisecond, start)
}

// A comm containing its own brackets is why the parser anchors on the final
// ')' rather than splitting the line on whitespace.
func TestReadPID1StartTimeCommWithBrackets(t *testing.T) {
	stat := strings.Replace(pid1Stat(t, 12345), "(my proc)", "(we(ir)d proc)", 1)
	sources := writeProcTree(t, map[string]string{"pid1stat": stat})

	start, err := sources.readPID1StartTime()
	require.NoError(t, err)
	assert.Equal(t, 123450*time.Millisecond, start)
}

func TestReadProcUptime(t *testing.T) {
	sources := writeProcTree(t, map[string]string{
		"uptime": "1234.56 987.65\n",
	})

	uptime, err := sources.readProcUptime()
	require.NoError(t, err)
	assert.InDelta(t, 1234.56, uptime.Seconds(), 0.001)
}

// containerUptime must never panic or misreport on the machine running the
// tests, whatever kind of machine that is.
func TestContainerUptimeOnRealHost(t *testing.T) {
	uptime, ok := containerUptime()
	if !ok {
		return
	}
	assert.Positive(t, uptime)
}
