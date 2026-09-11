#!/usr/bin/env bash
# Smoke test of the probe and of /readyz against a local `wrangler dev`.
# This says nothing about production: dev does not reproduce the runtime
# semantics S30 broke. It only proves this tooling works before a
# maintainer points it at a real deployment.
set -euo pipefail

cd "$(dirname "$0")/../.."
ROOT="$PWD"
MANIFEST="packages/k8flare-worker/assets/wasm/apiserver.manifest.json"
TOKEN="k8flare-dev-token"

if [ ! -f "$MANIFEST" ]; then
  echo "$MANIFEST is missing -- run 'make wasm' first" >&2
  exit 1
fi

PORT="${PORT:-}"
if [ -z "$PORT" ]; then
  for candidate in $(seq 8799 8830); do
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$candidate") 2>/dev/null; then
      PORT="$candidate"
      break
    fi
  done
fi
if [ -z "$PORT" ]; then
  echo "no free port in 8799-8830" >&2
  exit 1
fi

WORK="$(mktemp -d)"
LOG="$WORK/wrangler.log"
DEV_PID=""

cleanup() {
  if [ -f "$WORK/apiserver.manifest.json" ]; then
    mv -f "$WORK/apiserver.manifest.json" "$ROOT/$MANIFEST"
  fi
  if [ -n "$DEV_PID" ]; then
    kill "$DEV_PID" 2>/dev/null || true
    wait "$DEV_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

echo "== starting wrangler dev on 127.0.0.1:$PORT (state in $WORK)"
npx wrangler dev -c packages/k8flare-worker/wrangler.jsonc --local \
  --enable-containers=false --port "$PORT" --persist-to "$WORK/state" \
  --log-level info >"$LOG" 2>&1 &
DEV_PID=$!

for _ in $(seq 1 90); do
  if curl -sf -o /dev/null "http://127.0.0.1:$PORT/healthz"; then break; fi
  sleep 1
done
if ! curl -sf -o /dev/null "http://127.0.0.1:$PORT/healthz"; then
  echo "wrangler dev never came up" >&2
  cat "$LOG" >&2
  exit 1
fi

fail() {
  echo "SMOKE FAIL: $*" >&2
  cat "$LOG" >&2
  exit 1
}

echo "== /readyz reports every component on a healthy deployment"
body="$(curl -s -w '\n%{http_code}' -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/readyz")"
code="$(printf '%s' "$body" | tail -n1)"
printf '%s\n' "$body" | sed '$d'
[ "$code" = "200" ] || fail "/readyz returned $code on a healthy deployment"
printf '%s' "$body" | grep -q '^\[+\]apiserver ok' || fail "/readyz did not report the apiserver check"
printf '%s' "$body" | grep -q '^\[+\]storage ok' || fail "/readyz did not report the storage check"

echo "== /readyz refuses an unauthenticated caller"
code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/readyz")"
[ "$code" = "401" ] || fail "unauthenticated /readyz returned $code, want 401"

echo "== the probe itself, against dev (convergence phases only)"
K8FLARE_PROBE_SMOKE=1 \
  K8FLARE_PROBE_URL="http://127.0.0.1:$PORT" \
  K8FLARE_PROBE_TOKEN="$TOKEN" \
  go test ./cmd/prodprobe/ -run TestSmokeAgainstDev -count=1 -timeout 15m -v || fail "probe smoke test failed"

echo "== with the Loader path broken, liveness stays cheap and readiness goes 503"
mv "$ROOT/$MANIFEST" "$WORK/apiserver.manifest.json"
sleep 5
live="$(curl -s -o /dev/null -w '%{http_code} %{time_total}s' "http://127.0.0.1:$PORT/healthz")"
echo "/healthz: $live"
case "$live" in 200\ *) ;; *) fail "/healthz returned '$live' with the Loader path broken" ;; esac
body="$(curl -s -w '\n%{http_code}' -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/readyz")"
code="$(printf '%s' "$body" | tail -n1)"
printf '%s\n' "$body" | sed '$d'
[ "$code" = "503" ] || fail "/readyz returned $code with the Loader path broken, want 503"
printf '%s' "$body" | grep -q '^\[-\]apiserver failed' || fail "/readyz did not name the apiserver check as failed"
mv -f "$WORK/apiserver.manifest.json" "$ROOT/$MANIFEST"

echo "SMOKE PASS"
