#!/bin/sh
# cgroup v2 root evacuation, same dance as k3s's official docker entrypoint:
# the kubelet needs to create /sys/fs/cgroup/kubepods with domain
# controllers, which cgroupv2 forbids while processes sit in the root
# cgroup ("invalid state" otherwise -- hit live in S15's first local run).
set -e
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  mkdir -p /sys/fs/cgroup/init
  busybox xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || true
  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers > /sys/fs/cgroup/cgroup.subtree_control || true
fi
# Task #13's virtual kube-proxy (pkg/vkubeproxy) needs /dev/net/tun.
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
# (pkg/dnsshim's NodeLocalDNSIP is the precedent: cmd/agent and
# pkg/apiserver are different Go build targets, one js/wasm, with no
# shared-constant seam between them).
exec k8flare-agent -server "$SERVER_URL" -node-name "$NODE_NAME" -token "$K3S_TOKEN" -with-node-id=false -node-labels "k8flare.com/backend=containers" -node-taints "k8flare.com/pod-on-containers=true:NoSchedule" -kubelet-plain-proxy-port 10256 -virtual-kube-proxy-cidr 10.43.0.0/16
