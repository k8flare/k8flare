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
- exec, attach and port-forward work over WebSocket only; pod logs over
  WebSocket are broken. SPDY is not served.
- Watch: 1000-revision compaction window, no RequestWatchProgress,
  progress notifications only on node-lease writes.

## Joining a node

- Done and checked locally through devtls: the Worker forwards a
  Cloudflare-verified client certificate (`request.cf.tlsClientAuth`) and
  the apiserver re-verifies it against the client CA; the edge serves a
  server-CA certificate (`k8flare edge-certificate`); `k8flare token
  create` prints `K10` tokens. A stock `k3s agent` joined, pinned the CA
  hash, and its node went Ready with certificate identities.
- Not usable yet: the stock agent needs a listener on every node at
  `:6443`. The `kubernetes` EndpointSlice publishes node IPs on 6443 (the
  k8flare-agent's local API proxy), so the stock agent rewrites its tunnel
  targets to them and drops its tunnel about a minute after joining, and
  the in-cluster `kubernetes` Service has no backend. CI still joins
  k8flare-agent by default; `AGENT=k3s scripts/ci/e2e.sh up` joins the
  stock agent.
- The agent must run with `--disable-apiserver-lb`.
- Unchecked against Cloudflare: BYO-CA mTLS is Enterprise only, a
  `user_defined` custom certificate from a private CA, and that
  `certRFC9440` is populated for BYO-CA certificates.

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
