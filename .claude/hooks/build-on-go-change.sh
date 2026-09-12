#!/usr/bin/env bash
# PostToolUse hook (Write|Edit): runs `make build` after .go/go.mod/go.sum
# edits, per CLAUDE.md ("ビルドの自動フック" / .claude/rules/testing.md).
set -uo pipefail

input="$(cat)"
file="$(printf '%s' "$input" | jq -r '.tool_input.file_path // .tool_response.filePath // empty')"

case "$file" in
  *.go|*go.mod|*go.sum) ;;
  *) exit 0 ;;
esac

cd /workspaces/san-db-ox || exit 0

if ! out="$(make build 2>&1)"; then
  jq -n --arg reason "make build failed after editing $file:
$out" '{decision:"block", reason:$reason}'
fi
