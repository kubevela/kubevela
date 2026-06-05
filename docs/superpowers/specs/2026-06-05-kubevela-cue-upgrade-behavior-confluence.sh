#!/usr/bin/env bash
# Publish the CUE upgrade behaviour page to Guidewire Confluence.
#
# Prerequisites:
#   1. Generate an Atlassian API token: https://id.atlassian.com/manage-profile/security/api-tokens
#   2. Export credentials in your shell BEFORE running this script:
#        export ATLASSIAN_USER='your.name@guidewire.com'
#        export ATLASSIAN_TOKEN='<your-api-token>'
#
# Usage:
#   bash docs/superpowers/specs/2026-06-05-kubevela-cue-upgrade-behavior-confluence.sh
#
# What it does:
#   - GET the current version number of page 3259924659
#   - Read the storage-format XML body from the .xml sibling file
#   - PUT a new revision with version+1

set -euo pipefail

: "${ATLASSIAN_USER:?Set ATLASSIAN_USER to your Atlassian email}"
: "${ATLASSIAN_TOKEN:?Set ATLASSIAN_TOKEN to your Atlassian API token}"

BASE_URL='https://guidewireconfluence.atlassian.net/wiki'
PAGE_ID='3259924659'
PAGE_TITLE='Upgrade Behaviour on existing application'
BODY_FILE="$(dirname "$0")/2026-06-05-kubevela-cue-upgrade-behavior-confluence.xml"

[[ -f "$BODY_FILE" ]] || { echo "Storage-format file not found: $BODY_FILE" >&2; exit 1; }

AUTH=$(printf '%s:%s' "$ATLASSIAN_USER" "$ATLASSIAN_TOKEN" | base64 -w0)

echo "Fetching current version of page $PAGE_ID ..."
CURRENT_VERSION=$(curl -fsSL \
  -H "Authorization: Basic $AUTH" \
  -H "Accept: application/json" \
  "$BASE_URL/rest/api/content/$PAGE_ID?expand=version" \
  | python3 -c 'import sys, json; print(json.load(sys.stdin)["version"]["number"])')

NEW_VERSION=$((CURRENT_VERSION + 1))
echo "Current version: $CURRENT_VERSION -> new version: $NEW_VERSION"

PAYLOAD=$(python3 - "$BODY_FILE" "$PAGE_TITLE" "$NEW_VERSION" <<'PY'
import json, sys
body_file, title, version = sys.argv[1], sys.argv[2], int(sys.argv[3])
with open(body_file) as f:
    body = f.read()
print(json.dumps({
    "version": {"number": version},
    "title":   title,
    "type":    "page",
    "body":    {"storage": {"value": body, "representation": "storage"}},
}))
PY
)

echo "Publishing ..."
curl -fsSL -X PUT \
  -H "Authorization: Basic $AUTH" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  "$BASE_URL/rest/api/content/$PAGE_ID" \
  | python3 -c '
import sys, json
r = json.load(sys.stdin)
print(f"  Title:   {r[\"title\"]}")
print(f"  Version: {r[\"version\"][\"number\"]}")
print(f"  URL:     {r[\"_links\"][\"base\"]}{r[\"_links\"][\"webui\"]}")
'

echo "Done."
