#!/usr/bin/env bash
set -uo pipefail

cd "$(git rev-parse --show-toplevel)"
OUT=.build/ci/repro
mkdir -p "$OUT"
ROUNDS=${ROUNDS:-15}
STATE=/dev/shm/k8flare-state

printf 'ADMIN_TOKEN=%s\nREADONLY_TOKEN=%s\nJOIN_TOKEN=%s\nKUBELET_SCHEME=http\nKUBELET_PORT=10255\n' \
  "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > .dev.vars
admin=$(sed -n 's/^ADMIN_TOKEN=//p' .dev.vars)
make wrangler.dev.jsonc >/dev/null

workerd_pids() {
  ss -ltnpH | grep '"workerd"' | grep -o 'pid=[0-9]*' | cut -d= -f2 | sort -u
}

stop_all() {
  pkill -f '[w]rangler dev' 2>/dev/null
  for pid in $(workerd_pids); do kill -9 "$pid" 2>/dev/null; done
  sleep 2
}

main_ms() {
  local pid=$1 a b
  a=$(sed 's/.*) //' "/proc/$pid/task/$pid/stat" | awk '{ print $12 + $13 }')
  sleep 2
  b=$(sed 's/.*) //' "/proc/$pid/task/$pid/stat" | awk '{ print $12 + $13 }')
  echo $(((b - a) * 1000 / $(getconf CLK_TCK) / 2))
}

capture() {
  local round=$1 dir="$OUT/hang-$round"
  mkdir -p "$dir"
  for pid in $(workerd_pids); do
    echo "== pid $pid main_ms=$(main_ms "$pid")" >> "$dir/cpu.txt"
    sudo gdb -p "$pid" -batch -ex 'thread apply all bt 40' > "$dir/gdb-$pid.txt" 2>&1
  done
  local bases=(http://127.0.0.1:9229)
  for pid in $(workerd_pids); do
    for port in $(ss -ltnpH | grep "pid=$pid," | awk '{ n = split($4, a, ":"); print a[n] }'); do
      bases+=("http://127.0.0.1:$port")
    done
  done
  timeout 120 node scripts/ci/pause.mjs "${bases[@]}" > "$dir/pause.txt" 2>&1
  cp "$OUT/dev-$round.log" "$dir/"
}

for round in $(seq 1 "$ROUNDS"); do
  stop_all
  rm -rf "$STATE"
  nohup pnpm exec wrangler dev -c wrangler.dev.jsonc --local --enable-containers=false --persist-to "$STATE" --port 18787 \
    < /dev/null > "$OUT/dev-$round.log" 2>&1 &
  up=0
  for _ in $(seq 1 60); do
    if curl -sf -m 5 -o /dev/null -H "Authorization: Bearer $admin" http://127.0.0.1:18787/livez; then up=1; break; fi
    sleep 2
  done
  if [ "$up" = 0 ]; then
    echo "round $round: never became live"
    capture "$round"
    exit 1
  fi
  requests=()
  for path in /api /apis /api/v1/namespaces /api/v1/nodes /apis/apps/v1/deployments; do
    curl -s -m 20 -o /dev/null -H "Authorization: Bearer $admin" "http://127.0.0.1:18787$path" &
    requests+=($!)
  done
  wait "${requests[@]}"
  sleep 15
  if ! curl -sf -m 20 -o /dev/null -H "Authorization: Bearer $admin" http://127.0.0.1:18787/api/v1/namespaces; then
    echo "round $round: stopped answering"
    capture "$round"
    exit 1
  fi
  echo "round $round: fine"
done
stop_all
