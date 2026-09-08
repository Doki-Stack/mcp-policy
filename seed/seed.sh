#!/usr/bin/env bash
# Indexes seed/policies.json into a running mcp-policy instance via
# POST /mcp/v1/tools/ingest-policy. Requires the server (and its Qdrant /
# Ollama / Postgres dependencies) to be up and reachable.
#
# Usage: MCP_POLICY_URL=http://localhost:8080 ./seed/seed.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MCP_POLICY_URL="${MCP_POLICY_URL:-http://localhost:8080}"

echo "Seeding policies into ${MCP_POLICY_URL}/mcp/v1/tools/ingest-policy ..."

response="$(curl -sS -X POST \
  -H "Content-Type: application/json" \
  --data-binary "@${SCRIPT_DIR}/policies.json" \
  "${MCP_POLICY_URL}/mcp/v1/tools/ingest-policy")"

echo "${response}"

if command -v jq >/dev/null 2>&1; then
  failures="$(echo "${response}" | jq '[.results[] | select(.success == false)] | length')"
  total="$(echo "${response}" | jq '.results | length')"
  echo "Ingested ${total} policies, ${failures} failed."
  if [ "${failures}" != "0" ]; then
    exit 1
  fi
else
  echo "(install jq for a pass/fail summary; raw response printed above)"
fi
