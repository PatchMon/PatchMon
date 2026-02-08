# PatchMon Agent

PatchMon's monitoring agent sends package and repository information to the PatchMon server.

## Installation

### Binary Installation

1. **Download** the appropriate binary for your architecture from the releases
2. **Make executable** and move to system path:
   ```bash
   chmod +x patchmon-agent-linux-amd64
   sudo mv patchmon-agent-linux-amd64 /usr/local/bin/patchmon-agent
   ```

### From Source

1. **Prerequisites**:
   - Go 1.25 or later
   - Root access on the target system

2. **Build and Install**:
   ```bash
   # Clone or copy the source code
   make deps          # Install dependencies
   make build         # Build the application
   sudo make install  # Install to /usr/local/bin
   ```

## Configuration

### Initial Setup

1. **Configure Credentials**:
   ```bash
   sudo patchmon-agent config set-api <API_ID> <API_KEY> <SERVER_URL>
   ```

   Example:
   ```bash
   sudo patchmon-agent config set-api patchmon_1a2b3c4d abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890 http://patchmon.example.com
   ```

2. **Test Configuration**:
   ```bash
   sudo patchmon-agent ping
   ```

3. **Send Initial Report**:
   ```bash
   sudo patchmon-agent report
   ```

### Configuration Files

- **Main Config**: `/etc/patchmon/config.yml` (YAML format)
- **Credentials**: `/etc/patchmon/credentials.yml` (YAML format, 600 permissions)
- **Logs**: `/var/log/patchmon-agent.log`

## Usage

### Available Commands

```bash
# Configuration and setup
sudo patchmon-agent config set-api <API_ID> <API_KEY> <SERVER_URL>  # Configure credentials
sudo patchmon-agent config show                                     # Show current config
sudo patchmon-agent ping                                            # Test credentials and connectivity

# Data collection and reporting
sudo patchmon-agent report                                          # Report system & package status to server

# Agent management
sudo patchmon-agent check-version                                   # Check for updates
sudo patchmon-agent update-agent                                    # Update to latest version
sudo patchmon-agent update-crontab                                  # Update cron schedule
sudo patchmon-agent uninstall [flags]                               # Uninstall the agent

# Diagnostics
sudo patchmon-agent diagnostics                                     # Show system diagnostics
```

### Example Configuration File

Create `/etc/patchmon/config.yml`:

```yaml
patchmon_server: "https://patchmon.example.com"
api_version: "v1"
credentials_file: "/etc/patchmon/credentials.yml"
log_file: "/var/log/patchmon-agent.log"
log_level: "info"
```

### Example Credentials File

The credentials file is automatically created by the `configure` command:

```yaml
api_id: "patchmon_1a2b3c4d5e6f7890"
api_key: "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
```

## Automation

### Crontab Setup

The agent can automatically configure crontab based on server policies:

```bash
# Update crontab with current server policy
sudo patchmon-agent update-crontab
```

This creates entries like:
```bash
# Hourly reports (at minute 15)
15 * * * * /usr/local/bin/patchmon-agent report >/dev/null 2>&1
15 * * * * /usr/local/bin/patchmon-agent update-crontab >/dev/null 2>&1

# Or custom interval (every 30 minutes)
*/30 * * * * /usr/local/bin/patchmon-agent report >/dev/null 2>&1
*/30 * * * * /usr/local/bin/patchmon-agent update-crontab >/dev/null 2>&1
```

## Uninstallation

The agent includes a built-in uninstall command for complete removal:

### Basic Uninstall
```bash
# Remove agent binary, crontab entries, and backup files
sudo patchmon-agent uninstall
```

### Complete Uninstall
```bash
# Remove everything including configuration and logs
sudo patchmon-agent uninstall --remove-config --remove-logs

# Or use the shortcut flag
sudo patchmon-agent uninstall --remove-all  # or -a

# Silent complete removal
sudo patchmon-agent uninstall -af
```

### Uninstall Options
```bash
--remove-config    # Remove configuration and credentials files
--remove-logs      # Remove log files  
--remove-all, -a   # Remove all files (shortcut for --remove-config --remove-logs)
--force, -f        # Skip confirmation prompts
```

### What Gets Removed

**Always removed:**
- Agent binary (current executable)
- Additional binaries found in common locations
- Crontab entries related to patchmon-agent
- Backup files created during updates

**Optional (with flags):**
- Configuration files (`--remove-config`)
- Credentials files (`--remove-config`) 
- Log files (`--remove-logs`)

The uninstall process will:
1. Show what will be removed
2. Prompt for confirmation (unless `--force` is used)
3. Remove crontab entries first
4. Remove additional files and binaries
5. Use a self-destruct mechanism to remove the main binary

## Logging

Logs are written to `/var/log/patchmon-agent.log` with timestamps and structured format:

```
2023-09-27T10:30:00 level=info msg="Collecting package information..."
2023-09-27T10:30:01 level=info msg="Found packages" count=156
2023-09-27T10:30:02 level=info msg="Sending report to PatchMon server..."
2023-09-27T10:30:03 level=info msg="Report sent successfully"
```

Log levels: `debug`, `info`, `warn`, `error`

## Troubleshooting

### Common Issues

1. **Permission Denied**:
   ```bash
   # Ensure running as root
   sudo patchmon-agent <command>
   ```

2. **Credentials Not Found**:
   ```bash
   # Configure credentials first
   sudo patchmon-agent config set-api <API_ID> <API_KEY> <SERVER_URL>
   ```

3. **Network Connectivity**:
   ```bash
   # Test server connectivity
   sudo patchmon-agent ping
   sudo patchmon-agent diagnostics  # Detailed network info
   ```

4. **Package Manager Issues**:
   ```bash
   # Update package lists manually
   sudo apt update         # Ubuntu/Debian
   sudo dnf check-update   # Fedora/RHEL
   ```

### Diagnostics

Run comprehensive diagnostics:

```bash
sudo patchmon-agent diagnostics
```

This returns information about your system, the agent, the current configuration and server connectivity state, and more.

## Migration from Shell Script

The Go implementation maintains compatibility with the existing shell script workflow:

1. **Same command structure**: All commands work identically
2. **Same configuration files**: Uses the same paths and formats
3. **Same API compatibility**: Works with existing PatchMon servers
4. **Improved performance**: Faster execution and better error handling

To migrate:
1. Remove the old shell script agent, config, credentials, and crontab.
2. Install the Go binary as described above
3. No changes needed to crontab or server settings

## Development

### Building

```bash
# Install dependencies
make deps

# Build for current platform
make build

# Build for all supported platforms (Linux amd64, arm64, i386)
make build-all

# Run tests
make test

# Run tests with coverage
make test-coverage

# Format and lint
make fmt
make lint

# Clean build artifacts
make clean
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Run `make fmt` and `make lint`
6. Submit a pull request
