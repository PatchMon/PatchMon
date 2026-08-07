---
version: "0.1.2"
level: assist
processes:
  design: assist
  implementation: assist
  testing: assist
  documentation: pair
  review: hint
  deployment: hint
---

# AI Declaration

This file follows the [AI Declaration Standard v0.1.2](https://ai-declaration.md/en/0.1.2/).

## Notes

PatchMon is built with **transparency as a core value**.

AI assistance in coding and development is always a discussion that comes up when it comes to how it should and shouldn't be used in production environments.

The general rules are :

1. Human review
2. Double check the code line by line
3. Ensure no secrets are in areas where they shouldn't be
4. etc etc

### What we use AI for

Various AI models have contributed to PatchMon across the codebase and documentation:

- **Anthropic Claude** (Opus and Sonnet)
- **Cursor** (Compose 1.5 and 2.0)

Tooling used: Claude Code CLI, Cursor, other IDE / editor assistants.

### How AI is used

The declared level per process reflects how PatchMon is actually built:

- **Design - assist:** Architectural decisions are made by humans. AI is used to surface options, pressure-test trade-offs and sanity-check approaches, but the final call is always human.
- **Implementation - assist:** AI generates or accelerates code, tests and refactors. Every line that lands in `main` is read, understood and tested by a human before merge.
- **Testing - assist:** AI helps generate test cases and fixtures. Tests are executed and reviewed by humans.
- **Documentation - pair:** Docs, including this one, the README and parts of `docs/`, are often drafted collaboratively with AI and then edited by humans for accuracy, tone and UK English voice.
- **Deployment - hint:** CI/CD, release scripts and infrastructure changes are human-led. AI occasionally helps with syntax or config snippets.

### Human review is non-negotiable

Regardless of AI involvement, every change to PatchMon v2 goes through human review before it reaches production or the `main` branch:

- AI output is treated as a draft, not a commit.
- Humans verify factual claims (APIs, dependency names, documentation links) to guard against hallucination.
- Security-sensitive code (authentication, permissions, input validation, cryptography, secrets handling) receives additional human scrutiny.
- Tests must run and pass on a human's machine, not just in an AI's response.

### Expectations for contributors

If you use AI to help write a contribution, **we ask you to declare it**. The full disclosure template and contributor responsibilities are in [CONTRIBUTING.md - AI-Assisted Contributions](CONTRIBUTING.md#ai-assisted-contributions).

In short: AI-assisted contributions are welcome, disclosure is required, and you remain fully responsible for any code you submit.

### Why we publish this

Publishing an AI declaration is about trust. Users deploying PatchMon into production infrastructure deserve to know how the software they rely on is built. Auditors and procurement reviewers deserve honest answers. Contributors deserve a clear standard to follow.

If anything here changes - new tools, new workflows, different levels - we update this file and bump the declaration version accordingly.

---

*Questions about this declaration? Reach us at **support@patchmon.net** or on [Discord](https://patchmon.net/discord).*
