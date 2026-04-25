#!/usr/bin/env python3
"""Rebuild the consolidated `patchmon-release-notes.md` book from the
per-version release-note files that ship inside the server source tree:

    server-source-code/internal/handler/release_notes_data/RELEASE_NOTES_*.md

Versions are emitted newest first. Output goes to:

    docs/patchmon-release-notes.md

The other three books (admin, operator, api-integrations) are now edited
directly in `docs/` — no consolidation step is involved. This script no
longer touches them.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
DOCS_OUT = REPO / "docs"
RELEASE_NOTES_SRC = (
    REPO / "server-source-code" / "internal" / "handler" / "release_notes_data"
)

_HEADING_RE = re.compile(r"^(#{1,6})( +.*)$")
_VERSION_RE = re.compile(r"RELEASE_NOTES_(\d+)\.(\d+)\.(\d+)\.md")


def demote_headings(text: str) -> str:
    out_lines: list[str] = []
    in_fence = False
    fence_marker: str | None = None
    for line in text.split("\n"):
        stripped = line.lstrip()
        if not in_fence:
            if stripped.startswith("```") or stripped.startswith("~~~"):
                in_fence = True
                fence_marker = stripped[:3]
                out_lines.append(line)
                continue
        else:
            if fence_marker is not None and stripped.startswith(fence_marker):
                in_fence = False
                fence_marker = None
            out_lines.append(line)
            continue

        m = _HEADING_RE.match(line)
        if m:
            hashes, rest = m.group(1), m.group(2)
            level = len(hashes)
            extra = 2 if level == 1 else 1
            new_level = min(level + extra, 6)
            out_lines.append("#" * new_level + rest)
        else:
            out_lines.append(line)
    return "\n".join(out_lines)


def build_release_notes() -> str:
    if not RELEASE_NOTES_SRC.is_dir():
        raise FileNotFoundError(
            f"Release notes directory not found: {RELEASE_NOTES_SRC}"
        )
    files: list[tuple[tuple[int, int, int], Path]] = []
    for entry in RELEASE_NOTES_SRC.iterdir():
        m = _VERSION_RE.match(entry.name)
        if not m:
            continue
        files.append(
            ((int(m.group(1)), int(m.group(2)), int(m.group(3))), entry)
        )
    if not files:
        raise FileNotFoundError(
            f"No RELEASE_NOTES_*.md found under {RELEASE_NOTES_SRC}"
        )
    files.sort(key=lambda t: t[0], reverse=True)

    parts: list[str] = []
    for version, path in files:
        version_str = ".".join(str(v) for v in version)
        slug = f"v{version_str.replace('.', '-')}"
        raw = path.read_text()
        lines = raw.split("\n")
        while lines and lines[0].strip() == "":
            lines.pop(0)
        if lines and lines[0].startswith("# "):
            lines = lines[1:]
        body = "\n".join(lines).rstrip()
        demoted = demote_headings(body)
        parts.append(
            f'## Version {version_str} {{#{slug}}}\n\n{demoted}'
        )

    toc_lines = ["## Table of Contents", ""]
    for version, _ in files:
        version_str = ".".join(str(v) for v in version)
        slug = f"v{version_str.replace('.', '-')}"
        toc_lines.append(f"- [Version {version_str}](#{slug})")
    toc = "\n".join(toc_lines)

    front_matter = (
        "---\n"
        'title: "PatchMon Release Notes"\n'
        'description: "Version-by-version PatchMon release notes — features, fixes, breaking changes, and migration pointers."\n'
        "---\n\n"
    )
    intro = (
        "Each section below documents what changed in a PatchMon release. "
        "Versions are listed newest first. The same source files are also "
        "served by the application in the admin UI under Release Notes."
    )
    body = "\n\n---\n\n".join(parts)
    return f"{front_matter}# PatchMon Release Notes\n\n{intro}\n\n{toc}\n\n---\n\n{body}\n"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--out",
        type=Path,
        default=DOCS_OUT,
        help=f"Output directory for the consolidated release-notes book (default: {DOCS_OUT})",
    )
    args = parser.parse_args()

    args.out.mkdir(parents=True, exist_ok=True)

    try:
        content = build_release_notes()
    except FileNotFoundError as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)

    out_path = args.out / "patchmon-release-notes.md"
    out_path.write_text(content)
    print(f"Wrote {out_path.relative_to(REPO)}  {out_path.stat().st_size:,} bytes")


if __name__ == "__main__":
    main()
