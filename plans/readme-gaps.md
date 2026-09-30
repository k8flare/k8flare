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
- Streaming over WebSocket (logs, exec, attach, port-forward) has no CI
  spec yet. SPDY cannot be served: workerd only accepts `Upgrade:
  websocket`, so stream paths answer other upgrades with 426.

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
- Snapshots: scheduled ones (`SNAPSHOT_INTERVAL_HOURS`, default 12, 0 disables;
  `SNAPSHOT_RETENTION`, default 5) are tested against a fake R2 only, not
  against workerd.
- Cluster store: compaction keeps 5 to 10 minutes of history with a
  100000-revision cap; the cluster-store tests run the Durable Object on
  node:sqlite, not workerd.
- Audit log: the front records every request it routes as metadata
  (`AUDIT_POLICY` overrides `DefaultAuditPolicy`), unit tests only. Not yet:
  - Request and RequestResponse levels behave as Metadata; bodies and
    upstream's `omitManagedFields` need the group workers, which run the
    REST handlers, to log objects.
  - `edgehost.Proxy` and the supervisor routes bypass the filter chain.
  - Error `Status` messages (403 reason) are not in the event, and the
    401 "attempted: bearer" detail is overwritten by the 401 body.
  - Not run in a deployed Worker: `tail()` must relay the lines to
    Workers Logs.
- Upgrades: no storage migration mechanism.
- Bootstrap-token Secrets are not accepted for joins.

## Security

- Anonymous auth, the OIDC authenticator and Access group mapping have
  unit tests only; CI does not exercise them against a running cluster.
  - Anonymous requests get `system:anonymous` and RBAC decides. A bad
    bearer token is still 401.
  - OIDC is a request-scoped JWT verifier in `apiserver-auth`, not
    upstream's `token/oidc` (that adds 26 MB and takes the front to
    71 MB, past the 64 MiB cap). It accepts RS/PS/ES algorithms, drops
    `system:` groups from claims, has no CEL claim rules or
    `--oidc-signing-algs`, and does not refetch keys on an unknown `kid`
    until the 10 minute cache expires. Next: refetch on unknown `kid`,
    match upstream's algorithm and `system:` rules, and serve
    `AuthenticationConfiguration` CEL (claimMappings,
    claimValidationRules, userValidationRules) by delegating expression
    evaluation to the admission Worker, which already links CEL.
  - Access groups come from the token's `custom.<ACCESS_GROUPS_CLAIM>`;
    the IdP must send them, and Access trims `custom` above about 1 KB.
    `get-identity` is not used.
- Internal Workers no longer hold `ADMIN_TOKEN`; the front derives a
  token per component. The scheduler, GC, HPA and attach/detach run as
  their upstream identities. Still open:
  - Workloads, addons, admission and hookecho run in `system:masters`
    under their own identities, because workloads runs many controllers
    and addons applies arbitrary manifests.
  - Workers that hold the `STORAGE` binding (GC, workloads, admission)
    read and write the datastore without RBAC.
  - Group workers, CustomResources and the queue consumers still use
    `ADMIN_TOKEN` or trust `X-Remote-User` from the front.
  - `hpa-worker` and `attachdetach-worker` need the front deployed first
    (new `HPAAPIServer` and `AttachDetachAPIServer` entrypoints).
- Rate limiting is max-in-flight only (`MAX_REQUESTS_INFLIGHT`,
  `MAX_MUTATING_REQUESTS_INFLIGHT`, 400 and 200, watches and streams
  exempt, `system:masters` served when full). The count is per isolate,
  not cluster-wide. API Priority and Fairness does not build against the
  lean client-go (no FlowControl informers) and its controller needs
  goroutines and informers the Workers do not keep; no Workers Rate
  Limiting binding is wired.
- Admission: OwnerReferencesPermissionEnforcement,
  ClusterTrustBundleAttest and DenyServiceExternalIPs are missing.

## Cloudflare features

- `APIService` cannot point at a Worker.

## Conformance

- CI gates on 21 required specs. Multi-node networking is not exercised.
- First full run (run 36698321866, main at 005a405, one node): 290 of 446
  specs ran before the job was interrupted at 76 minutes (cause not yet
  found; the job timeout is 355 minutes): 255 passed, 35 failed.
  - DNS: the four `[sig-network] DNS` specs.
  - Services and proxying: NodePort, session affinity (3), multiport,
    ClusterIP/NodePort to ExternalName, proxy through a service and a
    pod, endpoints latency, the kubectl guestbook.
  - StatefulSet: five specs fail in BeforeEach.
  - EndpointSliceMirroring, Events API lifecycle, Job
    backoffLimitPerIndex, OrderedNamespaceDeletion, pod generation.
  - AdmissionWebhook (deny attaching pod, mutate pod with defaults),
    Aggregator sample API server, ServiceAccountIssuerDiscovery.
  - Storage: CSI PV/PVC lifecycle, VolumeAttributesClass lifecycle.
  - "at least two untainted nodes" needs a second node in CI.
