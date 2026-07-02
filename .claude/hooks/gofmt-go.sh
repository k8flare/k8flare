#!/bin/sh
# PostToolUse: keep Go files gofmt-clean after agent edits.
input=$(cat)
f=$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')

case "$f" in
  *.go) [ -f "$f" ] && gofmt -w "$f" ;;
esac

exit 0
