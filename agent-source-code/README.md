# PatchMon Agent

PatchMon's monitoring agent collects and reports package, system, hardware, and network information to the PatchMon server. It runs as a long-lived service with a persistent WebSocket connection for real-time communication.

## Supported Platforms

**Linux** (amd64, 386, arm64, arm):
- Debian / Ubuntu (apt)
- Fedora / RHEL / CentOS / AlmaLinux / Rocky Linux (dnf)
- Arch Linux / Manjaro (pacman)
- Alpine Linux (apk)

**FreeBSD** (amd64, 386, arm64, arm):
- FreeBSD

**Windows** (amd64, 386):
- Windows 10 / 11 / Server 2016+ (WinGet, Windows Update API, registry, optional Chocolatey inventory)

## Chocolatey inventory on Windows

When Chocolatey CLI is installed, the agent collects its installed packages and
checks `choco outdated --limit-output` using the host's configured sources. CLI
1.x uses `list --local-only`; CLI 2.x and later use the local `list` command.
Inventory collection does not install Chocolatey or change its sources, pins or packages.

Rows use `chocolatey:<package-id>` and source `chocolatey`. Chocolatey package
versions can differ from application versions, and meta packages need not have
an Uninstall registry entry. These records remain separate from registry/WinGet
application records; the same software may therefore have both records and
contribute to both counts. This avoids guessing package ownership or replacing
WinGet's update version with one from a different feed.

With a Chocolatey-capable agent and server, these records can be selected for a
validation (dry run) or upgrade. The Windows "Applications" scope also includes
Chocolatey packages. Pinned packages remain visible but are not patchable; pins
are checked again on the host before installation. Chocolatey does not identify
security updates here, so these rows are not classified as security updates.

Each requested package is rechecked, upgraded by ID to the version discovered
in the configured feed, and verified afterward. Missing packages are never
installed implicitly. No force or ignore-pin options are used. Stop requests
prevent subsequent upgrades without interrupting the active installer; command
and overall run time limits still apply. Restart-required exit codes are reported
and stop subsequent Chocolatey upgrades. PatchMon does not issue a reboot.
Package scripts run under the service account, as with other package managers.
A failed package script may leave a partial installation; Chocolatey does not
guarantee rollback, and PatchMon records the failure rather than claiming success.

The existing patching endpoints still accept package names, for example
`chocolatey:git`, with the existing `dry_run` flag. Agent reports add
`chocolateyId` and `chocolateyPinned`; patch-run items use kind `chocolatey`.
Clients with a closed enumeration of run-item kinds must recognize the new kind.
The server resolves targets from the host's inventory, never from arbitrary
client-supplied Chocolatey command arguments. Agent/server capability negotiation
keeps older servers compatible with inventory reports; reporting-only agents do
not advertise patchable Chocolatey IDs. Both components must be updated to patch.

The database migration adds Chocolatey metadata and allows Chocolatey run items.
Downgrading that migration refuses to discard Chocolatey run history: it requires
those run items to have been handled before restoring the old kind constraint.

Collection runs as the agent service account (normally SYSTEM), which needs
access to the configured feeds. The agent locates `choco.exe` through
`ChocolateyInstall`, the standard `ProgramData` installation, or PATH. Restart
the agent service after changing environment variables for a custom installation.
A command failure, timeout, malformed output or inconsistent scan aborts the
report, retaining the server's last successful inventory until a scan succeeds.
If Chocolatey is absent, normal Windows collection continues unchanged.

### Testing Chocolatey collection

Run `go test ./internal/packages -run Chocolatey` for the parser/command tests.
On Windows, set `PATCHMON_CHOCO_INTEGRATION=1` to additionally run the live
collector against an existing Chocolatey installation and its configured feeds.
The test does not install software or modify feed configuration.

For a repeatable local-feed test, install `patchmon-inventory-fixture` and
`patchmon-pinned-fixture` at version `1.0.0`, make `2.0.0` available in a configured
feed, and pin the latter package. Set `PATCHMON_CHOCO_EXPECT_FIXTURES=1` to assert
both updates. Optional `PATCHMON_CHOCO_TEST_OUTPUT` saves the collected JSON.
Run under SYSTEM as well as an administrator to check service-account feed access.

### Testing Chocolatey upgrades

`PATCHMON_CHOCO_PATCH_INTEGRATION=1` enables the opt-in Windows patching test.
Use a disposable host and a local feed containing `1.0.0` and `2.0.0` versions of
`patchmon-upgrade-fixture`, `patchmon-pinned-fixture`, `patchmon-failure-fixture`
and `patchmon-absent-fixture`. Install the first three at `1.0.0`, pin the second,
and make the failure fixture's `2.0.0` install script throw an error. Keep the
absent fixture uninstalled. All other packages should be harmless text-file
packages. The test verifies dry runs, a real upgrade, pins, failure reporting,
no implicit installation, and an already-current target. It modifies only these
fixtures. Optional `PATCHMON_CHOCO_PATCH_OUTPUT` saves run results as JSON.
Reboot exit codes and stop behavior are covered by deterministic backend tests;
this live fixture test does not reboot the host.

## Installation

### Binary Installation

1. **Download** the appropriate binary for your OS and architecture from the releases page, or via the install script served by your PatchMon server.
2. **Make executable** and move to system path (Linux/FreeBSD):

```bash
chmod +x patchmon-agent-linux-amd64
sudo mv patchmon-agent-linux-amd64 /usr/local/bin/patchmon-agent
```

On **Windows**, download the `.exe` binary and place it in a suitable location (e.g. `C:\Program Files\PatchMon\`).

### From Source

**Prerequisites:**
- Go 1.26 or later
- Root / Administrator access on the target system

**Build and Install:**

```bash
make deps          # Download Go dependencies
make build         # Build for current platform (output: build/patchmon-agent)
sudo make install  # Install to /usr/local/bin (Linux/FreeBSD)
```

## Configuration

### Initial Setup

1. **Configure Credentials:**

```bash
sudo patchmon-agent config set-api <API_ID> <API_KEY> <SERVER_URL>
```

Example:

```bash
sudo patchmon-agent config set-api patchmon_1a2b3c4d abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890 https://patchmon.example.com
```

This saves the server URL to the config file and the API credentials to the credentials file, then automatically runs a connectivity test to verify everything is working.

2. **Test Connectivity** (optional — already tested during setup):

```bash
sudo patchmon-agent ping
```

3. **Send Initial Report:**

```bash
sudo patchmon-agent report
```

4. **Start the Service:**

```bash
sudo patchmon-agent serve
```

The `serve` command is the primary runtime mode. It maintains a WebSocket connection to the server, sends periodic reports on a configurable interval, handles real-time commands from the server, and manages auto-updates.

### Configuration Files

**Linux / FreeBSD:**

| File | Path |
|---|---|
| Main config | `/etc/patchmon/config.yml` |
| Credentials | `/etc/patchmon/credentials.yml` (0600 permissions) |
| Logs | `/etc/patchmon/logs/patchmon-agent.log` |

**Windows:**

| File | Path |
|---|---|
| Main config | `C:\ProgramData\PatchMon\config.yml` |
| Credentials | `C:\ProgramData\PatchMon\credentials.yml` |
| Logs | `C:\ProgramData\PatchMon\patchmon-agent.log` |

### Example Configuration File

`/etc/patchmon/config.yml`:

```yaml
patchmon_server: "https://patchmon.example.com"
api_version: "v1"
credentials_file: "/etc/patchmon/credentials.yml"
log_file: "/etc/patchmon/logs/patchmon-agent.log"
log_level: "info"
update_interval: 60
report_offset: 0
skip_ssl_verify: false
integrations:
  docker: false
  compliance:
    enabled: "on-demand"
    scan_interval: 1440
    openscap_enabled: true
    docker_bench_enabled: false
  ssh-proxy-enabled: false
  rdp-proxy-enabled: false
```

| Field | Description |
|---|---|
| `patchmon_server` | PatchMon server URL |
| `api_version` | API version (default `v1`) |
| `credentials_file` | Path to credentials file |
| `log_file` | Path to log file |
| `log_level` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `update_interval` | Report interval in minutes (synced from server) |
| `report_offset` | Stagger offset in seconds (auto-calculated from API ID) |
| `skip_ssl_verify` | Skip TLS verification (for self-signed or internal CA certs) |
| `integrations` | Toggle integrations on/off (synced from server) |
| `compliance.scan_interval` | Compliance scan interval in minutes (default 1440 = 24h, min 60, max 10080). Runs independently from the report timer. |

### Example Credentials File

The credentials file is automatically created by `config set-api`:

```yaml
api_id: "patchmon_1a2b3c4d5e6f7890"
api_key: "your_api_key_here"
```

## Usage

### Available Commands

```
patchmon-agent [command] [flags]
```

| Command | Description | Root Required |
|---|---|---|
| `serve` | Run the agent as a long-lived service | Yes |
| `report` | Collect and send system/package data to server | Yes |
| `report --json` | Output the report payload as JSON to stdout | Yes |
| `ping` | Test server connectivity and validate API credentials | Yes |
| `config set-api <ID> <KEY> <URL>` | Configure API credentials and server URL | Yes |
| `config show` | Display current configuration and credentials status | No |
| `check-version` | Check if an agent update is available | Yes |
| `update-agent` | Download and install the latest agent version | Yes |
| `diagnostics` | Show detailed system and agent diagnostics | No |

### Global Flags

| Flag | Description |
|---|---|
| `--config <path>` | Config file path (default: `/etc/patchmon/config.yml` on Linux/FreeBSD, `C:\ProgramData\PatchMon\config.yml` on Windows) |
| `--log-level <level>` | Override log level (`debug`, `info`, `warn`, `error`) |

## Service Mode (`serve`)

The `serve` command is how the agent is intended to run in production. It:

- Maintains a persistent **WebSocket connection** to the PatchMon server
- Sends periodic **package and system reports** on a configurable interval
- **Staggers report times** using a deterministic offset derived from the API ID to avoid thundering herd
- Receives and acts on **real-time server commands** (report now, update agent, toggle integrations, run compliance scans, etc.)
- **Syncs configuration** (report interval, integration status) from the server on startup
- Streams **Docker container events** in real-time when Docker integration is enabled
- Handles **auto-updates** with SHA256 binary integrity verification
- Supports **SSH proxy** and **RDP proxy** sessions when enabled in config

### Service Management

The agent supports the following init systems for service restarts during updates:

- **systemd** (most Linux distributions)
- **OpenRC** (Alpine Linux)
- **FreeBSD rc.d** (FreeBSD)
- **Windows Service** (Windows Service Control Manager via `golang.org/x/sys/windows/svc`)

If no init system is detected, it falls back to a helper script for safe restarts.

## Integrations

Integrations are managed from the PatchMon web interface and synced to the agent via WebSocket. They can also be configured manually in `config.yml`.

### Docker

When enabled, the agent collects Docker containers, images, volumes, networks, and available image updates. It also streams real-time container status events over WebSocket.

```yaml
integrations:
  docker: true
```

### Compliance Scanning (OpenSCAP)

Compliance scanning supports three modes:

| Mode | Config Value | Behaviour |
|---|---|---|
| Disabled | `false` | No compliance scanning |
| On-demand | `"on-demand"` | Scans only when triggered from the web UI (default) |
| Enabled | `true` | Automatic scans run with each scheduled report |

```yaml
integrations:
  compliance:
    enabled: "on-demand"
    openscap_enabled: true
    docker_bench_enabled: false
```

When enabled, the agent installs OpenSCAP and SCAP Security Guide content. Available scan tools:

- **OpenSCAP** — CIS benchmark scanning and remediation
- **Docker Bench** — CIS Docker Benchmark (requires Docker integration)
- **oscap-docker** — Docker image CVE scanning (requires Docker integration)

### SSH Proxy

Enables browser-based SSH sessions through the agent. Must be enabled manually in `config.yml` for security reasons — it cannot be pushed from the server.

```yaml
integrations:
  ssh-proxy-enabled: true
```

### RDP Proxy

Enables browser-based RDP sessions through the agent (relaying traffic via the `guacd` sidecar on the server). Must be enabled manually in `config.yml`.

```yaml
integrations:
  rdp-proxy-enabled: true
```

## Agent Updates

The agent supports automatic updates with security protections:

- **SHA256 hash verification** — downloaded binary integrity is verified against a server-provided hash before installation
- **Version validation** — the downloaded binary is executed in test mode before replacing the current binary
- **Atomic replacement** — the binary is replaced using `os.Rename` for atomicity
- **Backup retention** — the last 3 binary backups are kept
- **Loop prevention** — a timestamp marker prevents repeated update attempts within 5 minutes
- **TLS** — `skip_ssl_verify` allows self-signed or internal CA certificates

To manually check for updates:

```bash
sudo patchmon-agent check-version
```

To manually update:

```bash
sudo patchmon-agent update-agent
```

## Logging

Logs are written to the platform-appropriate path (see [Configuration Files](#configuration-files)) with rotation (max 10 MB per file, 5 backups, 14-day retention, compressed).

```
2026-02-18T10:30:00 level=info msg="Collecting package information..."
2026-02-18T10:30:01 level=info msg="Found packages" count=156
2026-02-18T10:30:02 level=info msg="Sending report to PatchMon server..."
2026-02-18T10:30:03 level=info msg="Report sent successfully"
```

Log levels: `debug`, `info`, `warn`, `error`

## Diagnostics

Run comprehensive diagnostics to check agent health:

```bash
patchmon-agent diagnostics
```

This displays:

- **System information** — OS, architecture, kernel, hostname, machine ID
- **Agent information** — version, config file paths, log level
- **Configuration status** — whether config and credentials files exist
- **Network connectivity** — TCP reachability test and API credential validation
- **Recent logs** — last 10 log entries

## Troubleshooting

### Common Issues

1. **Permission Denied:**

```bash
# Most commands require root (Linux/FreeBSD) or Administrator (Windows)
sudo patchmon-agent <command>
```

2. **Credentials Not Found:**

```bash
# Configure credentials first
sudo patchmon-agent config set-api <API_ID> <API_KEY> <SERVER_URL>
```

3. **Network Connectivity:**

```bash
# Test server reachability and credentials
sudo patchmon-agent ping

# Detailed diagnostics including network info
patchmon-agent diagnostics
```

4. **Package Manager Issues:**

```bash
# Update package lists manually
sudo apt update         # Debian/Ubuntu
sudo dnf check-update   # Fedora/RHEL
sudo apk update         # Alpine
sudo pacman -Sy         # Arch
```

## Uninstallation

Uninstall functionality is handled by the `patchmon_remove.sh` script rather than a built-in command. This ensures clean removal of the binary, service files, crontab entries, configuration, and logs.

## Development

### Building

```bash
make deps                  # Download Go dependencies
make build                 # Build for current platform (output: build/patchmon-agent)
make build-linux           # Build Linux binaries (amd64, 386, arm64, arm)
make build-freebsd         # Build FreeBSD binaries (amd64, 386, arm64, arm)
make build-windows         # Build Windows binaries (amd64, 386 — outputs .exe)
make build-all             # Build Linux + FreeBSD + Windows, copy to agents/
make build-all-for-docker  # Build all platforms into agents-prebuilt/ for local Docker build
make test                  # Run tests
make test-coverage         # Run tests with coverage report
make fmt                   # Format code
make fmt-check             # Verify code is formatted (used in CI)
make vet                   # Run go vet
make lint                  # Lint code (requires golangci-lint)
make check                 # Run fmt-check, vet, lint, test (pre-commit)
make clean                 # Remove build artifacts
make install               # Build and install to /usr/local/bin (Linux/FreeBSD)
```

### Project Structure

```
cmd/patchmon-agent/
  main.go                       Entry point
  commands/
    root.go                     Root command and global flags
    config.go                   config set-api / config show
    connectivity_tests.go       ping command
    report.go                   report command and integration data
    diagnostics.go              diagnostics command
    version_update.go           check-version / update-agent
    serve.go                    serve command (service mode, WebSocket, integrations)
    service_unix.go             Unix service/restart helpers
    service_windows.go          Windows Service Control Manager integration
    sysproc_linux.go            Linux process attributes
    sysproc_freebsd.go          FreeBSD process attributes
    sysproc_darwin.go           macOS process attributes
    sysproc_windows.go          Windows process attributes
internal/
  config/                       Configuration and credentials management (OS-aware paths)
  client/                       HTTP client for PatchMon API
  packages/                     Package managers (apt, dnf, pacman, apk, freebsd, windows)
  repositories/                 Repository detection (apt, dnf, pacman, apk, freebsd, windows)
  system/                       OS detection, system info, reboot status
  hardware/                     CPU, RAM, disk info
  network/                      Network interfaces, DNS, gateway
  crontab/                      Crontab management
  logutil/                      Log sanitisation utilities
  integrations/
    docker/                     Docker container/image/volume/network monitoring
    compliance/                 OpenSCAP, Docker Bench, oscap-docker
  constants/                    Shared constants
  utils/                        Timezone, offset calculation, utilities
  pkgversion/                   Agent version constant
  bufpool/                      Buffer pool for memory optimisation
pkg/models/                     Shared data models and API payloads
```

## License

This project is licensed under AGPL v3 (AGPL-3.0). Copyright 9 Technology Group LTD. See the [LICENSE](LICENSE) file for details.
