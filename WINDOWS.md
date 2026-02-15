# Windows Support

PatchMon supports Windows hosts alongside Linux. A lightweight agent runs as a Windows Service and reports package (Windows Update) and repository (update source) information to your PatchMon server.

---

## Requirements

- **OS:** Windows Server 2016+ or Windows 10/11
- **PowerShell:** 5.1 or later (typically included)
- **Administrator:** Install and uninstall must be run as Administrator
- **Network:** Outbound HTTPS to your PatchMon server (no inbound ports on the Windows host)

---

## Installing the Agent

1. **Add the host in PatchMon**
   - In the PatchMon UI, go to **Hosts** and add a new host (or use an existing pending host).
   - Open the **credentials / quick install** modal for that host.

2. **Select Windows**
   - In the install modal, choose **Windows** (instead of Linux/Unix).
   - The UI will show a PowerShell command that includes your server URL and API credentials.

3. **Run PowerShell as Administrator**
   - On the Windows machine, open **PowerShell as Administrator** (right‑click → Run as administrator).
   - Copy the full command from PatchMon. It will look like:
     ```powershell
     $script = Invoke-WebRequest -Uri "https://your-patchmon-server/api/v1/hosts/install?os=windows" -Headers @{"X-API-ID"="..."; "X-API-KEY"="..."} -UseBasicParsing; $script.Content | Out-File -FilePath "$env:TEMP\patchmon-install.ps1" -Encoding utf8; powershell.exe -ExecutionPolicy Bypass -File "$env:TEMP\patchmon-install.ps1"
     ```
   - Paste and run the command. The script downloads the agent from your server, installs it under `C:\Program Files\PatchMon`, and registers a Windows Service.

4. **Confirm in PatchMon**
   - The host should appear as **online** in PatchMon within a few minutes. Package and update-source data will start populating after the first report.

---

## What the Agent Collects

- **Packages:** Installed and available updates from Windows Update (including security and other classifications).
- **Repositories / sources:** Windows Update sources (e.g. Windows Update, WSUS, Microsoft Update).
- **System info:** Hostname, OS version, architecture, and other standard host metadata.

The agent uses the same outbound-only model as the Linux agent: it connects to your PatchMon server on a schedule; no inbound ports are opened on the Windows host.

---

## Uninstalling the Agent

**Standard removal** (removes service and binaries; keeps config and logs):

- From PatchMon: **Settings → Agent Updates** (or **Settings → Agent Uninstall Command**), select **Windows**, then copy the **Standard Removal** command.
- On the Windows machine, run PowerShell as Administrator and execute the copied command. It downloads and runs the removal script from your server.

**Complete removal** (removes service, binaries, config, and logs):

- Use the **Complete Removal** command from the same Settings section. The script supports `-RemoveAll -Force` for a full cleanup.

You can also run the removal script manually if you have it locally:

```powershell
.\patchmon_remove_windows.ps1              # Standard removal
.\patchmon_remove_windows.ps1 -RemoveAll  # Remove config and logs too
```

---

## Example Install Output

When installation succeeds, you should see output similar to the following:

```powershell
PS C:\Users\Administrator> $script = Invoke-WebRequest -Uri "https://your-patchmon-server/api/v1/hosts/install?os=windows" -Headers @{"X-API-ID"="YOUR_API_ID"; "X-API-KEY"="YOUR_API_KEY"} -UseBasicParsing; $script.Content | Out-File -FilePath "$env:TEMP\patchmon-install.ps1" -Encoding utf8; powershell.exe -ExecutionPolicy Bypass -File "$env:TEMP\patchmon-install.ps1"
Fetching credentials from PatchMon server...
Credentials received successfully.
PatchMon Agent Installation for Windows
=======================================
Downloading from PatchMon server: https://your-patchmon-server/api/v1/hosts/agent/download?arch=amd64&os=windows&force=binary
Architecture: amd64
Install Path: C:\Program Files\PatchMon
Config Path: C:\ProgramData\PatchMon

Creating installation directory...
Creating configuration directory...
Downloading PatchMon agent...
Download completed from server.
Stopping existing PatchMon Agent service...
Installing agent to C:\Program Files\PatchMon\patchmon-agent.exe...
Configuring API credentials...
Credentials configured successfully.

Testing installation...
✅ API credentials are valid
✅ Connectivity test successful
Installation test successful!

Setting up Windows Service...
Service already exists, stopping it...
Creating Windows Service...
Service created successfully.
Starting service...
Service started successfully!

PatchMon Agent installation completed successfully!

Installation Summary:
   • Configuration directory: C:\ProgramData\PatchMon
   • Agent binary installed: C:\Program Files\PatchMon\patchmon-agent.exe
   • Architecture: amd64
   • Windows Service: configured and running
   • API credentials configured and tested
   • Logs: C:\ProgramData\PatchMon\patchmon-agent.log

Management Commands:
   • Test connection: patchmon-agent ping
   • Manual report: patchmon-agent report
   • Check status: patchmon-agent diagnostics
   • Service status: Get-Service -Name PatchMonAgent
   • Service logs: Get-Content "C:\ProgramData\PatchMon\patchmon-agent.log" -Tail 50 -Wait
   • Restart service: Restart-Service -Name PatchMonAgent

Your system is now being monitored by PatchMon!
```

---

## Troubleshooting

- **"This script must be run as Administrator"**  
  Right‑click PowerShell and choose **Run as Administrator**.

- **Host stays "pending" or never appears online**  
  Check that the Windows host can reach the PatchMon server over HTTPS (port 443 or your configured port). Ensure firewall or proxy allows outbound connections from the Windows host to the server.

- **No packages or updates shown**  
  The agent reports Windows Update data. If the host uses WSUS or group policy that restricts Windows Update, the list may reflect that. Allow the agent a few minutes after first install to run its initial report.

---

*For Linux agent installation and general documentation, see the [main README](README.md) and [docs.patchmon.net](https://docs.patchmon.net).*
