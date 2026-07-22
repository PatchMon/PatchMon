# PatchMon CLI

The CLI reuses PatchMon's existing authenticated host and web SSH APIs.

## Build

```bash
cd server-source-code
make build-cli
```

## Use

```bash
./patchmon login --server https://patchmon.example.com
./patchmon instances list
./patchmon instances list --output json
./patchmon ssh root@my-server
./patchmon ssh --identity ~/.ssh/id_ed25519 admin@my-server
```

For an HTTP development instance, pass `--insecure` to `login`. SSH sessions use the agent proxy exclusively, so `ssh-proxy-enabled: true` must be enabled in the target agent's `config.yml`.

The CLI stores the PatchMon access token in the user's configuration directory with mode `0600`. SSH passwords, private keys, and passphrases are held only for the duration of the connection.
