# S15 — cf-containers-style node-in-a-Container: local Docker round (2026-07-06)

Goal: replace workers/nodes' hand-written VirtualNode/PodContainer backend
with EKS-cf-containers-shaped per-Pod ephemeral nodes: one microVM per Pod,
running OUR fixed node image (unmodified k3s agent embed = kubelet +
containerd), containerd pulling the Pod's arbitrary image inside.

## Round 1: local Docker (cost-free stand-in for the microVM) — PROVEN

`docker run --privileged -v s15-data:/var/lib/rancher/k3s k8flare/containers-node`
against the PRODUCTION gateway:

- Node `s15-local-docker-<id>` registered and went Ready (kubelet
  v1.36.2-k3s1), kube-proxy started.
- `nginx:1.27` — an arbitrary, non-allowlisted image — was scheduled there
  (host cmd/scheduler), pulled by the inner containerd, and reached
  Running/ready in ~110s including the pull. crictl inside confirms the
  container. THE image-allowlist limitation dissolves with this design.

Build/runtime lessons (all encoded in Dockerfile/entrypoint.sh):
1. cmd/agent requires the official `k3s` binary on PATH (self-extracts
   containerd/runc/CNI on first `k3s check-config`).
2. cgroupv2 root evacuation required (same dance as k3s's docker image).
3. overlayfs-on-overlayfs is EINVAL — the k3s data dir needs a real
   filesystem (volume locally; microVM disk should be fine in production).
4. Builder needs jq/perl-utils(shasum)/python3 for this repo's mirror scripts.

## apiserver gaps surfaced by a REAL kubelet (follow-ups)

- kubelet's node-status PATCH (strategic-merge with $setElementOrder)
  intermittently fails "unknown" — node still reached Ready, but the
  patch path needs a look.
- events.k8s.io POSTs made the gateway throw (1101) — group unregistered;
  should 404 as JSON instead of crashing.
- Another phantom-state episode: RS status.replicas=1 with zero Pods
  wedged the RS controller out of recreating (fixed by deleting the RS);
  same Cluster-DO consistency class as docs' OPEN follow-up.

## Round 2 (next): Cloudflare Containers microVM

Same image (amd64), one instance via a throwaway spike Worker, verify
nested containerd inside the real microVM, then teardown. Wall-clock cost
while running ≈ $0.07/h (standard-1 + 4GiB) — minutes-scale test.

## Round 2: Cloudflare Containers microVM (2026-07-06) — PROVEN, spike complete

Throwaway worker k8flare-s15-spike (deployed, verified, then fully
deleted): one standard-1 microVM booted the same node image (amd64,
host-cross-compiled agent + official k3s binary).

- Agent came up inside the microVM (kubelet port 10250 reachable ->
  Container state "healthy" ~60s after /up).
- Node `s15-cf-microvm-1f7ac066` registered against the PRODUCTION
  gateway and went Ready (v1.36.2-k3s1).
- `nginx:1.27` (arbitrary, non-allowlisted) scheduled there and reached
  Running/ready in 102s including the containerd pull INSIDE the microVM.
- No overlayfs issue in the real microVM (as predicted; Docker-only).
- Cleanup verified: deployment deleted (pods -A empty), container
  destroyed, spike worker deleted, node object removed. Total billed
  microVM wall-clock: ~12 minutes of standard-1 (~$0.015).

VERDICT: the cf-containers-style replacement is feasible end-to-end on the
real platform. Remaining engineering (not spike): per-Pod lifecycle
manager in workers/nodes (pending annotated pod -> boot VM with
NODE_NAME=pod-derived, envVars token via secret -> bind -> destroy on
pod deletion), node image productization, apiserver followups
(node-status SMP patch, events.k8s.io), and removing the old
VirtualNode/PodContainer backend.
