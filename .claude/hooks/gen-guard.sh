#!/bin/sh
# PreToolUse guard: generated/build files are never hand-edited.
# Exit 2 blocks the tool call; stderr is fed back to Claude.
input=$(cat)
f=$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')
[ -z "$f" ] && exit 0

case "$f" in
  */build/*|*.gen.go|*.gen.ts|*.gen.json|*_generated_*.go|*/gen/*)
    echo "BLOCKED: $f is generated or build output. Edit the source of truth instead (the Go source for build:wasm output; cmd/k8flare-gen for gen/ files) and regenerate: 'npm run build:wasm' / 'go generate ./...'." >&2
    exit 2
    ;;
esac

if [ -f "$f" ] && head -3 "$f" 2>/dev/null | grep -q "Code generated .* DO NOT EDIT"; then
  echo "BLOCKED: $f has a 'Code generated ... DO NOT EDIT' header. Modify the generator (cmd/k8flare-gen) and re-run 'go generate ./...' instead." >&2
  exit 2
fi

exit 0
