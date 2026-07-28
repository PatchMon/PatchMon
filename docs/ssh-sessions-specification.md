# PatchMon Remote Sessions Specification

## 1. Status and purpose

This document defines the implemented reference behavior for PatchMon remote access. It consolidates the specifications for the CLI, recorded PTY bastion, raw tunnels, local Linux authentication, session recording, replay, and the **Sessions** web page.

The following superseded designs are not part of the active specification:

- the web terminal no longer uses the legacy `ssh_proxy` path;
- PTY sessions no longer execute `/usr/bin/login` or `/bin/login`;
- PAM, LDAP, Active Directory/SSSD, and Linux MFA are outside the first version;
- PatchMon never stores a managed host password or Linux private key.

## 2. Access paths

PatchMon exposes two separate paths over the agent's existing outbound WebSocket.

### 2.1 Recorded interactive session

```console
patchmon login --server https://patchmon.example.com
patchmon instances list
patchmon ssh deploy@host
```

`patchmon ssh user@host` opens a human PTY session:

1. the CLI authenticates to PatchMon and stores a revocable token locally;
2. it resolves the host and requested Linux account;
3. it generates a temporary Ed25519 key;
4. the server issues a five-minute SSH certificate scoped to the tenant, PatchMon user, host, and Linux principal;
5. OpenSSH connects to the PatchMon bastion;
6. the agent prompts for the selected account's local password directly in the PTY;
7. after validating `/etc/shadow`, the agent starts the account's login shell;
8. permitted interactive events are recorded and replayable.

The five-minute certificate lifetime only limits when a new connection may be established. It does not terminate a session that is already connected.

The CLI displays the recording warning and the OpenSSH local escape instruction: press Enter, then type `~.`.

### 2.2 Unrecorded raw tunnel

```console
patchmon ssh-tunnel host
ssh -o 'ProxyCommand=patchmon ssh-tunnel host' user@host
```

The tunnel relays raw TCP for OpenSSH, Ansible, SCP/SFTP, and automation:

- the first version only permits port 22;
- payload remains end-to-end encrypted and is neither inspected nor recorded;
- connection metadata is retained for audit;
- tunnels appear on the Sessions page as non-replayable entries.

Managed hosts do not expose their SSH port. Traffic uses the agent's outbound connection.

## 3. Authorization and authentication

### 3.1 PatchMon authorization

Before opening a session, the server verifies:

- an authenticated and active PatchMon user;
- tenant isolation;
- the `can_use_remote_access` permission;
- the host and an active agent connection;
- remote access module availability;
- the requested Linux account in the host allow-list;
- concurrent session limits per PatchMon user and host.

For CLI access, the SSH certificate principal must exactly match the requested Linux account. The account passed to the agent is immutable; PTY input can never select another identity.

### 3.2 Local Shadow authentication

For CLI and web interactive sessions, the agent:

- rejects `root`, every UID 0 account, and missing accounts;
- reads only the matching entry from `/etc/shadow`;
- rejects empty hashes, locked accounts, expired accounts, expired passwords, and unsupported hashes;
- supports yescrypt (`$y$`), SHA-512 crypt (`$6$`), SHA-256 crypt (`$5$`), and MD5 crypt (`$1$`);
- displays one `Password:` prompt with terminal ECHO disabled;
- accepts at most 1,024 password bytes with a 60-second authentication timeout;
- closes on the first failure with status `failed`, without showing a `login:` prompt or accepting another identity;
- applies a short delay after failure to slow repeated attempts.

After success, the agent applies supplementary groups, GID, and UID, then starts the shell declared in `/etc/passwd` as a login shell. It sets `HOME`, `USER`, `LOGNAME`, `SHELL`, `TERM`, and a restricted `PATH`. Missing or non-executable shells, `nologin`, and `false` are rejected.

This version does not modify `/etc/login.defs`, `/etc/passwd`, `/etc/shadow`, or PAM configuration.

## 4. PTY protocol and stream security

The multiplexed server-to-agent protocol supports:

- session open;
- terminal output;
- sequenced input;
- input acknowledgement with the observed ECHO state;
- resize;
- `INT`, `TERM`, and `HUP` signals;
- process exit and session close.

The agent creates the PTY before authentication, disables ECHO, consumes the password locally, and attaches the shell to the same terminal after successful verification. Input received while the password hash is being evaluated is discarded to prevent pre-authentication command injection.

After authentication, input is forwarded to the shell process group. Resize and signals are propagated. Connection loss closes the PTY and sends SIGHUP to the process group.

The password is never sent as a dedicated API field. Input is added to a recording only after the agent acknowledges it with `echo=true`.

## 5. Unified web terminal

The web terminal uses the same PTY broker and recording service as the CLI.

The connection interface provides:

- the host's allowed Linux accounts;
- account selection;
- a Connect button;
- a warning that the session is recorded and no-ECHO input is excluded.

Connection is disabled with an explicit message when no account is allowed or the agent is offline. Legacy password, private key, port, and direct/proxy controls are no longer displayed.

The frontend keeps the `connected`, `data`, `error`, and `closed` messages. The historical `ssh_proxy` protocol remains agent-side for compatibility but is no longer used by the web terminal.

## 6. Recording, replay, and audit

Recorded CLI and web sessions store:

- terminal output;
- input only while ECHO is enabled;
- resize events;
- markers and relative timestamps.

Local passwords and all other no-ECHO input must be absent from events, replay data, and logs. Secrets printed by an application may still appear in terminal output.

Events are block-compressed and encrypted at rest with AES-256-GCM. PostgreSQL stores metadata and indexes; encrypted blocks use the configured local artifact store. A future S3 implementation must be replaceable without changing protocol or metadata semantics.

Default retention is 90 days and is configurable. Automatic deletion is audited. Replay access requires `can_view_session_recordings`, separately from remote access permission.

Reference statuses are:

- `active`: session in progress;
- `completed`: normal shell termination;
- `failed`: authentication, initialization, or relay failure;
- `disconnected`: client, agent, or server connection loss.

A `pty_exited` message containing an error produces `failed`, including local password rejection.

## 7. Sessions page

The compatible route remains `/ssh-recordings`, while the visible navigation label and page title are **Sessions**.

The page combines interactive sessions and tunnel audits and provides:

- Refresh action;
- Total, Active, Interactive, and Failed summary cards;
- search across host, PatchMon user, Linux account, client address, and status;
- type and status filters;
- a blue table header and colored badges;
- pagination with 10, 25, or 50 rows;
- local date formatting from the ISO `started_at` string, with `-` for missing or invalid values;
- Replay only for available recorded sessions.

Visual conventions:

- `completed`: green;
- `failed`: red;
- `disconnected`: amber;
- `active`: blue with a pulsing indicator;
- interactive session: cyan;
- tunnel: purple.

The web player reconstructs the timestamped stream without generating MP4. It provides play/pause, speed, timeline navigation, fullscreen, and text search.

## 8. Configuration and deployment

Each agent requires explicit local activation:

```yaml
integrations:
  ssh-bastion-enabled: true
```

The agent must run with the root privileges required to read `/etc/shadow`, allocate PTYs, and change UID/GID.

The server deployment must:

- enable the SSH bastion;
- publish its TCP port;
- retain a stable SSH host key;
- retain the encrypted SSH CA and separate passphrase;
- configure a dedicated recording encryption key;
- retain previous CA public keys during rotation until certificates signed by them have expired.

The legacy `ssh-proxy-enabled` option does not replace `ssh-bastion-enabled` for recorded interactive sessions.

## 9. Acceptance criteria

A release must verify:

- successful and unsuccessful local password authentication from CLI and web;
- inability to enter `root` or another login after a failed password;
- rejection of root, UID 0, locked or expired accounts, unsupported hashes, and invalid shells;
- effective shell UID, GID, and supplementary groups;
- absence of password and hash data from events, replays, and logs;
- colors, Unicode, resize, fullscreen programs, signals, and clean termination;
- `completed`, `failed`, and `disconnected` statuses;
- rejection of a five-minute certificate for a new connection after expiry;
- OpenSSH and Ansible operation through an unrecorded raw tunnel;
- valid Started dates for CLI, web, and tunnel entries;
- replay authorization, encryption, retention, and deletion audit;
- no regression of the web terminal or legacy agent protocol.

Technical validation includes Biome, frontend tests, agent/server/CLI Go tests, a production frontend build, and Linux agent builds for amd64, 386, arm64, and arm.

## 10. Current limitations and future work

Current limitations:

- only local accounts present in `/etc/passwd` and `/etc/shadow` are supported;
- PAM, LDAP/SSSD, and Linux MFA are unsupported;
- certificate expiry does not disconnect an established session;
- tunnel payload is audited but not replayable;
- S3 artifact storage is not implemented;
- startup reconciliation for sessions left `active` after an abrupt shutdown requires further hardening.

Potential follow-up work:

- optional PAM authentication;
- maximum session duration and a `disconnect_expired_cert` equivalent;
- S3 artifact storage;
- dedicated tunnel, file transfer, and port forwarding policies;
- automatic orphaned-session reconciliation.
