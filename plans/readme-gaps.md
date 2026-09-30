# README gaps

README.md describes the target. This file tracks what still differs from
it. Remove an entry when the behaviour exists and CI covers it.

## What you get

- Upstream validation is not used for admissionregistration (CEL pushed
  the worker past 64 MiB) and pods and nodes run local strategies
  (upstream `pkg/registry/core/{pod,node}` pulls in the kubelet client and
  grpc). Retry with `-tags grpcnotrace`, which cut attach/detach from
  69.9 MB to 44.1 MB.
- VAP bindings, MutatingAdmissionPolicy, resourceclaims and
  resourceclaimtemplates have no upstream strategy.
- CRDs still use the deduced SSA type converter.
- SPDY is not served and cannot be: workerd only accepts `Upgrade:
  websocket` (KJ `HttpHeaders::isWebSocket`, `acceptWebSocket`), so stream
  paths answer other upgrades with 426. exec, attach and port-forward
  (`v5`/`v4.channel.k8s.io`, `SPDY/3.1+portforward.k8s.io`) and pod logs
  (`binary.k8s.io`, `base64.binary.k8s.io`) work over WebSocket. The pod-log
  fix (fetch `containerLogs` over HTTP, relay one way) has no recorded CI
  run; README needs "over WebSocket" and no SPDY claim.
- Watch: 1000-revision compaction window, no RequestWatchProgress,
  progress notifications only on node-lease writes.

## Joining a node

- Nodes run `k8flare-agent` (a patched k3s agent with bearer-token
  kubeconfigs), not stock `k3s agent`. No client-certificate
  authentication and no K10 CA pinning. CI joins k8flare-agent.

## Packaged components

- HelmChart is not installed by anything. Helm's engine does not fit a
  Worker (about 70 MB); options are a text/template + sprig engine or
  k3s's klipper-helm Job.
- Ingress and Gateway API are not routed at the edge.
- local-path runs as a resident Deployment, not helper pods only.
- CoreDNS NodeHosts is never written.
- attach/detach runs from a separate Worker config that CI does not load.

## Operations

- Certificates: no leaf or CA rotation.
- Snapshots: no scheduled snapshots.
- Audit log: none.
- Upgrades: no storage migration mechanism.
- Bootstrap-token Secrets are not accepted for joins.

## Security

- No anonymous auth, no OIDC authenticator, Access groups are not mapped
  to Kubernetes groups.
- Internal Workers still authenticate with the shared ADMIN_TOKEN.
- No API Priority and Fairness or rate limiting.
- Admission: OwnerReferencesPermissionEnforcement,
  ClusterTrustBundleAttest and DenyServiceExternalIPs are missing.

## Cloudflare features

- `APIService` cannot point at a Worker.

## Conformance

- CI gates on 21 required specs. The nightly full run has no recorded
  result yet. Multi-node networking is not exercised.
