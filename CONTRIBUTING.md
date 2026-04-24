# Contributing to PatchMon

Thanks for your interest in contributing - we welcome bug reports, feature ideas, documentation improvements and pull requests from the community.

This guide covers everything you need to know to get set up, make a change and get it merged.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Ways to Contribute](#ways-to-contribute)
- [Getting Started](#getting-started)
- [Repository Layout](#repository-layout)
- [Development Environment](#development-environment)
- [Running Tests & Linters](#running-tests--linters)
- [Commit Conventions](#commit-conventions)
- [AI-Assisted Contributions](#ai-assisted-contributions)
- [Pull Request Process](#pull-request-process)
- [Documentation](#documentation)
- [Security Issues](#security-issues)
- [Getting Help](#getting-help)

---

## Code of Conduct

Be respectful, constructive and patient. We want PatchMon to be a welcoming project for contributors of every background and experience level.

- Assume good intent.
- Give feedback on the work, not the person.
- Keep discussions focused and civil on GitHub, Discord and email.
- Harassment, discrimination or personal attacks will not be tolerated. Report concerns privately to **support@patchmon.net**.

---

## Ways to Contribute

You don't have to write code to contribute. Valued contributions include:

- **Bug reports** - open an issue with reproduction steps, expected vs actual behaviour, and environment details.
- **Feature requests** - describe the problem first, then the proposed solution. Discussions welcome on [Discord](https://patchmon.net/discord) before opening an issue.
- **Documentation** - fix typos, clarify steps, add missing detail.
- **Code** - bug fixes, new features, refactors, test coverage.
- **Community support** - answer questions on Discord, help new users.

### Good first issues

Looking for a starting point? Check issues labelled [`good first issue`](https://github.com/PatchMon/PatchMon/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) or [`help wanted`](https://github.com/PatchMon/PatchMon/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22).

---

## Getting Started

1. **Fork** the repository at [github.com/PatchMon/PatchMon](https://github.com/PatchMon/PatchMon).
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/<your-username>/PatchMon.git
   cd PatchMon
   ```
3. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/short-description
   ```
4. Make your changes, test them locally, and open a pull request.

---

## Repository Layout

| Path | Purpose |
|------|---------|
| `server-source-code/` | Go server (API + embedded frontend + migrations) |
| `agent-source-code/` | Go agent (Linux, FreeBSD, Windows) |
| `frontend/` | React + Vite frontend, bundled into the server binary |
| `docker/` | Dockerfiles and compose files |
| `agents/` | Prebuilt agent binaries and install/remove scripts |
| `tools/` | Developer tooling |

---

## Development Environment

### Prerequisites

- **Go** 1.26+
- **Node.js** 22+
- **PostgreSQL** 17
- **Redis** 7
- **Docker** & **Docker Compose** (recommended for running the stack)
- **Make**

### Running the stack

The fastest way to spin up a full dev environment:

```bash
# From repo root
docker compose -f docker/docker-compose.dev.yml up
```

This starts Postgres, Redis and the server with live reload on Go source changes.

### Server-only

```bash
cd server-source-code
make run            # Build and run
make run-pprof      # Run with ENABLE_PPROF=true
```

### Frontend-only

```bash
cd frontend
npm install
npm run dev         # Dev server on port 3000, proxies /api to :3001
```

### Agent-only

```bash
cd agent-source-code
make build          # Build native binary
make build-all      # Build all platforms
```

---

## Running Tests & Linters

**All PRs must pass lint and tests before review.**

### Go (server & agent)

```bash
# From server-source-code/ or agent-source-code/
make check          # fmt-check + vet + golangci-lint + tests
make test           # Tests only
make test-coverage  # Tests + HTML coverage report
```

`make check` is the single command CI runs. If it passes locally, your PR will pass the Go checks.

### Frontend

```bash
cd frontend
npx biome check --write src/    # Auto-fix style + lint
npx biome check src/            # Verify clean
npm run test:run                # Run Vitest suite
```

**All frontend PRs must pass `npx biome check src/` with zero errors and zero warnings** before being submitted.

### Database schema changes

If you change the database schema:

1. Edit `server-source-code/internal/sqlc/schema/schema.sql`.
2. Add or modify queries in `internal/sqlc/queries/*.sql`.
3. Run `make sqlc-generate` from `server-source-code/`.
4. Add a migration in `internal/migrate/migrations/` with both `.up.sql` and `.down.sql` files. **Check the directory for the latest migration number before choosing yours** - duplicate numbers crash startup.
5. Run `make check`.

Never edit files in `internal/db/` - they are generated by sqlc.

---

## Commit Conventions

We use [Conventional Commits](https://www.conventionalcommits.org/). Every commit message should start with a type:

| Type | When to Use |
|------|-------------|
| `feat:` | New user-facing feature |
| `fix:` | Bug fix |
| `docs:` | Documentation only |
| `style:` | Formatting, no code change |
| `refactor:` | Code change that is neither a fix nor a feature |
| `perf:` | Performance improvement |
| `test:` | Adding or updating tests |
| `chore:` | Build, tooling, dependencies |
| `ci:` | CI / pipeline changes |

**Examples:**

```
feat(patching): add dry-run retry for offline hosts
fix(auth): prevent session fixation on login
docs(readme): clarify Docker quick start
refactor(store): move host queries into HostsStore
```

Scope is optional but helpful (`feat(patching)`, `fix(agent)`, `docs(api)`, etc.).

### Signed commits

We do not require GPG-signed commits, but they are welcome.

---

## AI-Assisted Contributions

AI-assisted contributions are welcome. Tools like Claude Code, Cursor, GitHub Copilot, Windsurf, ChatGPT and others can meaningfully speed up development - but reviewers need context to evaluate the change, and we need to know a human has actually read and understood it.

### Disclosure is required

**If AI was used in any meaningful way to produce the contribution, disclose it in the pull request description.** "Meaningful" includes: generating code, tests, documentation, commit messages, or PR descriptions; agentic / multi-step workflows; or AI-driven refactoring. Using an editor's autocomplete or a spell-checker does not need disclosure.

Include the following block in your PR description:

```markdown
## AI Disclosure

- **Used AI:** yes / no
- **Model(s):** e.g. Claude Opus 4.7, GPT-5, Gemini 2.5 Pro
- **Harness / tool:** e.g. Claude Code CLI, Cursor, Copilot, Windsurf, ChatGPT web, n8n workflow
- **Scope:** what AI produced (code / tests / docs / commit messages / full PR / partial)
- **Human review completed:** yes / no - who reviewed and what was checked
```

If you did not use AI, a single line is enough:

```markdown
## AI Disclosure

No AI used.
```

### Contributor responsibilities

You remain fully responsible for any code you submit, AI-assisted or not. That specifically means:

- **You have read every line** of the change, not just the diff summary.
- **You understand what it does** and can answer review questions without re-prompting the AI.
- **You have run the tests and linters locally** (`make check`, `npx biome check src/`). AI-generated code that has not been executed is not acceptable.
- **You have verified factual claims** - invented APIs, hallucinated package names, non-existent functions, fabricated documentation links, and typosquatted dependencies are common AI failure modes. Check every dependency name against its official source before adding it.
- **Security-sensitive code** (auth, permissions, input validation, crypto, secrets handling) gets extra human scrutiny. Flag any AI involvement clearly so maintainers can review accordingly.
- **Licences and attribution** are your responsibility. Don't submit code an AI may have reproduced verbatim from a GPL-incompatible source.

### What we do with the disclosure

AI disclosure is used for review context, not as a judgement. Maintainers may ask additional questions, request extra tests, or apply tighter scrutiny to security-sensitive AI-generated changes. Undisclosed AI use that is discovered during review will delay the PR and may be closed without merge.

---

## Pull Request Process

1. **One change per PR.** Keep PRs focused and reviewable. Big refactors can be split into multiple PRs.
2. **Write a clear PR description.** Explain:
   - What problem the PR solves.
   - How it solves it.
   - How you tested it.
   - Any follow-up work or known limitations.
3. **Link related issues.** Use `Closes #123` or `Fixes #456` so the issue auto-closes on merge.
4. **Keep the branch up to date** with `main`. Rebase or merge as you prefer.
5. **Respond to review feedback** promptly. Push fixes as new commits; we'll squash on merge.
6. **CI must pass.** Lint, tests and build checks all have to be green.

### PR size guidance

- < 400 lines changed: easy to review, fast merge.
- 400-1000 lines: OK if cohesive, but consider splitting.
- > 1000 lines: please discuss with maintainers on Discord first.

### What maintainers look for

- Correctness - does it do what it says, without regressions?
- Tests - is the new code covered?
- Security - any auth, permission, input-validation or secret-handling concerns?
- Consistency - does it follow the patterns already in the codebase?
- Documentation - are user-facing changes reflected in the docs?

---

## Documentation

User-facing changes must include documentation updates in the same PR.

- **README.md** - high-level project overview, quick start, deployment options.
- **docs.patchmon.net** - full user documentation, hosted on BookStack. Sign in with GitHub; request editor access via Discord or email once you've verified.
- **Internal docs** - in `docs/Internal documentation/` (architecture, database, OIDC, WebSocket protocol, testing, etc.).

If you're adding or changing:

- An environment variable → update the env vars reference page on docs.patchmon.net.
- An API endpoint → update the integration/auto-enrolment API docs.
- An agent config option → update the agent config reference.
- The OIDC flow → update the OIDC setup guide.

---

## Security Issues

**Do not open a public GitHub issue for security vulnerabilities.**

Report security issues privately to **support@patchmon.net** with:

- A description of the issue.
- Steps to reproduce or a proof-of-concept.
- The affected version or commit.
- Your disclosure timeline preferences.

We will acknowledge within 2 business days and work with you on a coordinated disclosure.

---

## Getting Help

- **Discord:** [patchmon.net/discord](https://patchmon.net/discord) - fastest way to reach the team and community.
- **GitHub Discussions:** for longer-form questions.
- **Email:** support@patchmon.net - for professional, enterprise or security enquiries.

---

Thanks for contributing to PatchMon.
