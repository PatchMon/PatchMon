# Security Compliance - Installation Guide

## OpenSCAP Installation

### Ubuntu/Debian
```bash
# Install OpenSCAP
sudo apt-get update
sudo apt-get install -y openscap-scanner scap-security-guide

# Verify installation
oscap --version
ls /usr/share/xml/scap/ssg/content/
```

### RHEL/CentOS/Rocky
```bash
# Install OpenSCAP
sudo dnf install -y openscap-scanner scap-security-guide

# Or on older systems
sudo yum install -y openscap-scanner scap-security-guide

# Verify installation
oscap --version
ls /usr/share/xml/scap/ssg/content/
```

## Docker Bench for Security

Docker Bench runs as a container, so it only requires Docker to be installed:

```bash
# Verify Docker is running
docker info

# Test Docker Bench manually
docker run --rm --net host --pid host --userns host --cap-add audit_control \
  -v /etc:/etc:ro \
  -v /var/lib:/var/lib:ro \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  docker/docker-bench-security
```

## PatchMon Agent Configuration

The PatchMon agent will automatically detect available compliance tools and run scans.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `COMPLIANCE_SCAN_INTERVAL` | `86400` | Seconds between scheduled scans (default 24h) |
| `COMPLIANCE_ENABLED` | `true` | Enable/disable compliance scanning |

### Manual Scan Trigger

Compliance scans can be triggered on-demand from the PatchMon dashboard or via API:

```bash
# Via API
curl -X POST https://patchmon.example.com/api/v1/compliance/trigger/{host_id} \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{"profile_type": "all"}'
```
