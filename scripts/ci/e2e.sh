#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
WORK=.build/ci
LOGS=$WORK/logs
REGISTRY=$WORK/registry
API_PORT=16443
API=127.0.0.1:$API_PORT
CONTAINER_API=host.docker.internal:$API_PORT
K3S_VERSION=${K3S_VERSION:-v1.36.2+k3s1}
K3S_IMAGE=rancher/k3s:${K3S_VERSION/+/-}
NODES=${NODES:-1}
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
  local f keep
  for f in .build/wasm/*.opt.wasm; do
    keep=0
    for m in "${mine[@]}"; do
      [ "$f" = "$m" ] && keep=1
    done
    if [ "$keep" = 0 ]; then
      echo "shard $shard/$shards drops $f, another shard builds it"
      rm -f "$f" "$f.sha256" "${f%.opt.wasm}.raw.wasm"
    fi
  done
}

dev_vars() {
  [ -f .dev.vars ] && return 0
  printf 'ADMIN_TOKEN=%s\nREADONLY_TOKEN=%s\nJOIN_TOKEN=%s\nSECRETS_ENCRYPTION_KEYS=ci:%s\nKUBELET_SCHEME=http\nKUBELET_PORT=10255\n' \
    "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -base64 32)" > .dev.vars
  chmod 600 .dev.vars
}

schedulable_nodes() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get nodes \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .status.conditions[?(@.type=="Ready")]}{.status}{end}{"\t"}{range .status.conditions[?(@.type=="NetworkUnavailable")]}{.status}{end}{"\t"}{.spec.unschedulable}{"\t"}{.spec.taints}{"\n"}{end}' |
    awk -F'\t' '$2 == "True" && $3 != "True" && $4 != "true" && $5 == ""' | wc -l
}

nodes_ready() {
  [ "$(schedulable_nodes)" -ge "$1" ]
}

node_proxy_ready() {
  [ "$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n kube-system get ds k8flare-node-proxy -o jsonpath='{.status.numberReady}')" -ge "$NODES" ]
}

write_accepted() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s create namespace ci-writecheck &&
    kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s delete namespace ci-writecheck --wait=false
}

debug_port() {
  sed -n 's/.*"debugPortAddress": *"127\.0\.0\.1:\([0-9]*\)".*/\1/p' "$REGISTRY/k8flare" 2>/dev/null | head -1
}

user_worker_pid() {
  local debug pid
  for _ in $(seq 1 30); do
    debug=$(debug_port)
    if [ -n "$debug" ]; then
      pid=$(ss -ltnpH "sport = :$debug" | grep -o 'pid=[0-9]*' | head -1 | cut -d= -f2)
      if [ -n "$pid" ]; then
        echo "$pid"
        return 0
      fi
    fi
    sleep 2
  done
  echo "wrangler dev did not register the user worker in $REGISTRY" >&2
  return 1
}

user_worker_port() {
  local admin=$1 pid=$2 debug port
  debug=$(debug_port)
  for _ in $(seq 1 30); do
    for port in $(ss -ltnpH | awk -v pid="pid=$pid," '$0 ~ pid { n = split($4, a, ":"); print a[n] }' | sort -u); do
      [ "$port" = "$debug" ] && continue
      if curl -sf -m 5 -o /dev/null -H "Authorization: Bearer $admin" "http://127.0.0.1:$port/livez"; then
        echo "$port"
        return 0
      fi
    done
    sleep 2
  done
  echo "no port of workerd $pid answers /livez" >&2
  return 1
}

thread_ms() {
  local tick=$1
  shift
  cat "$@" 2>/dev/null | sed 's/.*) //' | awk -v t="$tick" '{ s += $12 + $13 } END { print int(s * 1000 / t) }'
}

dump_main_thread() {
  local pid=$1
  command -v gdb >/dev/null 2>&1 || sudo apt-get install -y -qq gdb >/dev/null 2>&1 || true
  if ! command -v gdb >/dev/null 2>&1; then
    echo "stack pid=$pid gdb unavailable"
    return 0
  fi
  sudo gdb -p "$pid" -batch -ex 'thread 1' -ex 'bt 60' 2>&1 | sed "s/^/stack pid=$pid /"
}

sample_procs() {
  local devlog=$1 tick pid main other prev_main prev_other stat quiet=0 devlog_size prev_devlog_size=-1 dumped=0
  shift
  local runtime=$1
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
      [ "$pid" = "$runtime" ] || continue
      devlog_size=$(stat -c %s "$devlog" 2>/dev/null || echo 0)
      if [ $((main - prev_main)) -ge 900 ] && [ "$devlog_size" = "$prev_devlog_size" ]; then
        quiet=$((quiet + 1))
      else
        quiet=0
        dumped=0
      fi
      prev_devlog_size=$devlog_size
      if [ "$quiet" -ge 20 ] && [ "$dumped" = 0 ]; then
        dumped=1
        echo "stack pid=$pid main thread busy for ${quiet}s with no new line in $devlog"
        dump_main_thread "$pid"
      fi
    done
    sleep 1
  done
}

join_token() {
  if [ "${AGENT:-k8flare}" = k8flare ]; then
    sed -n 's/^JOIN_TOKEN=//p' .dev.vars
    return 0
  fi
  "$WORK/k8flare" token create --kubeconfig "$KUBECONFIG_PATH" --ttl 1h --description ci
}

join_node() {
  local token=$1
  if [ "${AGENT:-k8flare}" = k8flare ]; then
    sudo install -m 0644 .build/devtls/server-ca.crt /usr/local/share/ca-certificates/k8flare-dev-ca.crt
    sudo update-ca-certificates >/dev/null
    sudo nohup "$WORK/k8flare-agent" --server "https://$API" --token "$token" --node-name "$(hostname)" \
      > "$LOGS/agent.log" 2>&1 < /dev/null &
    return 0
  fi
  docker build -t ghcr.io/k8flare/node-proxy:latest packages/node-proxy
  sudo mkdir -p /var/lib/rancher/k3s/agent/images
  docker save ghcr.io/k8flare/node-proxy:latest | sudo tee /var/lib/rancher/k3s/agent/images/node-proxy.tar >/dev/null
  sudo nohup /usr/local/bin/k3s agent --server "https://$API" --token "$token" --node-name "$(hostname)" --disable-apiserver-lb \
    > "$LOGS/agent.log" 2>&1 < /dev/null &
}

CGROUP_EVACUATE='if [ -f /sys/fs/cgroup/cgroup.controllers ]; then mkdir -p /sys/fs/cgroup/init && xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || :; sed -e "s/ / +/g" -e "s/^/+/" < /sys/fs/cgroup/cgroup.controllers > /sys/fs/cgroup/cgroup.subtree_control; fi; exec "$@"'

join_container_node() {
  local token=$1 index=$2 name
  name=$(hostname)-$index
  local -a run=(docker run -d --name "$name" --hostname "$name" --privileged --cgroupns=private --tmpfs /run --tmpfs /var/run
    --add-host "${CONTAINER_API%%:*}:host-gateway" --entrypoint /bin/sh)
  if [ "${AGENT:-k8flare}" = k8flare ]; then
    "${run[@]}" \
      -v /var/lib/rancher/k3s/data:/var/lib/rancher/k3s/data:ro \
      -v "$PWD/$WORK/k8flare-agent:/k8flare-agent:ro" \
      -v "$PWD/.build/devtls/server-ca.crt:/k8flare-dev-ca.crt:ro" \
      -e SSL_CERT_FILE=/k8flare-dev-ca.crt \
      "$K3S_IMAGE" -c "$CGROUP_EVACUATE" sh /k8flare-agent \
      --server "https://$CONTAINER_API" --token "$token" --node-name "$name" >/dev/null
  else
    "${run[@]}" \
      -v /var/lib/rancher/k3s/agent/images:/var/lib/rancher/k3s/agent/images:ro \
      "$K3S_IMAGE" -c "$CGROUP_EVACUATE" sh /bin/k3s \
      agent --server "https://$CONTAINER_API" --token "$token" --node-name "$name" --disable-apiserver-lb >/dev/null
  fi
  nohup docker logs -f "$name" > "$LOGS/agent-$index.log" 2>&1 < /dev/null &
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
  local admin runtime worker
  admin=$(sed -n 's/^ADMIN_TOKEN=//p' .dev.vars)
  rm -rf "$REGISTRY"
  WRANGLER_REGISTRY_PATH=$PWD/$REGISTRY X_LOCAL_OBSERVABILITY=false nohup pnpm exec wrangler dev -c wrangler.dev.jsonc --local --enable-containers=false --persist-to "$STATE" --port 18787 \
    < /dev/null 2>&1 | stamped "$LOGS/dev.log" &
  wait_for "the control plane" 360 5 curl -sf -m 10 -o /dev/null -H "Authorization: Bearer $admin" http://127.0.0.1:18787/livez
  runtime=$(user_worker_pid)
  nohup bash -c "$(declare -f thread_ms dump_main_thread sample_procs); sample_procs $LOGS/dev.log $runtime $(ps -o ppid= -p "$runtime")" \
    < /dev/null 2>&1 | stamped "$LOGS/procs.log" &
  worker=$(user_worker_port "$admin" "$runtime")
  echo "user worker $runtime listens on 127.0.0.1:$worker"
  nohup "$WORK/devtls" -listen ":$API_PORT" -upstream "http://127.0.0.1:$worker" -dir .build/devtls -hosts "localhost,${CONTAINER_API%%:*}" -admin-token "$admin" \
    < /dev/null 2>&1 | stamped "$LOGS/devtls.log" &
  wait_for "devtls" 180 1 curl -sf -m 10 --cacert .build/devtls/server-ca.crt -o /dev/null -H "Authorization: Bearer $admin" "https://$API/livez"

  printf 'apiVersion: v1\nkind: Config\nclusters:\n- name: k8flare-ci\n  cluster:\n    server: https://%s\n    certificate-authority: %s/.build/devtls/server-ca.crt\nusers:\n- name: admin\n  user:\n    token: %s\ncontexts:\n- name: k8flare-ci\n  context:\n    cluster: k8flare-ci\n    user: admin\ncurrent-context: k8flare-ci\n' \
    "$API" "$PWD" "$admin" > "$KUBECONFIG_PATH"
  chmod 600 "$KUBECONFIG_PATH"

  local token index
  token=$(join_token)
  join_node "$token"
  wait_for "a Ready node" 120 5 nodes_ready 1
  if [ "$NODES" -gt 1 ]; then
    docker pull -q "$K3S_IMAGE"
    sudo iptables -C DOCKER-USER -o docker0 -j ACCEPT 2>/dev/null || sudo iptables -I DOCKER-USER -o docker0 -j ACCEPT
    for index in $(seq 2 "$NODES"); do
      join_container_node "$token" "$index"
    done
    wait_for "$NODES Ready untainted nodes" 120 5 nodes_ready "$NODES"
  fi
  wait_for "the API to accept a write" 60 5 write_accepted
  if [ "${AGENT:-k8flare}" = k3s ]; then
    wait_for "the node proxy" 60 5 node_proxy_ready
  fi
  kubectl --kubeconfig "$KUBECONFIG_PATH" get nodes -o wide
}

sample_resources() {
  while true; do
    echo "resources $(date -u +%H:%M:%S) $(free -m | awk '/^Mem:/ { print "mem_used_mb=" $3 " mem_available_mb=" $7 }') $(df -m / /dev/shm | awk 'NR > 1 { printf "%s_used_mb=%s ", $6, $3 }') $(du -sm "$STATE"/v3/* 2>/dev/null | awk '{ n = split($2, a, "/"); printf "state_%s_mb=%s ", a[n], $1 }') workerd_rss_mb=$(ps -C workerd -o rss= | awk '{ s += $1 } END { print int(s / 1024) }')"
    sleep 60
  done
}

run_e2e() {
  local status=0
  sample_resources &
  local sampler=$!
  (cd scripts && go run ./e2e -set "${SET:-required}" -focus "${FOCUS:-}" -procs "${PROCS:-4}" -shard "${SHARD:-0}" -shards "${SHARDS:-1}" -kubeconfig "$KUBECONFIG_PATH") || status=$?
  kill "$sampler" 2>/dev/null || true
  return "$status"
}

case "${1:-all}" in
  build) build ;;
  shard) build_shard "$2" "$3" ;;
  up) up ;;
  test) run_e2e ;;
  all) build; up; run_e2e ;;
  *) echo "usage: $0 [build|shard N OF|up|test|all]" >&2; exit 2 ;;
esac
