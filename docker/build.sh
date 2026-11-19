#!/usr/bin/env bash
# =============================================================================
# Build the PatchMon server Docker image locally.
#
# The git tag is the single source of truth for the version. Nothing in the
# repo declares one, so the version has to be passed into the image build.
# This script works it out for you and passes it through as a build arg.
#
# Docker builds the frontend, server and bundled agents in builder containers.
# Only Docker and Bash are required locally; Git is used to resolve the version
# when --version is omitted.
#
# Usage:
#   ./docker/build.sh                      # version from the nearest git tag
#   ./docker/build.sh --version 2.0.1      # pin a version (useful for testing
#                                          # the auto-update path)
#   ./docker/build.sh --tag mytag          # image tag (default: local)
#   ./docker/build.sh --platform linux/arm64 # cross-build for another platform
#   ./docker/build.sh --skip-agents        # reuse all 10 agents in agents-prebuilt/
# =============================================================================

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION=""
IMAGE_TAG="local"
SKIP_AGENTS="false"
PLATFORM=""

die() { echo "error: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version|--tag|--platform)
      [ $# -ge 2 ] && [ -n "$2" ] && [[ "$2" != --* ]] || die "$1 requires a value"
      case "$1" in
        --version) VERSION="$2" ;;
        --tag) IMAGE_TAG="$2" ;;
        --platform) PLATFORM="$2" ;;
      esac
      shift 2
      ;;
    --skip-agents) SKIP_AGENTS="true"; shift ;;
    -h|--help) sed -n '2,25p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

command -v docker >/dev/null 2>&1 || die "Docker is required to build the image"

# `--abbrev=0` gives the bare nearest tag (v2.0.2), not v2.0.2-60-gABC. A
# suffix is read as a pre-release, so v2.0.2-60-gABC sorts just below 2.0.2 and
# the instance would believe it is behind the current release, offering an
# update that can never satisfy it.
#
# Local builds always produce a bare release version. The only deliberate
# pre-release is the one CI mints for edge images, in
# .github/actions/release-context.
if [ -z "$VERSION" ]; then
  command -v git >/dev/null 2>&1 || die "Git is required to resolve the version; install Git or pass --version"
  VERSION="$(git describe --tags --abbrev=0 2>/dev/null || true)"
  VERSION="${VERSION#v}"
fi

if [ -z "$VERSION" ]; then
  die "could not determine a version. Either this clone has no tags, or it is
  shallow so there is no history for 'git describe' to walk back through.
  Fetching tags alone does not fix a shallow clone. Run:
      git fetch --unshallow --tags
  or pass a version explicitly:
      ./docker/build.sh --version 2.0.3"
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "version '$VERSION' is not MAJOR.MINOR.PATCH. Local builds must pass a bare
  release version; edge pre-releases like 2.0.3-rc.61 are minted by CI only."
fi

echo "==> Version: $VERSION"

# Normal builds compile every bundled agent in Docker. This opt-in compatibility
# path reuses explicitly staged binaries, as the release image pipeline does.
build_args=(--build-arg "VERSION=$VERSION")
if [ "$SKIP_AGENTS" = "true" ]; then
  for target in linux/amd64 linux/arm64 linux/386 linux/arm \
                freebsd/amd64 freebsd/arm64 freebsd/386 freebsd/arm \
                windows/amd64 windows/arm64; do
    suffix=""; case "$target" in windows/*) suffix=".exe" ;; esac
    binary="agents-prebuilt/patchmon-agent-${target/\//-}${suffix}"
    [ -s "$binary" ] || die "--skip-agents requires $binary; omit the flag to build agents in Docker"
  done
  build_args+=(--build-arg AGENT_SOURCE=agents-prebuilt)
  echo "==> Reusing agents-prebuilt/"
fi

if [ -n "$PLATFORM" ]; then
  build_args+=(--platform "$PLATFORM")
fi

echo "==> Building image patchmon-server:${IMAGE_TAG}"
docker build \
  -f docker/server.Dockerfile \
  -t "patchmon-server:${IMAGE_TAG}" \
  "${build_args[@]}" \
  .

echo ""
echo "Built patchmon-server:${IMAGE_TAG} reporting version ${VERSION}"
