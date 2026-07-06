#!/usr/bin/env bash
# Drive the Openmind MCP endpoint over raw JSON-RPC (Streamable HTTP) to verify
# initialize + tools/list + a few tools/call against a running instance.
#
#   API=http://localhost:8787 OPENMIND_TOKEN=devtoken apps/api/scripts/mcp-e2e.sh
#
# Streamable HTTP requires an Accept header advertising both JSON and SSE, and a
# session: initialize returns an `Mcp-Session-Id` response header that every
# subsequent request must echo back. This script captures it and reuses it.
# Note: no `set -u` — an empty bash-3.2 array expansion (macOS) trips it.
set -eo pipefail
API="${API:-http://localhost:8787}"
TOK="${OPENMIND_TOKEN:-devtoken}"
ACCEPT="application/json, text/event-stream"

post() { # $1 = json body ; echoes response body, sends session header if known
  local extra=()
  if [[ -n "${SESSION:-}" ]]; then extra=(-H "Mcp-Session-Id: $SESSION"); fi
  curl -sS -D /tmp/mcp-h.txt \
    -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
    -H "Accept: $ACCEPT" "${extra[@]}" \
    -X POST "$API/mcp" -d "$1"
}

echo "== initialize =="
INIT=$(post '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}')
echo "$INIT"
SESSION=$(grep -i '^Mcp-Session-Id:' /tmp/mcp-h.txt | tr -d '\r' | awk '{print $2}' || true)
echo "session: ${SESSION:-<none>}"
# The SDK requires a `notifications/initialized` after initialize before it will
# service other requests on the session.
post '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' >/dev/null || true

echo "== tools/list =="
post '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
echo

echo "== tools/call save_item =="
post '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_item","arguments":{"url":"https://danluu.com/why-benchmark/"}}}'
echo

echo "== tools/call list_recent =="
post '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_recent","arguments":{"limit":5}}}'
echo

echo "== tools/call search_items =="
post '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_items","arguments":{"query":"benchmark"}}}'
echo

echo "== tools/call list_lenses =="
post '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_lenses","arguments":{}}}'
echo
