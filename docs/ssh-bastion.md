# SSH bastion and session recordings

PatchMon exposes two separate paths over the agent's existing outbound WebSocket:

- `patchmon ssh user@host` opens an interactive PTY terminated by PatchMon. The session is recorded.
- `patchmon tunnel host 22` relays raw TCP for OpenSSH, Ansible and SCP/SFTP. Tunnel payloads are never recorded or inspected.

Linux hosts do not expose TCP/22 to PatchMon. Enable the capability explicitly on each agent:

```yaml
integrations:
  ssh-bastion-enabled: true
```

Root is rejected by default. An administrator must add every permitted Linux account to the host's SSH account allow-list. Linux sudoers remains authoritative.

## Server setup

Run `docker/setup-env.sh` to generate:

- a stable bastion host key;
- an encrypted Ed25519 SSH CA and a separate passphrase secret;
- a 32-byte recording encryption key.

Then set `SSH_BASTION_ENABLED=true` in `.env`, publish TCP/2222 through the firewall or reverse TCP proxy, and restart the server. Session blocks are compressed, encrypted with AES-256-GCM, and kept in the `ssh_recordings` Docker volume. Metadata and indexes remain in PostgreSQL. Retention defaults to 90 days.

For CA rotation, place the new encrypted CA in the configured secret and retain old public CA lines in `SSH_PREVIOUS_CA_PUBLIC_KEYS` until all certificates issued by the old CA have expired.

Object storage is intentionally not part of this first version. The recording store is isolated behind its own package so an S3 backend can be added without changing the bastion protocol or database metadata.

## CLI

```console
patchmon login --server https://patchmon.example.com
patchmon instances list
patchmon ssh deploy@pve01
patchmon tunnel pve01 22
```

The SSH command creates an ephemeral local key, requests a five-minute certificate scoped to the PatchMon user, tenant, host and Linux account, shows the recording warning, then launches OpenSSH. PatchMon never stores a target host password or private key.

The tunnel command speaks raw SSH on stdin/stdout and is suitable as an OpenSSH `ProxyCommand`:

```sshconfig
Host patchmon-*
    ProxyCommand patchmon tunnel %h 22
```

Ansible can use the same `ProxyCommand` through `ansible_ssh_common_args`. Only tunnel metadata is audited; SSH, SCP/SFTP and Ansible content stays end-to-end encrypted.
