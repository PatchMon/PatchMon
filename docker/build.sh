#!/usr/bin/env bash
# =============================================================================
# Build the PatchMon server Docker image locally.
#
# The git tag is the single source of truth for the version. Nothing in the
# repo declares one, so the version has to be passed into the image build.
# This script works it out for you and passes it through as a build arg.
#
# .dockerignore excludes .git, so the Dockerfile cannot derive the version
# itself. That is why this wrapper exists rather than a plain `docker build`.
#
# Usage:
#   ./docker/build.sh                      # version from the nearest git tag
#   ./docker/build.sh --version 2.0.1      # pin a version (useful for testing
#                                          # the auto-update path)
#   ./docker/build.sh --tag mytag          # image tag (default: local)
#   ./docker/build.sh --skip-agents        # reuse whatever is in agents-prebuilt/
#
# Equivalent by hand:
#   docker build -f docker/server.Dockerfile \
#     --build-arg VERSION="$(git describe --tags --abbrev=0 | sed 's/^v//')" \
#     -t patchmon-server:local .
# =============================================================================

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION=""
IMAGE_TAG="local"
SKIP_AGENTS="false"

die() { echo "error: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --tag)     IMAGE_TAG="${2:-}"; shift 2 ;;
    --skip-agents) SKIP_AGENTS="true"; shift ;;
    -h|--help) sed -n '2,25p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

# `--abbrev=0` gives the bare nearest tag (v2.0.2), not v2.0.2-60-gABC. The
# server parses versions as dot-separated integers and silently treats any
# suffix as 0, so a describe with a suffix would report 2.0.0 and make the
# instance think an update is available.
if [ -z "$VERSION" ]; then
  VERSION="$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || true)"
fi

if [ -z "$VERSION" ]; then
  die "could not determine a version. Either this clone has no tags, or it is
  shallow so there is no history for 'git describe' to walk back through.
  Fetching tags alone does not fix a shallow clone. Run:
      git fetch --unshallow --tags
  or pass a version explicitly:
      ./docker/build.sh --version 2.0.3"
fi

if ! printf '%s' "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  die "version '$VERSION' is not MAJOR.MINOR.PATCH. Anything else is parsed as 0."
fi

echo "==> Version: $VERSION"

# The server image bundles agent binaries from agents-prebuilt/ (gitignored).
if [ "$SKIP_AGENTS" = "true" ]; then
  echo "==> Skipping agent build"
else
  echo "==> Building agent binaries"
  make -C agent-source-code build-linux build-freebsd build-windows VERSION="$VERSION"
fi

mkdir -p agents-prebuilt
shopt -s nullglob
agent_bins=(agent-source-code/build/patchmon-agent-*)
if [ ${#agent_bins[@]} -eq 0 ]; then
  die "agents-prebuilt/ has nothing to copy and no agents were built.
  Run without --skip-agents, or build them with:
      make -C agent-source-code build-linux build-freebsd build-windows"
fi
cp -f "${agent_bins[@]}" agents-prebuilt/
echo "==> Staged ${#agent_bins[@]} agent binaries"

echo "==> Building frontend for embed"
npm ci --workspace=frontend --include=dev
npm run build --workspace=frontend
mkdir -p server-source-code/cmd/server/static/frontend
cp -r frontend/dist server-source-code/cmd/server/static/frontend/

echo "==> Building image patchmon-server:${IMAGE_TAG}"
docker build \
  -f docker/server.Dockerfile \
  --build-arg VERSION="$VERSION" \
  -t "patchmon-server:${IMAGE_TAG}" \
  .

echo ""
echo "Built patchmon-server:${IMAGE_TAG} reporting version ${VERSION}"
