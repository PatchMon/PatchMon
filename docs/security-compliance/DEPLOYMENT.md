# Security Compliance Agent Deployment

## Overview

The compliance scanning feature is **integrated into the PatchMon Go agent**. When you install or update the PatchMon agent from the PatchMonEnhanced-agent repository, compliance scanning is automatically included.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Host System                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │              PatchMon Enhanced Agent                     │ │
│  │              (single Go binary)                          │ │
│  │                                                          │ │
│  │  Core Features:           Compliance Integration:        │ │
│  │  - Host info              - OpenSCAP CIS scanning        │ │
│  │  - Package updates        - Docker Bench scanning        │ │
│  │  - System metrics         - Automatic profile detection  │ │
│  │  - Docker integration     - Score calculation            │ │
│  │                                                          │ │
│  └──────────────────────────┬───────────────────────────────┘ │
│                             │                                 │
│                             │ HTTPS API                       │
│                             │                                 │
└─────────────────────────────┼─────────────────────────────────┘
                              │
                              ▼
                     ┌─────────────────┐
                     │  PatchMon       │
                     │  Enhanced       │
                     │  Server         │
                     └─────────────────┘
```

## Prerequisites

### Required on Host

1. **OpenSCAP** (for host compliance scanning)
   ```bash
   # Ubuntu/Debian
   sudo apt-get install -y openscap-scanner scap-security-guide

   # RHEL/CentOS/Rocky/Alma
   sudo dnf install -y openscap-scanner scap-security-guide
   ```

2. **Docker** (for Docker Bench scanning - optional)
   ```bash
   docker --version
   ```

## Installation

### Option 1: Download Pre-built Binary

Download the latest release from:
https://github.com/MacJediWizard/PatchMonEnhanced-agent/releases

```bash
# Download for your architecture
curl -L https://github.com/MacJediWizard/PatchMonEnhanced-agent/releases/latest/download/patchmon-agent-linux-amd64 -o patchmon-agent
chmod +x patchmon-agent
sudo mv patchmon-agent /usr/local/bin/
```

### Option 2: Build from Source

Requires Go 1.21+:

```bash
git clone https://github.com/MacJediWizard/PatchMonEnhanced-agent.git
cd PatchMonEnhanced-agent
make deps
make build
sudo make install
```

### Configure Credentials

```bash
sudo patchmon-agent config set-api \
  --api-id your-host-api-id \
  --api-key your-host-api-key \
  --server https://your-patchmon-server.com
```

### Test Connection

```bash
sudo patchmon-agent ping
```

### Run Initial Report (includes compliance scan)

```bash
sudo patchmon-agent report
```

## Enabling Compliance Integration

Compliance scanning is enabled by default. To configure it, edit `/etc/patchmon/config.yml`:

```yaml
integrations:
  docker: true
  compliance: true  # Enable compliance scanning
```

## Supported Compliance Profiles

### OpenSCAP CIS Benchmarks

The agent automatically detects your OS and selects the appropriate CIS profile:

| OS | Level 1 Server | Level 2 Server |
|----|----------------|----------------|
| Ubuntu | ✓ | ✓ |
| Debian | ✓ | ✓ |
| RHEL | ✓ | ✓ |
| Rocky Linux | ✓ | ✓ |
| Alma Linux | ✓ | ✓ |
| Fedora | ✓ | - |
| SLES/openSUSE | ✓ | - |

### Docker Bench for Security

If Docker is installed, the agent runs Docker Bench for Security to check:
- Host Configuration
- Docker Daemon Configuration
- Docker Daemon Configuration Files
- Container Images and Build Files
- Container Runtime
- Docker Security Operations
- Docker Swarm Configuration

## Usage

### Automatic Scans

Compliance scans run automatically with each agent report (typically hourly via cron).

### On-Demand Scans

Trigger scans from the PatchMon dashboard:

1. Go to **Hosts** → Select a host → **Compliance** tab
2. Click **Run Scan**

Or via API:

```bash
curl -X POST https://your-server.com/api/v1/compliance/trigger/{host-id} \
  -H "Authorization: Bearer {jwt-token}" \
  -H "Content-Type: application/json" \
  -d '{"profile_type": "all"}'
```

## Troubleshooting

### Compliance Not Running

1. Check if compliance integration is enabled:
   ```bash
   cat /etc/patchmon/config.yml | grep compliance
   ```

2. Run agent manually to see compliance output:
   ```bash
   sudo patchmon-agent report
   ```

### OpenSCAP Not Available

1. Verify installation:
   ```bash
   oscap --version
   ```

2. Check SCAP content:
   ```bash
   ls /usr/share/xml/scap/ssg/content/
   ```

3. Test manual scan:
   ```bash
   oscap xccdf eval --profile xccdf_org.ssgproject.content_profile_cis_level1_server \
     /usr/share/xml/scap/ssg/content/ssg-ubuntu2204-ds.xml
   ```

### Docker Bench Not Running

1. Verify Docker is running:
   ```bash
   docker info
   ```

2. Test Docker Bench manually:
   ```bash
   docker run --rm --net host --pid host --userns host --cap-add audit_control \
     -v /etc:/etc:ro -v /var/lib:/var/lib:ro \
     -v /var/run/docker.sock:/var/run/docker.sock:ro \
     docker/docker-bench-security
   ```

### Scans Not Appearing in Dashboard

1. Check agent logs:
   ```bash
   sudo journalctl -u patchmon-agent | grep -i compliance
   ```

2. Verify the agent report includes compliance data:
   ```bash
   sudo patchmon-agent report --json | grep -A5 compliance
   ```

## Updating

```bash
# Download latest binary
curl -L https://github.com/MacJediWizard/PatchMonEnhanced-agent/releases/latest/download/patchmon-agent-linux-amd64 -o patchmon-agent
chmod +x patchmon-agent
sudo mv patchmon-agent /usr/local/bin/

# Or use the built-in update
sudo patchmon-agent update-agent
```

## Uninstalling

```bash
sudo patchmon-agent uninstall --remove-all
```

Or manually:

```bash
sudo systemctl stop patchmon-agent
sudo systemctl disable patchmon-agent
sudo rm /usr/local/bin/patchmon-agent
sudo rm -rf /etc/patchmon
```
