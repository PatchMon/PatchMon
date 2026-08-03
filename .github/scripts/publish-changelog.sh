#!/usr/bin/env bash
# =============================================================================
# Push a release's notes to the Quackback changelog at feedback.patchmon.net.
#
set -euo pipefail

API_URL="${QUACKBACK_API_URL:-https://feedback.patchmon.net/api/v1}"

die() { echo "::error::$*" >&2; exit 1; }

VERSION="${1:-}"
NOTES_FILE="${2:-}"

[ -n "$VERSION" ]    || die "usage: publish-changelog.sh <version> <notes-file>"
[ -n "$NOTES_FILE" ] || die "usage: publish-changelog.sh <version> <notes-file>"
[ -f "$NOTES_FILE" ] || die "notes file not found: $NOTES_FILE"
[ -n "${QUACKBACK_API_KEY:-}" ] || die "QUACKBACK_API_KEY is not set"

command -v jq >/dev/null 2>&1 || die "jq is required"

TITLE="Version ${VERSION}"

BODY_FILE="$(mktemp)"
PAYLOAD_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE" "$PAYLOAD_FILE"' EXIT

# Strip a leading "# ..." heading: the portal renders the title separately, so
# keeping it would show the version twice.
CONTENT="$(sed '1{/^# /d;}' "$NOTES_FILE")"
# grep, not ${CONTENT//[[:space:]]/}: bash pattern substitution over a
# multi-kilobyte release-notes file takes tens of seconds on bash 3.2.
printf '%s' "$CONTENT" | grep -q '[^[:space:]]' || die "notes file has no body: $NOTES_FILE"

# Writes the response body to $BODY_FILE and prints the HTTP status. Bodies go
# to a file rather than a shell variable so jq can stream them: changelog pages
# run to hundreds of KB and bash string handling on that is painfully slow.
api() {
  local method="$1" path="$2"
  shift 2
  curl --silent --show-error --location \
    --request "$method" \
    --header "Authorization: Bearer ${QUACKBACK_API_KEY}" \
    --header "Content-Type: application/json" \
    --connect-timeout 10 --max-time 60 --retry 2 --retry-connrefused \
    --output "$BODY_FILE" \
    --write-out '%{http_code}' \
    "${API_URL}${path}" "$@"
}

expect() {
  local status="$1" want="$2" context="$3"
  [ "$status" = "$want" ] || die "${context}: HTTP ${status} — $(head -c 500 "$BODY_FILE")"
}

echo "Looking for an existing changelog entry titled '${TITLE}'"
ENTRY_ID=""
CURSOR=""
while :; do
  query="?limit=100"
  if [ -n "$CURSOR" ]; then
    query="${query}&cursor=${CURSOR}"
  fi
  status="$(api GET "/changelog${query}")" || die "list changelog: request failed"
  expect "$status" 200 "list changelog"

  match="$(jq -r --arg t "$TITLE" 'first(.data[]? | select(.title == $t) | .id) // empty' "$BODY_FILE")"
  if [ -n "$match" ]; then
    ENTRY_ID="$match"
    break
  fi

  # The schema puts pagination at the top level; tolerate a meta-nested shape too.
  has_more="$(jq -r '(.pagination.hasMore // .meta.pagination.hasMore) // false' "$BODY_FILE")"
  [ "$has_more" = "true" ] || break
  CURSOR="$(jq -r '(.pagination.cursor // .meta.pagination.cursor) // empty' "$BODY_FILE")"
  [ -n "$CURSOR" ] || break
done

jq -n --arg title "$TITLE" --arg content "$CONTENT" \
  '{title: $title, content: $content}' > "$PAYLOAD_FILE"

if [ -n "$ENTRY_ID" ]; then
  echo "Updating existing entry ${ENTRY_ID}"
  status="$(api PATCH "/changelog/${ENTRY_ID}" --data "@${PAYLOAD_FILE}")" || die "update changelog: request failed"
  expect "$status" 200 "update changelog"
else
  echo "Creating a new draft entry"
  status="$(api POST "/changelog" --data "@${PAYLOAD_FILE}")" || die "create changelog: request failed"
  expect "$status" 201 "create changelog"
fi

ENTRY_ID="$(jq -r '.data.id' "$BODY_FILE")"
PUBLISHED="$(jq -r '.data.publishedAt // "draft"' "$BODY_FILE")"

echo "Changelog entry ${ENTRY_ID} (${PUBLISHED})"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Quackback changelog"
    echo ""
    echo "- Entry: \`${ENTRY_ID}\` (${PUBLISHED})"
    echo "- Review and publish it from the changelog section of the Quackback admin dashboard."
  } >> "$GITHUB_STEP_SUMMARY"
fi
