#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
WORK=.build/ci
LOGS=$WORK/logs
API=127.0.0.1:16443
K3S_VERSION=${K3S_VERSION:-v1.36.2+k3s1}
KUBECONFIG_PATH=$PWD/$WORK/kubeconfig.yaml
STATE=${STATE:-/dev/shm/k8flare-state}
mkdir -p "$LOGS"

stamped() {
  perl -MPOSIX=strftime -MTime::HiRes=time -ne 'BEGIN { $| = 1 } my $t = time; printf "%s.%03d %s", strftime("%H:%M:%S", gmtime($t)), ($t - int($t)) * 1000, $_' > "$1"
}

wait_for() {
  local what=$1 tries=$2 delay=$3
  shift 3
  for _ in $(seq 1 "$tries"); do
    if "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  echo "timed out waiting for $what" >&2
  return 1
}

build() {
  pnpm install --frozen-lockfile
  make mirrors
  make wasm
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$WORK/k8flare-agent" ./packages/agent
  CGO_ENABLED=0 go build -o "$WORK/k8flare" ./packages/cli
  (cd scripts && CGO_ENABLED=0 go build -o "../$WORK/devtls" ./devtls)
}

build_shard() {
  local shard=$1 shards=$2
  local -a all mine=()
  read -ra all <<<"$(make -s opt-wasm-list)"
  for i in "${!all[@]}"; do
    if [ $((i % shards)) -eq "$shard" ]; then
      mine+=("${all[$i]}")
    fi
  done
  echo "shard $shard/$shards builds ${mine[*]}"
  make mirrors
  make "${mine[@]}"
}

dev_vars() {
  [ -f .dev.vars ] && return 0
  printf 'ADMIN_TOKEN=%s\nREADONLY_TOKEN=%s\nJOIN_TOKEN=%s\nSECRETS_ENCRYPTION_KEYS=ci:%s\nKUBELET_SCHEME=http\nKUBELET_PORT=10255\n' \
    "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -base64 32)" > .dev.vars
  chmod 600 .dev.vars
}

node_ready() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" get nodes --no-headers | awk '$2=="Ready"' | grep -q .
}

write_accepted() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" create namespace ci-writecheck &&
    kubectl --kubeconfig "$KUBECONFIG_PATH" delete namespace ci-writecheck --wait=false
}

user_worker_port() {
  local admin=$1 port
  for port in $(ss -ltnpH | awk '/"workerd"/ { n = split($4, a, ":"); print a[n] }' | sort -u); do
    [ "$port" = 18787 ] && continue
    if curl -sf -m 5 -o /dev/null -H "Authorization: Bearer $admin" "http://127.0.0.1:$port/livez"; then
      echo "$port"
      return 0
    fi
  done
  echo "no workerd port answers /livez" >&2
  return 1
}

thread_ms() {
  local tick=$1
  shift
  cat "$@" 2>/dev/null | sed 's/.*) //' | awk -v t="$tick" '{ s += $12 + $13 } END { print int(s * 1000 / t) }'
}

sample_procs() {
  local tick pid main other prev_main prev_other stat
  tick=$(getconf CLK_TCK)
  declare -A last_main last_other
  while true; do
    for pid in "$@"; do
      [ -r "/proc/$pid/stat" ] || continue
      stat=$(sed 's/.*) //' "/proc/$pid/stat")
      main=$(thread_ms "$tick" "/proc/$pid/task/$pid/stat")
      other=$(( $(thread_ms "$tick" /proc/"$pid"/task/*/stat) - main ))
      prev_main=${last_main[$pid]:-$main}
      prev_other=${last_other[$pid]:-$other}
      last_main[$pid]=$main
      last_other[$pid]=$other
      echo "procs pid=$pid comm=$(cat "/proc/$pid/comm") main_ms=$((main - prev_main)) other_ms=$((other - prev_other)) threads=$(ls "/proc/$pid/task" | wc -l) rss_kb=$(awk '/VmRSS/ { print $2 }' "/proc/$pid/status") state=$(cut -d' ' -f1 <<<"$stat") wchan=$(cat "/proc/$pid/wchan" 2>/dev/null)"
    done
    sleep 1
  done
}

join_node() {
  local admin=$1 token join
  if [ "${AGENT:-k3s}" = k8flare ]; then
    join=$(sed -n 's/^JOIN_TOKEN=//p' .dev.vars)
    sudo install -m 0644 .build/devtls/server-ca.crt /usr/local/share/ca-certificates/k8flare-dev-ca.crt
    sudo update-ca-certificates >/dev/null
    sudo nohup "$WORK/k8flare-agent" --server "https://$API" --token "$join" --node-name "$(hostname)" \
      > "$LOGS/agent.log" 2>&1 < /dev/null &
    return 0
  fi
  token=$("$WORK/k8flare" token create --kubeconfig "$KUBECONFIG_PATH" --ttl 1h --description ci)
  sudo nohup /usr/local/bin/k3s agent --server "https://$API" --token "$token" --node-name "$(hostname)" --disable-apiserver-lb \
    > "$LOGS/agent.log" 2>&1 < /dev/null &
}

up() {
  if [ ! -x /usr/local/bin/k3s ]; then
    curl -sfL -o "$WORK/k3s" "https://github.com/k3s-io/k3s/releases/download/${K3S_VERSION/+/%2B}/k3s"
    sudo install -m 0755 "$WORK/k3s" /usr/local/bin/k3s
  fi
  sudo /usr/local/bin/k3s ctr version >/dev/null 2>&1 || true
  sudo test -x /var/lib/rancher/k3s/data/current/bin/containerd

  dev_vars
  make wrangler.dev.jsonc
  local admin worker
  admin=$(sed -n 's/^ADMIN_TOKEN=//p' .dev.vars)
  nohup pnpm exec wrangler dev -c wrangler.dev.jsonc --local --enable-containers=false --persist-to "$STATE" --port 18787 \
    < /dev/null 2>&1 | stamped "$LOGS/dev.log" &
  wait_for "the control plane" 360 5 curl -sf -o /dev/null -H "Authorization: Bearer $admin" http://127.0.0.1:18787/livez
  worker=$(user_worker_port "$admin")
  echo "user worker listens on 127.0.0.1:$worker"
  local runtime
  runtime=$(ss -ltnpH "sport = :$worker" | grep -o 'pid=[0-9]*' | head -1 | cut -d= -f2)
  nohup bash -c "$(declare -f thread_ms sample_procs); sample_procs $runtime $(ps -o ppid= -p "$runtime")" \
    < /dev/null 2>&1 | stamped "$LOGS/procs.log" &
  nohup "$WORK/devtls" -listen "$API" -upstream "http://127.0.0.1:$worker" -dir .build/devtls -hosts localhost -admin-token "$admin" \
    < /dev/null 2>&1 | stamped "$LOGS/devtls.log" &
  wait_for "devtls" 180 1 curl -sf --cacert .build/devtls/server-ca.crt -o /dev/null -H "Authorization: Bearer $admin" "https://$API/livez"

  printf 'apiVersion: v1\nkind: Config\nclusters:\n- name: k8flare-ci\n  cluster:\n    server: https://%s\n    certificate-authority: %s/.build/devtls/server-ca.crt\nusers:\n- name: admin\n  user:\n    token: %s\ncontexts:\n- name: k8flare-ci\n  context:\n    cluster: k8flare-ci\n    user: admin\ncurrent-context: k8flare-ci\n' \
    "$API" "$PWD" "$admin" > "$KUBECONFIG_PATH"
  chmod 600 "$KUBECONFIG_PATH"

  join_node "$admin"
  wait_for "a Ready node" 120 5 node_ready
  wait_for "the API to accept a write" 60 5 write_accepted
  kubectl --kubeconfig "$KUBECONFIG_PATH" get nodes -o wide
}

run_e2e() {
  (cd scripts && go run ./e2e -set "${SET:-required}" -procs "${PROCS:-4}" -shard "${SHARD:-0}" -shards "${SHARDS:-1}" -kubeconfig "$KUBECONFIG_PATH")
}

case "${1:-all}" in
  build) build ;;
  shard) build_shard "$2" "$3" ;;
  up) up ;;
  test) run_e2e ;;
  all) build; up; run_e2e ;;
  *) echo "usage: $0 [build|shard N OF|up|test|all]" >&2; exit 2 ;;
esac
