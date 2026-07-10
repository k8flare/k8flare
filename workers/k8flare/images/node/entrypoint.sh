#!/bin/sh
# cgroup v2 root evacuation, same dance as k3s's official docker entrypoint:
# the kubelet needs to create /sys/fs/cgroup/kubepods with domain
# controllers, which cgroupv2 forbids while processes sit in the root
# cgroup ("invalid state" otherwise -- hit live in S15's first local run).
set -e
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  # Best-effort, like its two sibling lines below (this one was missing
  # its own `|| true` -- under `set -e` that's not "skip this step",
  # it's "abort the whole entrypoint with no further output": a
  # container run without cgroup-namespace write access (e.g. a plain,
  # non-privileged `wrangler dev` local Docker run, not a real
  # Firecracker microVM) exits 1 here before k8flare-agent ever starts,
  # surfacing upstream only as `@cloudflare/containers`' generic
  # "Container exited with unexpected exit code: 1" -- no entrypoint
  # output at all, since this line never got the chance to fail loudly.
  # Reproduced live in CI 2026-07-11 (smoke-nodes.yml).
  mkdir -p /sys/fs/cgroup/init || true
  xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || true
  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers > /sys/fs/cgroup/cgroup.subtree_control || true
fi
# Task #13's virtual kube-proxy (pkg/agent) needs /dev/net/tun.
# Firecracker microVMs are expected to devtmpfs-populate it automatically
# (unverified against a real deployment -- see
# docs/platform-verification.md); this mknod is a defensive fallback,
# best-effort like the cgroup dance above -- its absence only means
# ClusterIP routing won't work on this node, not a boot failure.
if [ ! -e /dev/net/tun ]; then
  mkdir -p /dev/net && mknod /dev/net/tun c 10 200 && chmod 600 /dev/net/tun || true
fi
# The gateway's logs/metrics bridge enters through the agent's
# plain-HTTP kubelet proxy (:10256, -kubelet-plain-proxy-port below --
# Workers can't TLS to the kubelet's self-signed 10250). Kubelet
# security config stays STOCK k3s: anonymous disabled, webhook token
# authentication and Webhook authorization -- the bridge forwards the
# cluster bearer token and the kubelet TokenReviews/SubjectAccessReviews
# it against this cluster's apiserver (pkg/apiserver/tokenreview.go),
# exactly like a real cluster. Without a valid cluster token the
# bridged port answers 401.
# -virtual-kube-proxy-cidr must match pkg/apiserver/supervisor.go's
# ServiceCIDR ("10.43.0.0/16") -- duplicated as a literal rather than
# shared, same call as this project's other cross-binary constants
# (pkg/agent's NodeLocalDNSIP is the precedent: cmd/agent and
# pkg/apiserver are different Go build targets, one js/wasm, with no
# shared-constant seam between them).
#
# Per-Pod Cloudflare Mesh membership (spikes/s17-mesh-nodevm/FINDINGS.md's
# per-Pod-Mesh entry): MESH_CONNECTOR_TOKEN is set per-Pod by
# scheduler.ts's createMeshConnector() via nodevm.ts's up() -- absent on
# a deployment without CLOUDFLARE_API_TOKEN/ACCOUNT_ID configured, in
# which case this whole block is a no-op and the Pod boots exactly as
# before this feature existed. `warp-svc` (the daemon `warp-cli`/
# pkg/agent talk to) has no init system to start it under here
# (unlike a real BYO VM, where the `cloudflare-warp` .deb's postinst
# enables a systemd unit) -- this microVM's entrypoint IS PID 1, so it
# must start and wait for the daemon itself before k8flare-agent's
# meshconnector.Run tries to talk to it. Bounded 10s readiness wait, same
# style as meshconnector.go's own bounded 30s waitForMeshIP poll -- a
# one-time boot-sequence wait, not a resident poll loop (cost invariant
# #3 is about DO alarms, not process startup).
if [ -n "$MESH_CONNECTOR_TOKEN" ]; then
  warp-svc >/var/log/warp-svc.log 2>&1 &
  i=0
  while [ "$i" -lt 10 ]; do
    warp-cli --accept-tos status >/dev/null 2>&1 && break
    i=$((i + 1))
    sleep 1
  done
fi
exec k8flare-agent -server "$SERVER_URL" -node-name "$NODE_NAME" -token "$K3S_TOKEN" -with-node-id=false -node-labels "k8flare.com/backend=containers" -node-taints "k8flare.com/pod-on-containers=true:NoSchedule" -kubelet-plain-proxy-port 10256 -virtual-kube-proxy-cidr 10.43.0.0/16 -mesh-ip-as-node-ip=true
