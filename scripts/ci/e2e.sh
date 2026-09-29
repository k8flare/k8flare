#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
WORK=.build/ci
LOGS=$WORK/logs
API=127.0.0.1:16443
K3S_VERSION=${K3S_VERSION:-v1.36.2+k3s1}
KUBECONFIG_PATH=$PWD/$WORK/kubeconfig.yaml
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
  (cd scripts && CGO_ENABLED=0 go build -o "../$WORK/devtls" ./devtls)
}

dev_vars() {
  [ -f .dev.vars ] && return 0
  printf 'ADMIN_TOKEN=%s\nREADONLY_TOKEN=%s\nJOIN_TOKEN=%s\nKUBELET_SCHEME=http\nKUBELET_PORT=10255\n' \
    "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > .dev.vars
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

up() {
  if [ ! -x /usr/local/bin/k3s ]; then
    curl -sfL -o "$WORK/k3s" "https://github.com/k3s-io/k3s/releases/download/${K3S_VERSION/+/%2B}/k3s"
    sudo install -m 0755 "$WORK/k3s" /usr/local/bin/k3s
  fi
  sudo /usr/local/bin/k3s ctr version >/dev/null 2>&1 || true
  sudo test -x /var/lib/rancher/k3s/data/current/bin/containerd

  dev_vars
  make wrangler.dev.jsonc
  local admin join worker
  admin=$(sed -n 's/^ADMIN_TOKEN=//p' .dev.vars)
  join=$(sed -n 's/^JOIN_TOKEN=//p' .dev.vars)
  nohup pnpm exec wrangler dev -c wrangler.dev.jsonc --local --enable-containers=false --persist-to .wrangler/state --port 18787 \
    < /dev/null 2>&1 | stamped "$LOGS/dev.log" &
  wait_for "the control plane" 360 5 curl -sf -o /dev/null -H "Authorization: Bearer $admin" http://127.0.0.1:18787/livez
  worker=$(user_worker_port "$admin")
  echo "user worker listens on 127.0.0.1:$worker"
  nohup "$WORK/devtls" -listen "$API" -upstream "http://127.0.0.1:$worker" -dir .build/devtls -hosts localhost \
    < /dev/null 2>&1 | stamped "$LOGS/devtls.log" &
  wait_for "devtls" 60 1 curl -skf -o /dev/null -H "Authorization: Bearer $admin" "https://$API/livez"

  sudo install -m 0644 .build/devtls/ca.crt /usr/local/share/ca-certificates/k8flare-dev-ca.crt
  sudo update-ca-certificates >/dev/null
  printf 'apiVersion: v1\nkind: Config\nclusters:\n- name: k8flare-ci\n  cluster:\n    server: https://%s\n    certificate-authority: %s/.build/devtls/ca.crt\nusers:\n- name: admin\n  user:\n    token: %s\ncontexts:\n- name: k8flare-ci\n  context:\n    cluster: k8flare-ci\n    user: admin\ncurrent-context: k8flare-ci\n' \
    "$API" "$PWD" "$admin" > "$KUBECONFIG_PATH"
  chmod 600 "$KUBECONFIG_PATH"

  sudo nohup "$WORK/k8flare-agent" --server "https://$API" --token "$join" --node-name "$(hostname)" \
    > "$LOGS/agent.log" 2>&1 < /dev/null &
  wait_for "a Ready node" 120 5 node_ready
  wait_for "the API to accept a write" 60 5 write_accepted
  kubectl --kubeconfig "$KUBECONFIG_PATH" get nodes -o wide
}

run_e2e() {
  (cd scripts && go run ./e2e -set "${SET:-required}" -procs "${PROCS:-4}" -kubeconfig "$KUBECONFIG_PATH")
}

case "${1:-all}" in
  build) build ;;
  up) up ;;
  test) run_e2e ;;
  all) build; up; run_e2e ;;
  *) echo "usage: $0 [build|up|test|all]" >&2; exit 2 ;;
esac
