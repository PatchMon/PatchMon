# Security Policy

PatchMon manages patching for other people's infrastructure. A vulnerability here
can affect every host a user monitors, so we take reports seriously and ask that
they are handled privately until a fix is available.

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately using GitHub's private vulnerability reporting:

👉 **https://github.com/PatchMon/PatchMon/security/advisories/new**

If you cannot use that, email **security@patchmon.net**.

### What to include

- The version of PatchMon affected, and the installation method (Docker, Proxmox script, `setup.sh`)
- Which component is affected — server, agent, or web interface
- Steps to reproduce, or a proof of concept
- The impact as you understand it
- Whether the issue is already public anywhere

You do not need a complete exploit. A clear description of the weakness is enough
to start.

## What happens next

| Stage | Target |
|---|---|
| Acknowledgement of your report | Within 3 working days |
| Initial assessment and severity | Within 7 working days |
| Fix released, or a plan with dates | Within 30 days for high severity |

We are a small team with a large community. If a report is going to take longer
than the above, we will tell you rather than go quiet.

## Disclosure

We follow coordinated disclosure. We will:

1. Confirm the issue and agree a timeline with you
2. Prepare and test a fix
3. Release the fix and publish a security advisory
4. Credit you in the advisory, unless you would rather stay anonymous

Please give us a reasonable window to release a fix before disclosing publicly.
Because self-hosters have to apply updates themselves, early public disclosure
leaves real installations exposed.

## Scope

**In scope:** the PatchMon server, the agent, the web interface, the official
Docker images, and the official install scripts in this repository.

**Out of scope:** vulnerabilities in third-party dependencies that have already
been publicly disclosed and are awaiting an upstream fix (please still tell us,
but they are not treated as new findings); issues that require an already
compromised host or existing administrative access; social engineering; and
findings against `patchmon.net` marketing pages that have no security impact.

## Supported versions

Security fixes are released against the **current stable release line** only.
If you are running an older version, the fix will be to upgrade. We do not
backport security patches to unsupported releases.
