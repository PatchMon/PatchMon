# PatchMonEnhanced - Linux Patch Monitoring made Simple

[![Website](https://img.shields.io/badge/Website-patchmon.net-blue?style=for-the-badge)](https://patchmon.net)
[![Discord](https://img.shields.io/badge/Discord-Join%20Server-blue?style=for-the-badge&logo=discord)](https://patchmon.net/discord)
[![GitHub](https://img.shields.io/badge/GitHub-Repository-black?style=for-the-badge&logo=github)](https://github.com/9technologygroup/patchmon.net)
[![Roadmap](https://img.shields.io/badge/Roadmap-View%20Progress-green?style=for-the-badge&logo=github)](https://github.com/users/9technologygroup/projects/1)
[![Documentation](https://img.shields.io/badge/Documentation-docs.patchmon.net-blue?style=for-the-badge&logo=book)](https://docs.patchmon.net/)

---

## Please STAR this repo :D

## Purpose

PatchMonEnhanced provides centralized patch management across diverse server environments. Agents communicate outbound-only to the PatchMonEnhanced server, eliminating inbound ports on monitored hosts while delivering comprehensive visibility and safe automation.

![Dashboard Screenshot](https://raw.githubusercontent.com/PatchMon/PatchMon/main/dashboard.jpeg)

## Features

### Dashboard
- Customisable dashboard with per‑user card layout and ordering

### Users & Authentication
- Multi-user accounts (admin and standard users)
- Roles, Permissions & RBAC

### Hosts & Inventory
- Host inventory/groups with key attributes and OS details
- Host grouping (create and manage host groups)

### Packages & Updates
- Package inventory across hosts
- Outdated packages overview and counts
- Repositories per host tracking

### Agent & Data Collection
- Agent version management and script content stored in DB

### Settings & Configuration
- Server URL/protocol/host/port
- Signup toggle and default user role selection

### API & Integrations
- REST API under `/api/v1` with JWT auth
- Proxmox LXC Auto-Enrollment - Automatically discover and enroll LXC containers from Proxmox hosts

### Security
- Rate limiting for general, auth, and agent endpoints
- Outbound‑only agent model reduces attack surface

### Security Compliance
- **OpenSCAP** - CIS Benchmarks for Linux hosts (Ubuntu, Debian, RHEL, CentOS, Rocky)
- **Docker Bench for Security** - CIS Docker Benchmark for container security
- Automated scheduled scans (configurable interval)
- On-demand scan triggering from dashboard
- Compliance score tracking over time
- Detailed rule-level results with remediation guidance
- Dashboard with fleet-wide compliance overview

See [Security Compliance Installation Guide](docs/security-compliance/INSTALLATION.md) for setup instructions.

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `COMPLIANCE_SCAN_INTERVAL` | `86400` | Seconds between scheduled scans |
| `COMPLIANCE_ENABLED` | `true` | Enable/disable compliance feature |

### Deployment & Operations
- Docker installation & One‑line self‑host installer (Ubuntu/Debian)
- systemd service for backend lifecycle
- nginx vhost for frontend + API proxy; optional Let’s Encrypt integration


## Getting Started

### PatchMonEnhanced Cloud (coming soon)

Managed, zero-maintenance PatchMonEnhanced hosting. Stay tuned.

### Self-hosted Installation

#### Docker (preferred)

For getting started with Docker, see the [Docker documentation](https://github.com/PatchMon/PatchMon/blob/main/docker/README.md)

#### Native Install (advanced/non-docker)

Run on a clean Ubuntu/Debian server with internet access:

#### Debian:
```bash
apt update -y
apt upgrade -y
apt install curl -y
```

#### Ubuntu:
```bash
apt-get update -y
apt-get upgrade -y
apt install curl -y
```

#### Install Script
```bash
curl -fsSL -o setup.sh https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/setup.sh && chmod +x setup.sh && bash setup.sh
```

#### Update Script (--update flag)
```bash
curl -fsSL -o setup.sh https://raw.githubusercontent.com/PatchMon/PatchMon/refs/heads/main/setup.sh && chmod +x setup.sh && bash setup.sh --update
```

#### Minimum specs for building : #####
CPU : 2 vCPU
RAM : 2GB
Disk : 15GB

During setup you’ll be asked:
- Domain/IP: public DNS or local IP (default: `patchmon.internal`)
- SSL/HTTPS: `y` for public deployments with a public IP, `n` for internal networks
- Email: only if SSL is enabled (for Let’s Encrypt)
- Git Branch: default is `main` (press Enter)

The script will:
- Install prerequisites (Node.js, PostgreSQL, nginx)
- Clone the repo, install dependencies, build the frontend, run migrations
- Create a systemd service and nginx site vhost config
- Start the service and write a consolidated info file at:
  - `/opt/<your-domain>/deployment-info.txt`
  - Copies the full installer log to `/opt/<your-domain>/patchmon-install.log` from /var/log/patchmon-install.log

After installation:
- Visit `http(s)://<your-domain>` and complete first-time admin setup
- See all useful info in `deployment-info.txt`

## Forcing updates after host package changes
Should you perform a manual package update on your host and wish to see the results reflected in PatchMonEnhanced quicker than the usual scheduled update, you can trigger the process manually by running:
```bash
/usr/local/bin/patchmon-agent.sh update
```

This will send the results immediately to PatchMonEnhanced.

## Communication Model

- Outbound-only agents: servers initiate communication to PatchMonEnhanced
- No inbound connections required on monitored servers
- Secure server-side API with JWT authentication and rate limiting

## Architecture

- Backend: Node.js/Express + Prisma + PostgreSQL
- Frontend: Vite + React
- Reverse proxy: nginx
- Database: PostgreSQL
- System service: systemd-managed backend

```mermaid
flowchart LR
    A[End Users / Browser<br>Admin UI / Frontend] -- HTTPS --> B[nginx<br>serve FE, proxy API]
    B -- HTTP --> C["Backend<br>(Node/Express)<br>/api, auth, Prisma"]
    C -- TCP --> D[PostgreSQL<br>Database]

    E["Agents on your servers (Outbound Only)"] -- HTTPS --> F["Backend API<br>(/api/v1)"]
```
Operational
- systemd manages backend service
- certbot/nginx for TLS (public)
- setup.sh bootstraps OS, app, DB, config

## Support

- Discord: [https://patchmon.net/discord](https://patchmon.net/discord)
- Email: support@patchmon.net

## Roadmap

- Roadmap board: https://github.com/orgs/PatchMon/projects/2


## License

- AGPLv3 (More information on this soon)


---

## 🤝 Contributing

We welcome contributions from the community! Here's how you can get involved:

### Development Setup
1. **Fork the Repository**
   ```bash
   # Click the "Fork" button on GitHub, then clone your fork
   git clone https://github.com/YOUR_USERNAME/patchmon.net.git
   cd patchmon.net
   ```

2. **Create a Feature Branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

4. **Install Dependencies and Setup Hooks**
   ```bash
   npm install
   npm run prepare
   ```

5. **Make Your Changes**
   - Write clean, well-documented code
   - Follow existing code style and patterns
   - Add tests for new functionality
   - Update documentation as needed

6. **Test Your Changes**
   ```bash
   # Run backend tests
   cd backend
   npm test
   
   # Run frontend tests
   cd ../frontend
   npm test
   ```

7. **Commit and Push**
   ```bash
   git add .
   git commit -m "Add: descriptive commit message"
   git push origin feature/your-feature-name
   ```

8. **Create a Pull Request**
   - Go to your fork on GitHub
   - Click "New Pull Request"
   - Provide a clear description of your changes
   - Link any related issues

### Contribution Guidelines
- **Code Style**: Follow the existing code patterns and Biome configuration
- **Commits**: Use conventional commit messages (feat:, fix:, docs:, etc.)
- **Testing**: Ensure all tests pass and add tests for new features
- **Documentation**: Update README and code comments as needed
- **Issues**: Check existing issues before creating new ones

---


## 🏢 Enterprise & Custom Solutions

### PatchMonEnhanced Cloud
- **Fully Managed**: We handle all infrastructure and maintenance
- **Scalable**: Grows with your organization
- **Secure**: Enterprise-grade security and compliance
- **Support**: Dedicated support team

### Custom Integrations
- **API Development**: Custom endpoints for your specific needs
- **Third-Party Integrations**: Connect with your existing tools
- **Custom Dashboards**: Tailored reporting and visualization
- **White-Label Solutions**: Brand PatchMonEnhanced as your own

### Enterprise Deployment
- **On-Premises**: Deploy in your own data center
- **Air-Gapped**: Support for isolated environments
- **Compliance**: Meet industry-specific requirements
- **Training**: Comprehensive team training and onboarding

*Contact us at support@patchmon.net for enterprise inquiries*

---



---

## 🙏 Acknowledgments

### Special Thanks
- **Jonathan Higson** - For inspiration, ideas, and valuable feedback
- **@Adam20054** - For working on Docker Compose deployment
- **@tigattack** - For working on GitHub CI/CD pipelines
- **Cloud X** and **Crazy Dead** - For moderating our Discord server and keeping the community awesome
- **Beta Testers** - For keeping me awake at night
- **My family** - For understanding my passion
  

### Contributors
Thank you to all our contributors who help make PatchMonEnhanced better every day!


## 🔗 Links

- **Website**: [patchmon.net](https://patchmon.net)
- **Discord**: [https://patchmon.net/discord](https://patchmon.net/discord)
- **Roadmap**: [GitHub Projects](https://github.com/users/9technologygroup/projects/1)
- **Documentation**: [https://docs.patchmon.net](https://docs.patchmon.net)
- **Support**: support@patchmon.net

---

<div align="center">

**Made with ❤️ by the PatchMonEnhanced Team**

[![Discord](https://img.shields.io/badge/Discord-Join%20Server-blue?style=for-the-badge&logo=discord)](https://patchmon.net/discord)
[![GitHub](https://img.shields.io/badge/GitHub-Repository-black?style=for-the-badge&logo=github)](https://github.com/PatchMon/PatchMon)

</div>
