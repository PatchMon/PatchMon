# PatchMon Documentation

This directory is the **source of truth** for the public-facing documentation
published at <https://patchmon.net/docs>. The website pulls these files from
the `main` branch of `PatchMon/PatchMon` at build time, so changes here go
live with the next website deploy.

## What's here

Four single-file books, kept monolithic on purpose so each one is easy to
read top-to-bottom and ships as a downloadable PDF on the website:

| File | Book |
| --- | --- |
| `patchmon-admin-guide.md` | Day-to-day usage for admins/operators in the web UI |
| `patchmon-operator-guide.md` | Install, configure, OIDC SSO, agent management, troubleshooting |
| `patchmon-api-integrations-guide.md` | Discord, gethomepage, Ansible, Proxmox auto-enrollment, REST APIs |
| `patchmon-release-notes.md` | Version-by-version release notes (auto-built from `server-source-code/internal/handler/release_notes_data/`) |
| `assets/` | Images referenced by the books |

## Editing workflow

1. Edit the relevant `.md` file directly in this directory.
2. Drop any new images into `assets/` and reference them as
   `![alt](assets/your-image.png)`. The website's sync script will copy them
   into `public/docs/assets/` and rewrite the URLs.
3. If you paste an image directly into the GitHub web editor, GitHub uploads
   it to `https://github.com/user-attachments/assets/<uuid>`. That's fine —
   the website's sync script downloads those at build time, hashes them,
   and saves them under `public/docs/assets/_gh/`. The GitHub-hosted URL
   keeps working in `main`'s rendered Markdown view too.
4. Commit + push. The next website deploy picks up the change.

## Frontmatter

Each book declares `title` and `description`. The website lib parses these
and uses them on the `/docs` landing page and per-book `<meta>` tags. Don't
remove them.

## TOC anchors

Top-level chapter headings (`## Chapter N: Title`) get auto-generated
anchor IDs by the website renderer (via `rehype-slug`). The book's own
table of contents at the top links to those anchors. Adding a new chapter?
Update the in-document TOC list to match — there's no separate manifest.

## Release notes

`patchmon-release-notes.md` is **regenerated** from
`server-source-code/internal/handler/release_notes_data/RELEASE_NOTES_*.md`
by `tools/consolidate-docs.py`. To publish a new version:

1. Add `RELEASE_NOTES_<version>.md` to that directory.
2. Run `python3 tools/consolidate-docs.py` from the repo root. This
   rebuilds `patchmon-release-notes.md` from the release-note sources
   (newest version first). It does not touch the other three books;
   those are edited directly.

Alternatively, if you just want to add a manual release-notes entry without
running the script, edit `patchmon-release-notes.md` by hand. The website
doesn't care which path produced the file.

## Why monolithic?

Each book renders as a single page on the website with a left chapter
sidebar — a "book on a page" experience — and downloads as one PDF. Small
per-page files would mean a fragmented browsing experience and many tiny
PDFs. The trade-off is bigger PR diffs; we accept that.
