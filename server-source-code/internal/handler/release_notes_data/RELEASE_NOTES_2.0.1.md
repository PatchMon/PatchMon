# PatchMon V2.0.1 Release Notes

A small follow-up release on top of 2.0.0 covering documentation, packaging, and a couple of important fixes.

## Documentation

- Documentation now lives at [https://patchmon.net/docs](https://patchmon.net/docs). The source of truth is this GitHub repository: the public site builds from `PatchMon/docs/*.md` at deploy time, so corrections and contributions flow through the normal pull-request process.

## Fixes

- **Docker image format on older clients**: fixed an image format issue that prevented older Docker and Podman versions from pulling `ghcr.io/patchmon/patchmon-server:2.0.0`. Image layers are now published with gzip compression instead of zstd, so clients without zstd support (Podman versions before 5.7, and Docker installations without the containerd image store) can pull cleanly. See [issue #679](https://github.com/PatchMon/PatchMon/issues/679).
- **Database migration failure at migration 30**: fixed an upgrade path that could fail at migration `000030` on some installations. The migration is now safe to re-run on a partially-upgraded database, so retries succeed without manual intervention.
- **SMTP / TLS**: the **Use TLS** option for email destinations is now respected end-to-end. Notification and scheduled-report delivery no longer upgrade the connection with **STARTTLS** when the server advertises it if you have turned TLS off in the UI.

## New features

- **Email opt-in for security and instance notifications** is now available.

## Upgrade

No special steps. Pull the new image and restart your stack:

```bash
docker compose pull patchmon-server
docker compose up -d patchmon-server
```

Migrations run on startup as usual.
