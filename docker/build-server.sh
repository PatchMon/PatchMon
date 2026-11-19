#!/bin/sh
# Compile the embedded-frontend server inside the Docker builder stages.
set -eu

output_dir="$1"
architectures="$2"
version="${VERSION:-0.0.0}"
social_pkg="github.com/PatchMon/PatchMon/server-source-code/internal/social"
ldflags="-s -w -X github.com/PatchMon/PatchMon/server-source-code/internal/config.DefaultVersion=${version}"

for pair in "GitHubStars:${GITHUB_STARS:-}" "DiscordMembers:${DISCORD_MEMBERS:-}" \
            "YouTubeSubscribers:${YOUTUBE_SUBSCRIBERS:-}" "LinkedInFollowers:${LINKEDIN_FOLLOWERS:-}"; do
    name="${pair%%:*}"
    value="${pair#*:}"
    [ -n "$value" ] || continue
    case "$value" in
        *[!0-9]*) echo "Invalid community count for $name" >&2; exit 1 ;;
    esac
    ldflags="$ldflags -X $social_pkg.$name=$value"
done

mkdir -p "$output_dir"
for arch in $architectures; do
    case "$arch" in
        amd64|arm64|386) arm_version=6 ;;
        arm) arm_version="${TARGETVARIANT:-v6}"; arm_version="${arm_version#v}" ;;
        *) echo "Unsupported server architecture: $arch" >&2; exit 1 ;;
    esac
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOARM="$arm_version" \
        go build -buildvcs=false -ldflags="$ldflags" \
            -o "$output_dir/patchmon-server-linux-${arch}" ./cmd/server
done
