#!/usr/bin/env bash
# Runs cmd/prodprobe against the real deployment, on a schedule launchd owns,
# and installs/inspects that schedule.
#
#   prodprobe-local.sh run         one probe run (what launchd invokes)
#   prodprobe-local.sh install     render the plist for THIS checkout, load it
#   prodprobe-local.sh status      what launchd actually holds, and how stale
#   prodprobe-local.sh uninstall   unload and remove it
#
# Use the Makefile entry points: make probe-install / probe-status /
# probe-uninstall.
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
# silently, which is the same failure shape the probe exists to catch. That is
# what `status` is for: it goes non-zero on a job that is not loaded, not
# scheduled, has never run, failed, or has not run in two days.
#
# Credentials come from a file this script reads and never writes:
#   ~/.config/k8flare/probe.env   (mode 600)
#     K8FLARE_PROBE_URL=https://...
#     K8FLARE_PROBE_TOKEN=...
#     CLOUDFLARE_ACCOUNT_ID=...       # optional, enables the parking check
#     CLOUDFLARE_API_TOKEN=...        # optional, Account Analytics:Read
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LABEL="${K8FLARE_PROBE_LABEL:-com.k8flare.prodprobe}"
CONFIG="${K8FLARE_PROBE_CONFIG:-$HOME/.config/k8flare/probe.env}"
LOGDIR="${K8FLARE_PROBE_LOGDIR:-$HOME/Library/Logs/k8flare}"
TEMPLATE="$REPO/scripts/com.k8flare.prodprobe.plist"
INSTALLED="$HOME/Library/LaunchAgents/$LABEL.plist"
RENDERED="${TMPDIR:-/tmp}/$LABEL.rendered.plist"
DOMAIN="gui/$(id -u)"

notify() {
  osascript -e "display notification \"$1\" with title \"k8flare production probe\"" 2>/dev/null || true
}

config_instructions() {
  cat <<INSTR
Create it, with the deployment you want observed. The heredoc terminator has to
stay in column 1, so this block is flush left on purpose:

mkdir -p $(dirname "$CONFIG")
cat > $CONFIG <<'ENV'
K8FLARE_PROBE_URL=https://your-deployment.workers.dev
K8FLARE_PROBE_TOKEN=<a cluster token: the /clusters bootstrap kubeconfig, or the K3S_TOKEN secret's value for the default cluster>
CLOUDFLARE_ACCOUNT_ID=<account id>   # optional -- omit and only convergence is checked
CLOUDFLARE_API_TOKEN=<token>         # optional -- needs Account Analytics:Read, nothing else
ENV
chmod 600 $CONFIG

Point it at a canary deployment, not at one carrying user workloads: the
parking half asserts the whole Worker script is idle.
INSTR
}

probe_run() {
  mkdir -p "$LOGDIR"
  local LOG LATEST
  LOG="$LOGDIR/prodprobe-$(date -u +%Y%m%dT%H%M%SZ).log"
  LATEST="$LOGDIR/prodprobe-latest.log"

  if [ ! -r "$CONFIG" ]; then
    echo "no credentials at $CONFIG -- run 'make probe-install' for the exact contents" | tee "$LOG"
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

  # launchd gives a job PATH=/usr/bin:/bin:/usr/sbin:/sbin and no login shell,
  # so a toolchain installed under $HOME (mise, homebrew) is invisible unless
  # the plist carries it. `install` bakes go's directory in; a plist copied by
  # hand does not, and this is the failure that shape produces.
  if ! command -v go >/dev/null 2>&1; then
    echo "go is not on PATH ($PATH) -- reinstall the agent with 'make probe-install', which bakes go's directory into the plist" | tee "$LOG"
    ln -sf "$LOG" "$LATEST"
    notify "not configured: go not on PATH"
    exit 78
  fi

  # The parking half needs account analytics; the convergence half does not.
  # Degrade rather than skip -- S30's symptom (a control plane that serves the
  # first list and then ignores a scale for minutes) is caught by convergence
  # alone, and skipping the whole run for a missing optional credential would
  # have left it undetected for the same six weeks.
  local PARKING=true
  if [ -z "${CLOUDFLARE_ACCOUNT_ID:-}" ] || [ -z "${CLOUDFLARE_API_TOKEN:-}" ]; then
    PARKING=false
  fi

  local status=0
  {
    echo "k8flare production probe, started $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "url=$K8FLARE_PROBE_URL parking=$PARKING"
    echo
  } > "$LOG"
  (cd "$REPO" && go run ./cmd/prodprobe -parking="$PARKING" -compute "") >> "$LOG" 2>&1 || status=$?

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
}

BAKED_PATH=""

render_plist() {
  local gobin
  gobin="$(command -v go || true)"
  if [ -z "$gobin" ]; then
    echo "go is not on PATH here either -- install it (mise use go@…) before installing the agent" >&2
    exit 1
  fi
  BAKED_PATH="$(dirname "$gobin"):/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
  local bakedpath="$BAKED_PATH"

  sed -e "s|__K8FLARE_CHECKOUT__|$REPO|g" \
      -e "s|__K8FLARE_PATH__|$bakedpath|g" \
      -e "s|__K8FLARE_LOGDIR__|$LOGDIR|g" \
      "$TEMPLATE" > "$RENDERED"
  plutil -lint "$RENDERED" >/dev/null
  echo "rendered:  $RENDERED"
  echo "  checkout $REPO"
  echo "  PATH     $bakedpath"
  echo "  logs     $LOGDIR"
}

probe_install() {
  render_plist

  # A job that cannot build is a job that fails silently at 02:23. go.mod's
  # replace directives point into .build/*-mirror, so a wiped .build/ breaks
  # every run; catch it here rather than in a log nobody reads.
  echo
  # Under the environment launchd will impose, not this shell's: an rc file's
  # GOFLAGS/GOTOOLCHAIN/GOPROXY is exactly the kind of thing that makes a build
  # work here and fail at 02:23.
  echo "preflight: go build ./cmd/prodprobe, under the plist's own PATH"
  if ! (cd "$REPO" && env -i HOME="$HOME" PATH="$BAKED_PATH" go build -o /dev/null ./cmd/prodprobe); then
    echo "cannot build the probe from this checkout -- if the error mentions .build/*-mirror, run 'make gen-mirrors' first" >&2
    exit 1
  fi

  if [ ! -r "$CONFIG" ]; then
    echo
    echo "NOT INSTALLED: no credentials at $CONFIG."
    echo
    config_instructions
    echo
    echo "Then run 'make probe-install' again. The plist named above was generated"
    echo "but not installed, and launchd has not been touched."
    exit 78
  fi

  mkdir -p "$HOME/Library/LaunchAgents" "$LOGDIR"
  cp "$RENDERED" "$INSTALLED"
  # bootout first so a reinstall replaces rather than collides; enable undoes a
  # previous `launchctl unload -w`, which otherwise makes bootstrap fail 119.
  launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
  launchctl enable "$DOMAIN/$LABEL" 2>/dev/null || true
  launchctl bootstrap "$DOMAIN" "$INSTALLED"

  echo
  echo "installed: $INSTALLED"
  echo "It will not run until the next scheduled time. To force one now (~25min):"
  echo "  launchctl kickstart -p $DOMAIN/$LABEL"
  echo
  # Not a gate: a freshly loaded job has never run, which `status` reports as a
  # problem and is not one yet. Run `make probe-status` tomorrow.
  probe_status || true
}

probe_uninstall() {
  launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
  rm -f "$INSTALLED"
  if launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then
    echo "launchd still holds $LABEL -- 'launchctl bootout $DOMAIN/$LABEL' by hand" >&2
    exit 1
  fi
  echo "removed:   $LABEL is gone from launchd and $INSTALLED is deleted."
  echo "Logs in $LOGDIR are left alone."
}

probe_status() {
  local out rc=0
  out="$(launchctl print "$DOMAIN/$LABEL" 2>/dev/null || true)"

  echo "label:     $LABEL"

  if [ -z "$out" ]; then
    if [ -e "$INSTALLED" ]; then
      echo "launchd:   PRESENT BUT NOT LOADED -- $INSTALLED exists, launchd does not know it"
    else
      echo "launchd:   NOT INSTALLED -- no $INSTALLED"
    fi
    echo "           nothing is observing production. Run: make probe-install"
    rc=1
  else
    echo "launchd:   loaded from $(print_field "$out" 'path')"
    echo "           state=$(print_field "$out" 'state') runs=$(print_field "$out" 'runs')"
    # The original defect was a wrong path inside a plist launchd was happy
    # with, so show the command launchd actually holds, not the one this
    # checkout would install: an agent left over from a moved or second
    # checkout is otherwise indistinguishable from a correct one.
    local args
    args="$(printf '%s\n' "$out" | awk '/^\targuments = \{$/{f=1;next} f&&/^\t\}$/{exit} f{gsub(/^[ \t]+/,""); printf "%s ", $0}')"
    echo "           command: ${args:-$(print_field "$out" 'program')}"

    if printf '%s\n' "$out" | grep -q 'com.apple.launchd.calendarinterval'; then
      local when
      when="$(printf '%s\n' "$out" | grep -E '"(Hour|Minute)" =>' | sed -e 's/[^"]*"//' -e 's/" => /=/' | tr '\n' ' ')"
      echo "scheduled: yes -- launchd holds a calendar trigger ($when)"
    else
      echo "scheduled: NO -- loaded, but launchd holds no calendar trigger; it will never fire on its own"
      rc=1
    fi

    local code
    code="$(print_field "$out" 'last exit code')"
    case "$code" in
      '(never exited)'|'')
        echo "last run:  NEVER -- loaded, but launchd has not run it yet"
        rc=1 ;;
      0)  echo "last run:  PASSED (exit 0)" ;;
      78) echo "last run:  NOT CONFIGURED (exit 78) -- the run never reached the deployment"
          rc=1 ;;
      *)  echo "last run:  FAILED (exit $code)"
          rc=1 ;;
    esac
  fi

  local newest
  newest="$(find "$LOGDIR" -maxdepth 1 -name 'prodprobe-2*.log' -exec stat -f '%m %N' {} + 2>/dev/null | sort -rn | head -n 1 || true)"
  if [ -z "$newest" ]; then
    echo "logs:      NONE in $LOGDIR -- no run has ever produced a log here"
    rc=1
  else
    local hours
    hours=$(( ( $(date +%s) - ${newest%% *} ) / 3600 ))
    newest="${newest#* }"
    local verdict="${hours}h old"
    # Daily job plus a laptop that sleeps: 48h is the first age that cannot be
    # explained by a late wake-up, so it is the first age worth shouting about.
    if [ "$hours" -ge 48 ]; then
      verdict="STALE, ${hours}h old ($(( hours / 24 )) days)"
      rc=1
    fi
    echo "logs:      $verdict -- $(basename "$newest")"
    # launchd's exit code and the log's own last line can disagree: the code is
    # from the last run launchd reaped, the log is from the last run that
    # started. Believe whichever is unhappy.
    local tail_line
    tail_line="$(tail -n 1 "$newest")"
    case "$tail_line" in
      OK)      echo "           the log agrees: OK" ;;
      FAILED*) echo "           the log says: $tail_line"
               rc=1 ;;
      *)       echo "           the log is INCOMPLETE -- last line is '$tail_line' (still running, or cut off mid-run)"
               rc=1 ;;
    esac
  fi

  if [ -r "$CONFIG" ]; then
    echo "config:    present at $CONFIG"
  else
    echo "config:    MISSING at $CONFIG -- every run will exit 78"
    rc=1
  fi

  if [ "$rc" -eq 0 ]; then
    echo "VERDICT:   scheduled and observing production."
  else
    echo "VERDICT:   NOT observing production -- see the lines above."
  fi
  return "$rc"
}

# launchctl print indents its own top-level fields with exactly one tab; nested
# blocks (event triggers) reuse names like `state`, so anchoring on the tab is
# what keeps `state = active` out of the answer.
print_field() {
  printf '%s\n' "$1" | awk -v key="$2" 'index($0, "\t" key " = ") == 1 { print substr($0, length(key) + 5); exit }'
}

case "${1:-run}" in
  run)       probe_run ;;
  install)   probe_install ;;
  status)    probe_status ;;
  uninstall) probe_uninstall ;;
  *) echo "usage: $(basename "$0") [run|install|status|uninstall]" >&2; exit 2 ;;
esac
