#!/usr/bin/env bash
# Runs cmd/prodprobe against the real deployment, on a schedule launchd owns.
#
# This replaces .github/workflows/prod-probe.yml, which stopped running when
# GitHub Actions was switched off on cost grounds (2026-09-12). The probe is
# the only check in this repo that observes a REAL deployment; S30 stood
# undetected for six weeks with every local gate green, so leaving it
# unscheduled is not a neutral choice.
#
# What it gives up compared with the workflow: it runs only while this
# machine is awake. launchd fires a missed StartCalendarInterval job on wake,
# so a laptop that sleeps nightly still gets a run per day, late. A machine
# that stays off for a week reports nothing for a week -- and reports it
# silently, which is the same failure shape the probe exists to catch. Check
# the last line of the log when you care.
#
# Install:
#   cp scripts/com.k8flare.prodprobe.plist ~/Library/LaunchAgents/
#   # edit the plist's WorkingDirectory to this checkout, then
#   launchctl load ~/Library/LaunchAgents/com.k8flare.prodprobe.plist
#
# Credentials come from a file this script reads and never writes:
#   ~/.config/k8flare/probe.env   (mode 600)
#     K8FLARE_PROBE_URL=https://...
#     K8FLARE_PROBE_TOKEN=...
#     CLOUDFLARE_ACCOUNT_ID=...       # optional, enables the parking check
#     CLOUDFLARE_API_TOKEN=...        # optional, Account Analytics:Read
set -euo pipefail

CONFIG="${K8FLARE_PROBE_CONFIG:-$HOME/.config/k8flare/probe.env}"
LOGDIR="${K8FLARE_PROBE_LOGDIR:-$HOME/Library/Logs/k8flare}"
mkdir -p "$LOGDIR"
LOG="$LOGDIR/prodprobe-$(date -u +%Y%m%dT%H%M%SZ).log"
LATEST="$LOGDIR/prodprobe-latest.log"

notify() {
  osascript -e "display notification \"$1\" with title \"k8flare production probe\"" 2>/dev/null || true
}

if [ ! -r "$CONFIG" ]; then
  echo "no credentials at $CONFIG -- see the header of $0" | tee "$LOG"
  ln -sf "$LOG" "$LATEST"
  notify "not configured: $CONFIG missing"
  exit 78
fi

set -a
# shellcheck disable=SC1090
. "$CONFIG"
set +a

if [ -z "${K8FLARE_PROBE_URL:-}" ] || [ -z "${K8FLARE_PROBE_TOKEN:-}" ]; then
  echo "K8FLARE_PROBE_URL and K8FLARE_PROBE_TOKEN are required in $CONFIG" | tee "$LOG"
  ln -sf "$LOG" "$LATEST"
  notify "not configured: URL or token missing"
  exit 78
fi

# The parking half needs account analytics; the convergence half does not.
# Degrade rather than skip -- S30's symptom (a control plane that serves the
# first list and then ignores a scale for minutes) is caught by convergence
# alone, and skipping the whole run for a missing optional credential would
# have left it undetected for the same six weeks.
PARKING=true
if [ -z "${CLOUDFLARE_ACCOUNT_ID:-}" ] || [ -z "${CLOUDFLARE_API_TOKEN:-}" ]; then
  PARKING=false
fi

status=0
{
  echo "k8flare production probe, started $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "url=$K8FLARE_PROBE_URL parking=$PARKING"
  echo
} > "$LOG"
go run ./cmd/prodprobe -parking="$PARKING" -compute "" >> "$LOG" 2>&1 || status=$?

ln -sf "$LOG" "$LATEST"
if [ "$status" -ne 0 ]; then
  echo "FAILED (exit $status)" >> "$LOG"
  tail -n 40 "$LOG" >> "$LOGDIR/prodprobe-failures.log"
  notify "FAILED -- see $LATEST"
else
  echo "OK" >> "$LOG"
fi

# Keep a month; the logs are small and the history is the only record of
# whether the deployment was observed at all on a given day.
find "$LOGDIR" -name 'prodprobe-2*.log' -mtime +31 -delete 2>/dev/null || true
exit "$status"
