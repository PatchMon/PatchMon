# Security Compliance Agent Deployment

## Overview

The compliance scanning feature uses a **Python-based agent** that runs alongside the existing PatchMon Go agent. This agent handles CIS benchmark scanning using OpenSCAP and Docker Bench for Security.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Host System                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────────┐    ┌─────────────────────────────┐ │
│  │   PatchMon Go Agent │    │  Compliance Python Agent    │ │
│  │   (existing)        │    │  (new)                      │ │
│  │                     │    │                             │ │
│  │  - Host info        │    │  - OpenSCAP scanning        │ │
│  │  - Package updates  │    │  - Docker Bench scanning    │ │
│  │  - System metrics   │    │  - Scheduled scans          │ │
│  │                     │    │  - On-demand via WebSocket  │ │
│  └──────────┬──────────┘    └──────────────┬──────────────┘ │
│             │                              │                 │
│             │         WebSocket            │                 │
│             └──────────────┬───────────────┘                 │
│                            │                                 │
└────────────────────────────┼─────────────────────────────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  PatchMon       │
                    │  Server         │
                    └─────────────────┘
```

## Prerequisites

### Required on Host

1. **Python 3.8+**
   ```bash
   python3 --version
   ```

2. **OpenSCAP** (for host compliance scanning)
   ```bash
   # Ubuntu/Debian
   sudo apt-get install -y openscap-scanner scap-security-guide

   # RHEL/CentOS/Rocky
   sudo dnf install -y openscap-scanner scap-security-guide
   ```

3. **Docker** (for Docker Bench scanning - optional)
   ```bash
   docker --version
   ```

4. **Python Dependencies**
   ```bash
   pip3 install aiohttp
   ```

## Installation

### 1. Copy Agent Files

Copy the compliance agent to the host:

```bash
# From your PatchMon-Enhanced repo
scp -r agent/ user@host:/opt/patchmon-compliance/
```

Or clone directly:

```bash
cd /opt
git clone https://github.com/MacJediWizard/PatchMon-Enhanced.git
ln -s /opt/PatchMon-Enhanced/agent /opt/patchmon-compliance
```

### 2. Configure Credentials

Create credentials file:

```bash
sudo mkdir -p /etc/patchmon
sudo nano /etc/patchmon/credentials
```

Add credentials (same as Go agent):

```
PATCHMON_SERVER=https://your-patchmon-server.com
API_ID=your-host-api-id
API_KEY=your-host-api-key
```

Set permissions:

```bash
sudo chmod 600 /etc/patchmon/credentials
```

### 3. Create Systemd Service

Create `/etc/systemd/system/patchmon-compliance.service`:

```ini
[Unit]
Description=PatchMon Compliance Scanner
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/patchmon-compliance
ExecStart=/usr/bin/python3 -m lib.patchmon_agent
Restart=always
RestartSec=30
Environment="PYTHONPATH=/opt/patchmon-compliance"

# Optional: Override default settings
# Environment="COMPLIANCE_SCAN_INTERVAL=86400"
# Environment="COMPLIANCE_ENABLED=true"

[Install]
WantedBy=multi-user.target
```

### 4. Enable and Start Service

```bash
sudo systemctl daemon-reload
sudo systemctl enable patchmon-compliance
sudo systemctl start patchmon-compliance
```

### 5. Verify Installation

Check service status:

```bash
sudo systemctl status patchmon-compliance
```

Check logs:

```bash
sudo journalctl -u patchmon-compliance -f
```

Expected startup logs:

```
Starting PatchMon agent, connecting to https://your-server.com
OpenSCAP available: True
OS: ubuntu 22
Available profiles: ['level1_server', 'level2_server']
Docker Bench available: True
Connected to PatchMon server
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PATCHMON_SERVER` | `https://patchmon.example.com` | Server URL |
| `API_ID` | (required) | Host API ID |
| `API_KEY` | (required) | Host API Key |
| `COMPLIANCE_SCAN_INTERVAL` | `86400` | Seconds between scheduled scans (default 24h) |
| `COMPLIANCE_ENABLED` | `true` | Enable/disable compliance scanning |

### Credentials File

The agent reads credentials from `/etc/patchmon/credentials` first, then environment variables override.

## Usage

### Scheduled Scans

By default, the agent runs compliance scans every 24 hours. Configure via:

```bash
# In systemd service file
Environment="COMPLIANCE_SCAN_INTERVAL=43200"  # 12 hours
```

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

### Manual Test Run

```bash
cd /opt/patchmon-compliance
python3 -m lib.patchmon_agent
```

## Troubleshooting

### Agent Not Connecting

1. Check credentials:
   ```bash
   cat /etc/patchmon/credentials
   ```

2. Test server connectivity:
   ```bash
   curl https://your-server.com/api/v1/health
   ```

3. Check WebSocket connection:
   ```bash
   sudo journalctl -u patchmon-compliance | grep -i websocket
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

1. Check scan submission:
   ```bash
   sudo journalctl -u patchmon-compliance | grep -i "scan results submitted"
   ```

2. Verify API endpoint:
   ```bash
   curl -X POST https://your-server.com/api/v1/compliance/scans \
     -H "X-API-ID: your-api-id" \
     -H "X-API-KEY: your-api-key" \
     -H "Content-Type: application/json" \
     -d '{"profile_name": "test", "profile_type": "test", "results": []}'
   ```

## Updating

```bash
cd /opt/PatchMon-Enhanced
git pull
sudo systemctl restart patchmon-compliance
```

## Uninstalling

```bash
sudo systemctl stop patchmon-compliance
sudo systemctl disable patchmon-compliance
sudo rm /etc/systemd/system/patchmon-compliance.service
sudo systemctl daemon-reload
rm -rf /opt/patchmon-compliance
```
