#!/usr/bin/env bash
# PreToolUse(Bash) hook: deny commands that invoke the Tiger CLI, so the agent
# uses the Tiger MCP tools instead of shelling out. Matches `tiger`,
# `./bin/tiger`/`bin/tiger`, and `go run ./cmd/tiger` (incl. a leading `VAR=val`),
# but not path mentions like `go build -o bin/tiger ./cmd/tiger`.
set -euo pipefail

cmd=$(jq -r '.tool_input.command // ""')

# Match a tiger invocation at a command boundary (start or after a separator),
# allowing leading `VAR=val ` env assignments.
boundary='(^|[;&|(]|&&|\|\|)[[:space:]]*([A-Za-z_][A-Za-z0-9_]*=[^[:space:]]+[[:space:]]+)*'
invocation="${boundary}((\./)?(bin/)?tiger([[:space:]]|\$)|go[[:space:]]+run[[:space:]]+[^;&|]*cmd/tiger)"

if [[ $cmd =~ $invocation ]]; then
  cat <<'JSON'
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Running the Tiger CLI from the agent is not allowed. Use the Tiger MCP tools instead of shelling out to the CLI."}}
JSON
fi

exit 0
