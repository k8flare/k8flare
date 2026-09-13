#!/bin/sh
# PostToolUse reminder: cost-sensitive APIs were touched. This never blocks the
# tool call -- the edit already happened -- it only resurfaces CLAUDE.md's cost
# invariants when a NEW occurrence of a cost-sensitive pattern shows up.
input=$(cat)
old=$(printf '%s' "$input" | jq -r '.tool_input.old_string // empty')
new=$(printf '%s' "$input" | jq -r '(.tool_input.new_string // .tool_input.content // empty)')
extra=$(printf '%s' "$input" | jq -r '[.tool_input.edits[]?.new_string // empty] | join("\n\n")' 2>/dev/null)
haystack="$new
$extra"

hits=""
for pat in 'setAlarm(' 'setInterval(' 'sleepAfter' 'onActivityExpired' '"cron"' 'scheduled(' 'waitUntil('; do
  case "$haystack" in
    *"$pat"*)
      case "$old" in
        *"$pat"*) ;; # already present before this edit -- not a new addition
        *) hits="$hits $pat" ;;
      esac
      ;;
  esac
done

[ -z "$hits" ] && exit 0

echo "COST CHECK: this edit introduces cost-sensitive API(s):$hits -- is it event-armed, and will it stay quiet on an idle cluster? Re-check CLAUDE.md's cost invariants and docs/cost-model.md." >&2
exit 2
