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
exec k8flare-agent -server "$SERVER_URL" -node-name "$NODE_NAME" -token "$K3S_TOKEN"
