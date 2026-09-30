# README gaps

README.md describes the target. This file tracks what still differs from
it. Remove an entry when the behaviour exists and CI covers it.

## What you get

- Upstream validation is not used for admissionregistration. Every
  upstream strategy there imports `pkg/apis/admissionregistration/validation`,
  which links the CEL compiler: the group worker goes from 56.1 MB to
  70.1 MB (50,963 functions), over the 64 MiB cap even with
  `-tags grpcnotrace`. This also leaves VAP bindings and
  MutatingAdmissionPolicy (and its binding) on the local strategy.
- Claim status writes skip the two upstream authorization checks
  (`resourceclaims/binding`, `resourceclaims/driver`): the strategy gets an
  allow-all authorizer, as the group worker has none.
- CRDs still use the deduced SSA type converter.
- exec, attach and port-forward work over WebSocket only; pod logs over
  WebSocket are broken. SPDY is not served.
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
