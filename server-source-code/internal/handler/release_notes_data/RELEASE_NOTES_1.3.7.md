## 📝 ALERT : Auto-update of Agent issue

Versions <1.3.6 have an issue where the service does not restart after auto-update. OpenRC systems are unaffected and work correctly.
This means you will unfortunately have to use `systemctl start patchmon-agent` on your systems to load up 1.3.7 agent when it auto-updates shortly.

Very sorry for this, future versions are fixed - I built this release notes notification feature specifically to notify you of this.

---

## 🎉 New Features & Improvements :

**Mobile UI**: Mobile user interface improvements are mostly complete, providing a better experience on mobile devices.

**Systemctl Helper Script**: In future versions (1.3.7+), a systemctl helper script will be available to assist with auto-update service restarts.

**Staggered Agent Intervals**: Agents now report at staggered times to prevent overwhelming the PatchMon server. If the agent report interval is set to 60 minutes, different hosts will report at different times. This is in the `config.yml` as "report_offset: nxyz"

**Reboot Detection Information**: Reboot detection information is now stored in the database. When the "Reboot Required" flag is displayed, hovering over it will show the specific reason why a reboot is needed (Reboot feature still needs work and it will be much better in 1.3.8)

**JSON Report Output**: The `patchmon-agent report --json` command now outputs the complete report payload to the console in JSON format instead of sending it to the PatchMon server. This is very useful for integrating PatchMon agent data with other tools and for diagnostic purposes.

**Persistent Docker Toggle**: Docker integration toggle state is now persisted in the database, eliminating in-memory configuration issues. No more losing Docker settings on container restarts (thanks to the community for initiating this feature).

**Config.yml Synchronization**: The agent now writes and compares the `config.yml` file with the server configuration upon startup, ensuring better synchronization of settings between the agent and server.

**Network Information Page**: Enhanced network information page to display IPv6 addresses and support multiple network interfaces, providing more comprehensive network details.

**Auto-Update Logic Fix**: Fixed an issue where agents would auto-update even when per-host auto-update was disabled. The logic now properly honors both server-wide auto-update settings and per-host auto-update settings.

**Prisma Version Fix**: Fixed Prisma version issues affecting Kubernetes deployments by statically setting the Prisma version in package.json files.

**Hiding Github Version**: Added a toggle in Server Version settings to disable showing the github release notes on the login screen

---
