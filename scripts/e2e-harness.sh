#!/bin/zsh
# Local e2e harness lifecycle. See docs/development.md.
# Env: HARNESS_ASSETS (required), HARNESS_WORK, HARNESS_REPO,
#      HARNESS_SCHED, HARNESS_CM, CLOUDFLARE_ACCOUNT_ID.
set -u
mkdir -p "${HARNESS_WORK:-${TMPDIR:-/tmp}/k8flare-harness}"
# One place that starts and stops the local conformance harness.
# S62: sixteen stray controller-managers survived ad-hoc pkill patterns and
# made a replicas=2 workload look like it churned sixty pods. Every run goes
# through this script now, and stop asserts zero before returning.
S=${HARNESS_ASSETS:?set HARNESS_ASSETS to the directory holding e2e/ (kubeconfig.yaml, tls/, kubernetes/)}
SP=${HARNESS_WORK:-${TMPDIR:-/tmp}/k8flare-harness}
REPO=${HARNESS_REPO:-$(cd "$(dirname "$0")/.." && pwd)}
PIDS=$SP/harness.pids
SCHED_BIN=${HARNESS_SCHED:-$SP/sched-now}
CM_BIN=${HARNESS_CM:-$SP/cm-now}
# Match the binary NAME wherever it was built, not a directory one machine
# happened to use: the count assertion exists to catch host control-plane
# processes left over from an earlier experiment (S62), and a pattern anchored
# on scratchpad/ silently counts zero when the harness itself put its binaries
# somewhere else -- which reads as "REFUSING: expects 2, found 0" against
# processes that started perfectly.
STRAY='/(cm-now|cm-now2|deps-cm|deps-cm2|deps-sched|k8flare-controller-manager|sched-now|deps-scheduler|k8flare-scheduler)( |$)'

stop() {
  [ -f $PIDS ] && for p in $(cat $PIDS); do kill -9 $p 2>/dev/null; done
  rm -f $PIDS
  pkill -f 'wrangler.js dev' 2>/dev/null; pkill -f workerd 2>/dev/null
  for p in $(pgrep -f "$STRAY"); do kill -9 $p 2>/dev/null; done
  sleep 3
  n=$(pgrep -f "$STRAY" | wc -l | tr -d ' ')
  w=$(pgrep -f 'wrangler.js dev' | wc -l | tr -d ' ')
  echo "stop: strays=$n wrangler=$w"
  [ "$n" = "0" ] || { echo "REFUSING: $n stray control-plane processes survived"; exit 1; }
}

status() {
  echo "strays:   $(pgrep -f "$STRAY" | wc -l | tr -d ' ')"
  echo "wrangler: $(pgrep -f 'wrangler.js dev' | wc -l | tr -d ' ')"
  pgrep -fl "$STRAY" | head -5
}

start() { # $1 = variant: host | alldw ; $2 = extra --var args
  stop
  variant=${1:-host}
  rm -rf /tmp/e2e-state-harness
  vars=()
  case "$variant" in
    host)    vars=(--var SCHED_DISABLED:1 --var CM_DISABLED:1); want_sched=yes; want_cm=yes; expect=2 ;;
    kcmdw)   vars=(--var SCHED_DISABLED:1 --var CM_DISABLED:0); want_sched=yes; want_cm=no;  expect=1 ;;
    scheddw) vars=(--var SCHED_DISABLED:0 --var CM_DISABLED:1); want_sched=no;  want_cm=yes; expect=1 ;;
    alldw)   want_sched=no; want_cm=no; expect=0 ;;
    *) echo "unknown variant $variant"; exit 2 ;;
  esac
  [ -n "$2" ] && vars+=(${=2})
  cd $REPO
  nohup npx wrangler dev \
    -c packages/k8flare-worker/wrangler.jsonc --enable-containers=false "${vars[@]}" \
    --local --port 8443 --local-protocol https \
    --https-key-path $S/e2e/tls/dev.key --https-cert-path $S/e2e/tls/dev.crt \
    --persist-to /tmp/e2e-state-harness > $SP/harness-dev.log 2>&1 &
  echo $! >> $PIDS
  for i in $(seq 1 70); do curl -sk -o /dev/null https://127.0.0.1:8443/version 2>/dev/null && break; sleep 5; done
  rm -rf $SP/harness-cp; mkdir -p $SP/harness-cp
  if [ "$want_sched" = "yes" ]; then
    $SCHED_BIN --server=https://127.0.0.1:8443 --token=k8flare-dev-token --data-dir=$SP/harness-cp/s --insecure-skip-tls-verify > $SP/harness-sched.log 2>&1 &
    echo $! >> $PIDS
  fi
  if [ "$want_cm" = "yes" ]; then
    $CM_BIN --server=https://127.0.0.1:8443 --token=k8flare-dev-token --data-dir=$SP/harness-cp/c --insecure-skip-tls-verify > $SP/harness-cm.log 2>&1 &
    echo $! >> $PIDS
  fi
  sleep 25
  n=$(pgrep -f "$STRAY" | wc -l | tr -d ' ')
  [ "$n" = "$expect" ] || { echo "REFUSING: variant $variant expects $expect host processes, found $n"; exit 1; }
  docker restart k8flare-e2e-node > /dev/null
  for i in $(seq 1 60); do kubectl --kubeconfig=$S/e2e/kubeconfig.yaml get nodes --no-headers 2>/dev/null | grep -q ' Ready' && break; sleep 10; done
  echo "start: variant=$variant node=$(kubectl --kubeconfig=$S/e2e/kubeconfig.yaml get nodes --no-headers 2>/dev/null | head -1 | awk '{print $2}')"
}

case "$1" in
  start) start "${2:-host}" "${3:-}" ;;
  stop) stop ;;
  status) status ;;
  *) echo "usage: harness.sh start [host|alldw] [extra-vars] | stop | status"; exit 2 ;;
esac
