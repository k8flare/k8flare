#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
WORK=.build/ci
LOGS=$WORK/logs
REGISTRY=$WORK/registry
API_PORT=16443
API=localhost:$API_PORT
CONTAINER_API=host.docker.internal:$API_PORT
K3S_VERSION=${K3S_VERSION:-v1.36.2+k3s1}
K3S_IMAGE=rancher/k3s:${K3S_VERSION/+/-}
NODES=${NODES:-1}
AGENT=${AGENT:-k3s}
SERVICE_API=10.43.0.1
KUBECONFIG_PATH=$PWD/$WORK/kubeconfig.yaml
STATE=${STATE:-/dev/shm/k8flare-state}
CLUSTER_DOMAIN=$(sed -n 's/^ *"CLUSTER_DOMAIN": "\([^"]*\)",$/\1/p' wrangler.jsonc)
EDGE_LB_HOST=web--ci-edge.$CLUSTER_DOMAIN
EDGE_INGRESS_HOST=ingress.ci-edge.example
EDGE_GATEWAY_HOST=gateway.ci-edge.example
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
  echo "timed out waiting for $what; the last try of $1 printed:" >&2
  "$@" >&2 || true
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
  printf 'ADMIN_TOKEN=%s\nREADONLY_TOKEN=%s\nJOIN_TOKEN=%s\nSECRETS_ENCRYPTION_KEYS=ci:%s\n' \
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

node_proxy_serves_anonymously() {
  local body
  body=$(curl -sS -m 10 --cacert .build/devtls/server-ca.crt "https://$SERVICE_API/api/v1/namespaces/kube-system/secrets") || return 1
  if ! grep -qF 'system:anonymous' <<<"$body"; then
    echo "https://$SERVICE_API answered without naming system:anonymous: $body"
    return 1
  fi
}

node_names() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get nodes -o jsonpath='{.items[*].metadata.name}'
}

kubelets_reachable() {
  local node
  for node in $(node_names); do
    [ "$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get --raw "/api/v1/nodes/$node/proxy/healthz")" = ok ] || return 1
  done
}

pinned_join() {
  if grep -sE 'Token CA hash will not be validated|token does not include a CA hash' "$LOGS"/agent*.log; then
    echo "an agent joined without checking the K10 token's CA hash" >&2
    return 1
  fi
}

node_identity() {
  local node=$1 kubeconfig=$2 want=$3 got
  local -a node_kubectl=(docker exec "$node" kubectl)
  if [ "$node" = "$(hostname)" ]; then
    node_kubectl=(sudo /usr/local/bin/k3s kubectl)
  fi
  got=$("${node_kubectl[@]}" --kubeconfig "/var/lib/rancher/k3s/agent/$kubeconfig" --request-timeout=15s auth whoami -o jsonpath='{.status.userInfo.username}') || true
  if [ "$got" != "$want" ]; then
    echo "$node: the client certificate in $kubeconfig authenticated as '$got' through https://$API, want '$want'" >&2
    return 1
  fi
  echo "$node: the client certificate in $kubeconfig authenticates as $got"
}

certificate_identities() {
  local node
  for node in $(node_names); do
    node_identity "$node" kubelet.kubeconfig "system:node:$node"
    node_identity "$node" kubeproxy.kubeconfig system:kube-proxy
  done
}

netpol_settled() {
  [ "$(grep -lsE 'Starting network policy controller version|Skipping network policy controller start' "$LOGS"/agent*.log | wc -l)" -ge "$NODES" ]
}

node_metrics_served() {
  local body node
  body=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get --raw /apis/metrics.k8s.io/v1beta1/nodes) || return 1
  for node in $(node_names); do
    if ! jq -e --arg node "$node" 'any(.items[]; .metadata.name == $node and (.usage.cpu | rtrimstr("n") | tonumber) > 0 and (.usage.memory | tonumber) > 0)' <<<"$body" >/dev/null; then
      echo "metrics.k8s.io has no CPU and memory usage for node $node: $body"
      return 1
    fi
  done
}

pod_metrics_served() {
  local running body
  running=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get pods -A --field-selector status.phase=Running -o json |
    jq -c '[.items[] | .metadata.namespace + "/" + .metadata.name]') || return 1
  body=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get --raw /apis/metrics.k8s.io/v1beta1/pods) || return 1
  if ! jq -e --argjson running "$running" 'any(.items[]; (.metadata.namespace + "/" + .metadata.name) as $pod | ($running | any(. == $pod)) and any(.containers[]; (.usage.memory | tonumber) > 0))' <<<"$body" >/dev/null; then
    echo "metrics.k8s.io has memory usage for none of the running pods $running: $body"
    return 1
  fi
}

metrics_served() {
  wait_for "node usage in metrics.k8s.io" 24 5 node_metrics_served
  wait_for "pod usage in metrics.k8s.io" 24 5 pod_metrics_served
  kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s top node
}

claim_state() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-volume get pvc data \
    -o jsonpath='{.status.phase} volumeName={.spec.volumeName} storage-provisioner={.metadata.annotations.volume\.kubernetes\.io/storage-provisioner} selected-node={.metadata.annotations.volume\.kubernetes\.io/selected-node}'
}

claim_waits() {
  local state
  for _ in $(seq 1 6); do
    state=$(claim_state) || return 1
    if [ "$state" != "Pending volumeName= storage-provisioner= selected-node=" ]; then
      echo "the WaitForFirstConsumer claim ci-volume/data moved before any pod used it: $state" >&2
      return 1
    fi
    sleep 5
  done
  echo "the WaitForFirstConsumer claim ci-volume/data stayed unbound for 30s with no pod: $state"
}

volume_pod_ready() {
  [ "$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-volume get pod writer -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')" = True ]
}

claim_bound_to_pod_node() {
  local state node want
  state=$(claim_state) || return 1
  node=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-volume get pod writer -o jsonpath='{.spec.nodeName}') || return 1
  want="Bound volumeName=?* storage-provisioner=rancher.io/local-path selected-node=$node"
  if [ -z "$node" ] || [[ $state != $want ]]; then
    echo "the claim ci-volume/data is '$state' while its pod runs on '$node'"
    return 1
  fi
  echo "the claim ci-volume/data is $state, the pod writer runs there and wrote to it"
}

no_local_path_pods() {
  local pods
  pods=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get pods -A -o json |
    jq -r '.items[] | select(.metadata.labels.app == "local-path-provisioner" or .metadata.annotations["k8flare.io/local-path-failures"] != null or (.metadata.name | test("^(local-path-provisioner|helper-pod)-"))) | .metadata.namespace + "/" + .metadata.name + " on " + (.spec.nodeName // "no node")') || return 1
  if [ -n "$pods" ]; then
    echo "local-path pods exist $1: $pods" >&2
    return 1
  fi
  echo "no local-path pod exists $1"
}

volume_bound_on_first_consumer() {
  local -a k=(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s)
  local mode
  mode=$("${k[@]}" get storageclass local-path -o jsonpath='{.volumeBindingMode}')
  if [ "$mode" != WaitForFirstConsumer ]; then
    echo "the local-path StorageClass binds '$mode', want WaitForFirstConsumer" >&2
    return 1
  fi
  no_local_path_pods "before the claim"
  "${k[@]}" create namespace ci-volume
  "${k[@]}" apply -f - <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data
  namespace: ci-volume
spec:
  storageClassName: local-path
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 16Mi
EOF
  claim_waits
  "${k[@]}" apply -f - <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: writer
  namespace: ci-volume
spec:
  terminationGracePeriodSeconds: 1
  containers:
  - name: writer
    image: rancher/mirrored-library-busybox:1.37.0
    command: [sh, -c, 'echo k8flare > /data/probe && exec sleep 3600']
    readinessProbe:
      exec:
        command: [grep, -qx, k8flare, /data/probe]
      periodSeconds: 2
    volumeMounts:
    - name: data
      mountPath: /data
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: data
EOF
  wait_for "the pod writer to write to its claim" 60 5 volume_pod_ready
  claim_bound_to_pod_node
  no_local_path_pods "after the claim bound"
  "${k[@]}" delete namespace ci-volume --wait=false
}

gateway_crds_established() {
  local crd
  for crd in gatewayclasses gateways httproutes; do
    [ "$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get crd "$crd.gateway.networking.k8s.io" -o jsonpath='{.status.conditions[?(@.type=="Established")].status}')" = True ] || return 1
  done
}

edge_pod_ready() {
  [ "$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-edge get pods -l app=web -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}')" = True ]
}

lb_published() {
  local got
  got=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-edge get svc web -o jsonpath='{.status.loadBalancer.ingress[*].hostname}') || return 1
  if [ "$got" != "$EDGE_LB_HOST" ]; then
    echo "the LoadBalancer Service ci-edge/web is published as '$got', want $EDGE_LB_HOST"
    return 1
  fi
  echo "the LoadBalancer Service ci-edge/web is published as $got"
}

edge_serves() {
  local via=$1 host=$2 pod got
  pod=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-edge get pods -l app=web -o jsonpath='{.items[*].metadata.name}') || return 1
  got=$(curl -sS -m 10 --cacert .build/devtls/server-ca.crt --resolve "$host:$API_PORT:127.0.0.1" -w ' status=%{http_code}' "https://$host:$API_PORT/") || return 1
  if [ "$got" != "k8flare-edge $pod status=200" ]; then
    echo "$via: https://$host:$API_PORT/ answered '$got', want 'k8flare-edge $pod status=200' from the pod"
    return 1
  fi
  echo "$via: https://$host:$API_PORT/ answered '$got' from the pod $pod"
}

gateway_programmed() {
  local gw route
  gw=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-edge get gateway edge -o json) || return 1
  route=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s -n ci-edge get httproute web -o json) || return 1
  if ! jq -e '. as $gw | all("Accepted", "Programmed"; . as $t | any($gw.status.conditions[]?; .type == $t and .status == "True"))' <<<"$gw" >/dev/null; then
    echo "the Gateway ci-edge/edge is not Accepted and Programmed: $(jq -c .status <<<"$gw")"
    return 1
  fi
  if ! jq -e 'any(.status.parents[]?; .controllerName == "k8flare.com/edge" and (. as $p | all("Accepted", "ResolvedRefs"; . as $t | any($p.conditions[]?; .type == $t and .status == "True"))))' <<<"$route" >/dev/null; then
    echo "the HTTPRoute ci-edge/web is not Accepted with ResolvedRefs by k8flare.com/edge: $(jq -c .status <<<"$route")"
    return 1
  fi
  echo "the Gateway ci-edge/edge is Accepted and Programmed and the HTTPRoute ci-edge/web is Accepted with ResolvedRefs"
}

edge_routing() {
  local -a k=(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s)
  wait_for "the Gateway API CRDs to be established" 60 5 gateway_crds_established
  "${k[@]}" create namespace ci-edge
  "${k[@]}" apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: ci-edge
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      terminationGracePeriodSeconds: 1
      containers:
      - name: web
        image: rancher/mirrored-library-busybox:1.37.0
        command: [sh, -c, 'mkdir /www && printf "k8flare-edge %s" "$(hostname)" > /www/index.html && exec httpd -f -p 8080 -h /www']
        ports:
        - containerPort: 8080
        readinessProbe:
          httpGet:
            path: /
            port: 8080
          periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: ci-edge
spec:
  type: LoadBalancer
  selector:
    app: web
  ports:
  - name: http
    port: 80
    targetPort: 8080
EOF
  wait_for "the pod of ci-edge/web to serve" 60 5 edge_pod_ready
  wait_for "the LoadBalancer hostname of ci-edge/web" 24 5 lb_published
  lb_published
  wait_for "$EDGE_LB_HOST to reach the pod through devtls" 24 5 edge_serves LoadBalancer "$EDGE_LB_HOST"
  edge_serves LoadBalancer "$EDGE_LB_HOST"
  "${k[@]}" apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: ci-edge
spec:
  ingressClassName: k8flare
  rules:
  - host: $EDGE_INGRESS_HOST
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web
            port:
              number: 80
EOF
  wait_for "the Ingress host $EDGE_INGRESS_HOST to reach the pod through devtls" 24 5 edge_serves Ingress "$EDGE_INGRESS_HOST"
  edge_serves Ingress "$EDGE_INGRESS_HOST"
  "${k[@]}" apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: k8flare
spec:
  controllerName: k8flare.com/edge
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: edge
  namespace: ci-edge
spec:
  gatewayClassName: k8flare
  listeners:
  - name: http
    protocol: HTTP
    port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: web
  namespace: ci-edge
spec:
  parentRefs:
  - name: edge
  hostnames:
  - $EDGE_GATEWAY_HOST
  rules:
  - backendRefs:
    - name: web
      port: 80
EOF
  wait_for "the Gateway ci-edge/edge and the HTTPRoute ci-edge/web to be accepted" 24 5 gateway_programmed
  gateway_programmed
  wait_for "the HTTPRoute host $EDGE_GATEWAY_HOST to reach the pod through devtls" 24 5 edge_serves Gateway "$EDGE_GATEWAY_HOST"
  edge_serves Gateway "$EDGE_GATEWAY_HOST"
  "${k[@]}" delete namespace ci-edge --wait=false
  "${k[@]}" delete gatewayclass k8flare --wait=false
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
  sudo gdb -p "$pid" -batch -ex 'thread 1' -ex 'bt 60' -ex 'thread apply all bt 30' 2>&1 | sed "s/^/stack pid=$pid /"
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
  if [ "$AGENT" = k8flare ]; then
    sed -n 's/^JOIN_TOKEN=//p' .dev.vars
    return 0
  fi
  local token
  token=$("$WORK/k8flare" token create --kubeconfig "$KUBECONFIG_PATH" --description ci)
  if [[ ! $token =~ ^K10[0-9a-f]{64}:: ]]; then
    echo "k8flare token create printed no K10 token with a CA hash (${#token} characters, starting ${token:0:3})" >&2
    return 1
  fi
  echo "$token"
}

join_node() {
  local token=$1
  if [ "$AGENT" = k8flare ]; then
    sudo install -m 0644 .build/devtls/server-ca.crt /usr/local/share/ca-certificates/k8flare-dev-ca.crt
    sudo update-ca-certificates >/dev/null
    sudo nohup "$WORK/k8flare-agent" --server "https://$API" --token "$token" --node-name "$(hostname)" \
      > "$LOGS/agent.log" 2>&1 < /dev/null &
    return 0
  fi
  docker build -t ghcr.io/k8flare/node-proxy:latest packages/node-proxy
  sudo mkdir -p /var/lib/rancher/k3s/agent/images
  docker save ghcr.io/k8flare/node-proxy:latest | sudo tee /var/lib/rancher/k3s/agent/images/node-proxy.tar >/dev/null
  nohup sudo journalctl -u k3s-agent -f -o short-precise > "$LOGS/agent.log" 2>&1 < /dev/null &
  curl -sfL https://get.k3s.io |
    INSTALL_K3S_SKIP_DOWNLOAD=true K3S_URL="https://$API" K3S_TOKEN="$token" timeout 600 sh -s - agent --disable-apiserver-lb --node-name "$(hostname)"
}

CGROUP_EVACUATE='if [ -f /sys/fs/cgroup/cgroup.controllers ]; then mkdir -p /sys/fs/cgroup/init && xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || :; sed -e "s/ / +/g" -e "s/^/+/" < /sys/fs/cgroup/cgroup.controllers > /sys/fs/cgroup/cgroup.subtree_control; fi; exec "$@"'

join_container_node() {
  local token=$1 index=$2 name
  name=$(hostname)-$index
  local -a run=(docker run -d --name "$name" --hostname "$name" --privileged --cgroupns=private --tmpfs /run --tmpfs /var/run
    --add-host "${CONTAINER_API%%:*}:host-gateway" --entrypoint /bin/sh)
  if [ "$AGENT" = k8flare ]; then
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
      -e K3S_URL="https://$CONTAINER_API" -e K3S_TOKEN="$token" \
      "$K3S_IMAGE" -c "$CGROUP_EVACUATE" sh /bin/k3s \
      agent --node-name "$name" --disable-apiserver-lb >/dev/null
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
  WRANGLER_REGISTRY_PATH=$PWD/$REGISTRY X_LOCAL_OBSERVABILITY=false MINIFLARE_WORKERD_V8_FLAGS="${MINIFLARE_WORKERD_V8_FLAGS:---single-threaded-gc}" nohup pnpm exec wrangler dev -c wrangler.dev.jsonc --local --enable-containers=false --persist-to "$STATE" --port 18787 \
    < /dev/null 2>&1 | stamped "$LOGS/dev.log" &
  wait_for "the control plane" 360 5 curl -sf -m 10 -o /dev/null -H "Authorization: Bearer $admin" http://127.0.0.1:18787/livez
  runtime=$(user_worker_pid)
  nohup bash -c "$(declare -f thread_ms dump_main_thread sample_procs); sample_procs $LOGS/dev.log $runtime $(ps -o ppid= -p "$runtime")" \
    < /dev/null 2>&1 | stamped "$LOGS/procs.log" &
  worker=$(user_worker_port "$admin" "$runtime")
  echo "user worker $runtime listens on 127.0.0.1:$worker"
  nohup "$WORK/devtls" -listen ":$API_PORT" -upstream "http://127.0.0.1:$worker" -dir .build/devtls -hosts "${API%%:*},${CONTAINER_API%%:*},$EDGE_LB_HOST,$EDGE_INGRESS_HOST,$EDGE_GATEWAY_HOST" -admin-token "$admin" -strict-edge \
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
  wait_for "the network policy controller on $NODES nodes" 120 5 netpol_settled
  if grep -s 'Skipping network policy controller start' "$LOGS"/agent*.log; then
    echo "a node skipped the network policy controller" >&2
    return 1
  fi
  wait_for "the API to accept a write" 60 5 write_accepted
  wait_for "every kubelet through the node tunnel" 60 5 kubelets_reachable
  if [ "$AGENT" = k3s ]; then
    pinned_join
    certificate_identities
    wait_for "the node proxy" 60 5 node_proxy_ready
    wait_for "the node proxy to serve $SERVICE_API" 30 5 node_proxy_serves_anonymously
  fi
  kubectl --kubeconfig "$KUBECONFIG_PATH" get nodes -o wide
  node_cost
  metrics_served
  volume_bound_on_first_consumer
  edge_routing
}

node_cost() {
  local node budget procs pods total
  node=$(hostname)
  budget=${NODE_PLATFORM_BUDGET_MIB:-600}
  procs=$(ps -eo rss=,comm= | awk '$2 ~ /^(k8flare-agent|k3s|containerd)/ { s += $1 } END { print int(s / 1024) }')
  pods=$(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s get --raw "/api/v1/nodes/$node/proxy/stats/summary" |
    jq -r '.pods[] | select(.podRef.namespace == "kube-system") | "\(.podRef.name) \((.memory.workingSetBytes // 0) / 1048576 | floor)"') || pods=
  total=$(awk -v procs="$procs" '{ s += $2 } END { print procs + s }' <<<"$pods")
  {
    echo "node=$node nodes=$NODES agent_and_runtime_rss_mib=$procs kube_system_pods_working_set_mib=$((total - procs)) total_mib=$total budget_mib=$budget"
    awk 'NF { print (($1 ~ /^(coredns|k8flare-node-proxy)/) ? "pod " : "unexpected_pod ") $1 " " $2 }' <<<"$pods"
  } | tee "$LOGS/node-cost.txt"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    { echo '```'; cat "$LOGS/node-cost.txt"; echo '```'; } >> "$GITHUB_STEP_SUMMARY"
  fi
  if [ "$NODES" -eq 1 ] && [ "$total" -gt "$budget" ]; then
    echo "the platform costs the node $total MiB, over the budget of $budget MiB" >&2
    return 1
  fi
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
  (cd scripts && ${PROBE_MINUTES:+timeout -k 30 "${PROBE_MINUTES}m"} go run ./e2e -set "${SET:-required}" -focus "${FOCUS:-}" -procs "${PROCS:-4}" -shard "${SHARD:-0}" -shards "${SHARDS:-1}" -kubeconfig "$KUBECONFIG_PATH") || status=$?
  kill "$sampler" 2>/dev/null || true
  if [ -n "${PROBE_MINUTES:-}" ] && [ "$status" -ge 124 ]; then
    echo "probe window of $PROBE_MINUTES min over, spin dumps: $(grep -c 'main thread busy' "$LOGS/procs.log" || true)"
    status=0
  fi
  return "$status"
}

join_evidence() {
  local status=$? log
  [ "$status" -eq 0 ] && return 0
  local -a k=(kubectl --kubeconfig "$KUBECONFIG_PATH" --request-timeout=15s)
  for log in "$LOGS"/agent*.log "$LOGS/devtls.log"; do
    [ -f "$log" ] || continue
    echo "::group::last 300 lines of $log"
    tail -n 300 "$log" || true
    echo "::endgroup::"
  done
  echo "::group::nodes, kube-system pods, events and the node proxy"
  if [ "$AGENT" = k3s ]; then
    sudo systemctl status k3s-agent --no-pager || true
  fi
  "${k[@]}" get nodes -o wide || true
  "${k[@]}" -n kube-system get pods -o wide || true
  "${k[@]}" get events -A --sort-by=.lastTimestamp || true
  "${k[@]}" -n kube-system describe ds k8flare-node-proxy || true
  "${k[@]}" -n kube-system logs -l k8s-app=k8flare-node-proxy --all-containers --prefix --tail=200 || true
  echo "::endgroup::"
  echo "::group::metrics.k8s.io, the claim, its volume and local-path"
  "${k[@]}" get --raw /apis/metrics.k8s.io/v1beta1/nodes || true
  "${k[@]}" get --raw /apis/metrics.k8s.io/v1beta1/pods || true
  "${k[@]}" get storageclass,pv -o wide || true
  "${k[@]}" -n ci-volume describe pvc,pod || true
  "${k[@]}" -n kube-system logs -l app=local-path-provisioner --tail=200 || true
  grep -E 'metrics|hpa:' "$LOGS/dev.log" | tail -n 100 || true
  echo "::endgroup::"
  echo "::group::the edge routes of ci-edge and the Gateway API"
  "${k[@]}" get crd -o wide | grep -E '^NAME|gateway\.networking\.k8s\.io' || true
  "${k[@]}" -n ci-edge get deploy,pods,svc,endpointslices,ingress -o wide || true
  "${k[@]}" -n ci-edge get svc,ingress -o yaml || true
  "${k[@]}" -n ci-edge get gateway,httproute -o yaml || true
  "${k[@]}" get gatewayclass -o yaml || true
  "${k[@]}" -n ci-edge logs -l app=web --tail=50 || true
  grep -E 'consume changed=|gateway|loadbalancer' "$LOGS/dev.log" | tail -n 100 || true
  echo "::endgroup::"
  exit "$status"
}

case "${1:-all}" in
  build) build ;;
  shard) build_shard "$2" "$3" ;;
  up) trap join_evidence EXIT; up ;;
  test) run_e2e ;;
  all) build; trap join_evidence EXIT; up; trap - EXIT; run_e2e ;;
  *) echo "usage: $0 [build|shard N OF|up|test|all]" >&2; exit 2 ;;
esac
