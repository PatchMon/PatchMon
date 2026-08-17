#!/usr/bin/env bash
# =============================================================================
# Point the vcheck TXT records at a released version.
#
# CLOUDFLARE_API_TOKEN and CLOUDFLARE_ZONE_ID required.
# VCHECK_RECORDS overrides the default record list.
#
set -euo pipefail

API="https://api.cloudflare.com/client/v4"
VERSION="${1:-}"
RECORDS="${VCHECK_RECORDS:-server.vcheck.patchmon.net agent.vcheck.patchmon.net}"

die() { echo "::error::$*" >&2; exit 1; }

[ -n "$VERSION" ] || die "usage: update-vcheck-dns.sh <version>"
printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' \
  || die "version must be N.N.N with no pre-release suffix, got: ${VERSION}"
[ -n "${CLOUDFLARE_API_TOKEN:-}" ] || die "CLOUDFLARE_API_TOKEN is not set"
[ -n "${CLOUDFLARE_ZONE_ID:-}" ]   || die "CLOUDFLARE_ZONE_ID is not set"
command -v jq >/dev/null 2>&1 || die "jq is required"

BODY_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE"' EXIT

api() {
  local method="$1" path="$2"
  shift 2
  curl --silent --show-error \
    --request "$method" \
    --header "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" \
    --header "Content-Type: application/json" \
    --connect-timeout 10 --max-time 30 --retry 2 --retry-connrefused \
    --output "$BODY_FILE" \
    --write-out '%{http_code}' \
    "${API}${path}" "$@"
}

expect_ok() {
  local status="$1" context="$2"
  [ "$status" = "200" ] || die "${context}: HTTP ${status} — $(jq -c '.errors // .' "$BODY_FILE" 2>/dev/null | head -c 300)"
  [ "$(jq -r '.success' "$BODY_FILE")" = "true" ] || die "${context}: $(jq -c '.errors' "$BODY_FILE" | head -c 300)"
}

CHANGED=0
SUMMARY=""

for name in $RECORDS; do
  status="$(api GET "/zones/${CLOUDFLARE_ZONE_ID}/dns_records?type=TXT&name=${name}")" || die "${name}: lookup request failed"
  expect_ok "$status" "${name}: lookup"

  count="$(jq -r '.result | length' "$BODY_FILE")"
  [ "$count" = "1" ] || die "${name}: expected exactly 1 TXT record, found ${count}"

  id="$(jq -r '.result[0].id' "$BODY_FILE")"
  current="$(jq -r '.result[0].content' "$BODY_FILE" | tr -d '"')"

  if [ "$current" = "$VERSION" ]; then
    echo "${name} already at ${VERSION}"
    SUMMARY="${SUMMARY}- \`${name}\` already at ${VERSION}"$'\n'
    continue
  fi

  # sort -V, so a rerun of an older release cannot walk the fleet backwards.
  if [ -n "$current" ] && [ "$(printf '%s\n%s\n' "$current" "$VERSION" | sort -V | tail -1)" != "$VERSION" ]; then
    die "${name} holds ${current}, refusing to move it back to ${VERSION}"
  fi

  status="$(api PATCH "/zones/${CLOUDFLARE_ZONE_ID}/dns_records/${id}" \
    --data "$(jq -n --arg c "$VERSION" '{content: $c}')")" || die "${name}: update request failed"
  expect_ok "$status" "${name}: update"

  echo "${name}: ${current} -> ${VERSION}"
  SUMMARY="${SUMMARY}- \`${name}\`: ${current} → **${VERSION}**"$'\n'
  CHANGED=$((CHANGED + 1))
done

echo "${CHANGED} record(s) updated"

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Version check DNS"
    echo ""
    printf '%s' "$SUMMARY"
    echo ""
    echo "Instances pick this up on their next daily \`version-update-check\`."
  } >> "$GITHUB_STEP_SUMMARY"
fi
