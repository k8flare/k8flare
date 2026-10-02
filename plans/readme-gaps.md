# README gaps

README.md describes the target. This file tracks what still differs from
it. Remove an entry when the behaviour exists and CI covers it.

Decided on 2026-10-01, with the owner:

- Where README and the code differ, the code changes: deadlines move onto
  Durable Object alarms and nothing runs on an unconditional timer. The
  one exception is the join line, which gained `--disable-apiserver-lb`
  because no change here can make a stock agent join without it.
- Network policy is turned on.
- main follows this branch whenever Unit and E2E pass on it.
- The Cloudflare account may be deployed to, but it is over its free
  allowance: light checks only, no full Conformance run there.

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
- Streaming over WebSocket: logs and exec pass in Conformance (36857951122:
  `remote command execution over websockets`, `retrieving logs from the
  container over websockets`). attach has no CI spec. `kubectl
  port-forward` is unverified: kubectl 1.36 first offers the
  `SPDY/3.1+portforward.k8s.io` tunnel
  (`kubectl/pkg/cmd/portforward/portforward.go:145-153`), upstream answers
  it with `translator.NewTunnelingHandler` unless the kubelet takes it
  (`pkg/registry/core/pod/rest/subresources.go:296-304`), and nothing here
  translates; the front relays the offered subprotocol to the kubelet
  (`edgehost/stream.go`). SPDY cannot be served: workerd only accepts
  `Upgrade: websocket`, so stream paths answer other upgrades with 426.
- `apiservices` has no upstream validation either
  (`apiserver-apiregistration/hooks.go`).
- ValidatingAdmissionPolicy is evaluated by a local cel-go environment
  (`admission/vap.go`, `cel.go`): `params` is never bound (no `ParamRef`
  lookup), `matchConditions` and `auditAnnotations` are not read, an
  expression error denies whatever `failurePolicy` says, and the `Warn`
  and `Audit` actions produce nothing. MutatingAdmissionPolicy
  (`admission/map.go`) passes no params and uses the deduced type
  converter, so an ApplyConfiguration is not schema-aware.
- `jobs/scale` is served (`apiserver-registry/zz_generated_resources.go`),
  which upstream does not serve; it came in with 86c0c1a.
- Implemented with no CI check: protobuf bodies (the e2e client is pinned
  to JSON, `scripts/e2e/conformance.go`), Table output, `sendInitialEvents`,
  CRD `x-kubernetes-validations`, custom resource `/status` and `/scale`.
  `unit.yml` skips the tests that would cover several of these because they
  need a running cluster: TestAdmissionExtensions, TestCompaction,
  TestConfigMapVerbs, TestControllers, TestCustomResources,
  TestInformerWatchList, TestOpenAPI, TestNodePodCIDRAndPodLifecycle,
  TestRBAC, TestScheduler, TestSchedulerWakesOnNode, TestSupervisorJoin,
  TestTables, TestWatch, TestWatchSelectorTransitions,
  TestNodeFieldSelectorDefaults.
- Controllers, against `cmd/kube-controller-manager/app/controller_descriptor.go`:
  - serviceaccount-token-controller is absent: nothing fills a
    `kubernetes.io/service-account-token` Secret.
  - validatingadmissionpolicy-status is linked (`workloads/shards/vap`) and
    must not be run yet: an admission policy write was routed to it in
    252dd21 and the pass panicked with a nil dereference where it lists
    ValidatingAdmissionPolicies (`workloads/workloads.go`, the
    `validatingadmissionpolicies` source), stalling the workloads queue
    about 230s per panic (run 36885722274: 439 of 446). The write no longer
    starts a pass. What is left: find why that list call has no client in
    the `workloads-vap` worker, and decide what happens to the inline type
    check (`apiserver-admissionregistration/hooks.go`) once upstream's
    controller writes the same status field. service-cidr now runs on
    ServiceCIDR writes; device-taint-eviction on pod, ResourceClaim and
    ResourceSlice writes.
  - The HPA controller is loaded in CI since 23aabe4: the main Worker
    consumes `k8flare-hpa` and calls the `HorizontalPodAutoscaler`
    entrypoint.
  - Node health deletes a pod that tolerates the taint without
    `tolerationSeconds`: `tolerationWait` (`workloads/nodehealth.go`)
    returns not-tolerated for it. Upstream never evicts such a pod. Read,
    not run; no test.
  - Job backoff has no test that a delay is booked; the 10 s recheck of
    unfinished Jobs (`workloads.go`) would hide a missing one.
  - Read, not run: the scheduler fills no ReplicationController,
    ReplicaSet or StatefulSet lister (`scheduler/scheduler.go`), which
    PodTopologySpread's default selectors read.

## Architecture

- Controller deadlines ride on Queues `delaySeconds`
  (`control-plane-worker/src/queues.ts`, `edgehost/followup.go`), not on
  Durable Object alarms; the Cluster DO alarm only does store housekeeping.
  They do fire with no further write.
- Things poll: the metrics scrape re-sends itself every 15 s whatever it
  found (`followup.go`, case `metrics`) and wakes HPA each time; an
  unfinished Job re-books a workloads pass every 10 s; each node gets a
  lease check about every 60 s; unschedulable pods are retried within 60 s.
- node-tunnel imports its wasm statically into the Durable Object instead
  of loading it through the Worker Loader.

## Joining a node

- Done and checked locally through devtls: the Worker forwards a
  Cloudflare-verified client certificate (`request.cf.tlsClientAuth`) and
  the apiserver re-verifies it against the client CA; the edge serves a
  server-CA certificate (`k8flare edge-certificate`); `k8flare token
  create` prints `K10` tokens. A stock `k3s agent` joined, pinned the CA
  hash, and its node went Ready with certificate identities.
- The stock agent and the node API proxy: the `kubernetes` EndpointSlice
  publishes each Ready node's IP on 6443 as `ready=false, serving=true,
  terminating=true`. The k3s tunnel watch skips endpoints that are not
  ready and ignores an empty list, so a stock agent keeps the tunnel it
  opened to the server URL (the supervisor's `/v1-k3s/apiservers` now
  returns `host:port`, so the address key matches). kube-proxy falls back
  to serving-terminating endpoints when none is ready, so `10.43.0.1:443`
  reaches `nodeIP:6443`. The `node-proxy` add-on (host-network DaemonSet,
  `packages/node-proxy`, image `ghcr.io/k8flare/node-proxy`) listens there.
  It reads the server URL and CA path from the agent's
  `kubeproxy.kubeconfig` (two hostPath files, no keys), asks
  `/v1-k3s/serving-node-proxy.crt` for a server-CA certificate for
  `kubernetes.default.svc` and `10.43.0.1` with its own ServiceAccount
  token (the endpoint accepts only
  `system:serviceaccount:kube-system:k8flare-node-proxy`), and forwards
  everything to the edge hostname over HTTP/1.1 with no client
  certificate, so a request without a bearer token stays anonymous.
  It skips nodes labelled `k8flare.com/agent` (k8flare-agent binds 6443
  itself). `DISABLE=node-proxy` turns it off.
  - Checked in an OrbStack VM (arm64, stock k3s v1.36.2, local wrangler
    dev + devtls, image preloaded from a local build): the node went
    Ready, the tunnel stayed on one connection for 8+ minutes with no
    resync, CoreDNS and local-path started, a pod reached
    `https://10.43.0.1` and `kubernetes.default.svc` with the SA `ca.crt`
    (bearer tokens passed through, no token stayed anonymous), a watch
    streamed, `kubectl logs` and `exec` worked. Not run: the e2e spec set
    with `AGENT=k3s`, so CI still defaults to k8flare-agent.
  - Unverified: the workflow has not run; the package must be public.
  - A Ready node without a running proxy is still published, so a pod
    hitting `10.43.0.1` can be sent to it and fail once per attempt.
  - A stolen node-proxy ServiceAccount token can obtain a certificate for
    the `kubernetes` names; RBAC on `serviceaccounts/token` in
    `kube-system` is what protects it.
  - The image tag is `latest` with `IfNotPresent`; nodes do not pick up a
    newer image on their own.
- CI still joins k8flare-agent by default; `AGENT=k3s
  scripts/ci/e2e.sh up` joins the stock agent with the node-proxy image
  built and preloaded from `packages/node-proxy`.
- The agent must run with `--disable-apiserver-lb`.
- CI nodes do not authenticate with certificates: `k8flare-agent` sends
  the bearer `node:<name>:<password>` (`packages/agent/main.go`) and joins
  with the raw `JOIN_TOKEN`, so CA pinning from a `K10` token and
  `k8flare token create` are never exercised by a join in CI.
  `control-plane-worker/src/clientcert_test.ts` (the `tlsClientAuth`
  forwarding) is outside the `unit.yml` test glob.
- The stock agent needs `--disable-apiserver-lb`, and README's one-line
  install now passes it (the owner's decision, 2026-10-02). No change here
  could have removed the need: with the load balancer on, the agent's
  supervisor URL is the balancer's own `https://127.0.0.1:6444` from the
  first request (`k3s/pkg/agent/proxy/apiproxy.go`, `NewSupervisorProxy`;
  `pkg/agent/run.go` validates the token through `proxy.SupervisorURL()`),
  the balancer is a TCP passthrough to the server address, and Go sends no
  SNI for an IP literal, so the edge sees a ClientHello without SNI, a
  `Host: 127.0.0.1:6444`, and is asked for a certificate valid for
  `127.0.0.1`. No server response arrives, so the supervisor cannot switch
  it off, and the flag has no environment variable
  (`pkg/cli/cmds/agent.go`, `DisableAgentLBFlag`). Still open: no CI run
  joins a stock agent with that line.
  - `devtls -strict-edge` refuses what the edge refuses (a handshake
    without SNI, a Host outside `-hosts`, IP SANs); CI does not run with it
    yet, and `scripts/ci/e2e.sh` still dials `127.0.0.1:16443`.
  - Read, not run: `publicHost` (`apiserver-supervisor/supervisor.go`)
    echoes the request Host, so through the balancer `/v1-k3s/apiservers`
    would answer `127.0.0.1:6444`.
- Unchecked against Cloudflare: BYO-CA mTLS is Enterprise only, a
  `user_defined` custom certificate from a private CA, and that
  `certRFC9440` is populated for BYO-CA certificates.

## Packaged components

- HelmChart and HelmChartConfig are honoured by a Worker-side controller in
  the addons Worker (`packages/helm`, `/helm` on the addons Worker, run
  from the `k8flare-addons` queue after the manifest deployer; HelmChart
  and HelmChartConfig writes route there from `cluster.ts`). The CRDs ship
  as the `helm-crd` add-on; `DISABLE=helm-controller` turns the controller
  off. It uses Helm's own `pkg/engine`, `chartutil`, `loader` and
  `releaseutil`, so `include`, `tpl`, `required`, `toYaml`, sprig,
  subcharts, `condition`/`tags`, `global`, `.Files`, `.helmignore`,
  `values.schema.json` and `crds/` behave as in `helm install`. The 70 MB
  figure belonged to `pkg/action`, `pkg/kube` and `cli-runtime`, which are
  not linked; the addons Worker is 50.1 MB after wasm-opt with `-tags grpcnotrace` (24.9 MB before), 14 MB
  under the cap; chart and index downloads go through the `OUTBOUND` binding.
  Releases are stored as `sh.helm.release.v1.<name>.v<N>` Secrets in the
  release namespace in Helm's own encoding, so `helm list` and `helm
  history` on a workstation see them; objects carry the
  `meta.helm.sh/release-*` ownership metadata. Verified against fakes and
  `helm template` output, not against a running cluster or workerd. Not
  supported:
  - Hooks are recorded in the release but never run (no pre/post-install
    Jobs, no `helm test`).
  - `spec.failurePolicy` (a retry after a failed first install re-renders as
    an install, but nothing is uninstalled first), `timeout`, `backOffLimit`,
    `jobImage`, `repoCA`, `insecureSkipTLSVerify` and `dockerRegistrySecret`
    are ignored; there is no Job. Objects are always server-side applied
    with force; `serverSide` and `forceConflicts` are ignored.
  - `spec.chart` must be a repo chart name with `spec.repo`, a chart URL,
    `oci://` or `spec.chartContent`; repo aliases such as `stable/x` are
    not resolved. OCI covers anonymous and basic/bearer-token registries,
    not `dockerRegistrySecret` or registry redirects that need extra auth.
  - Changes to Secrets named in `valuesSecrets` do not trigger a run and
    are not part of the config hash; the next spec change or deploy does.
  - `lookup` goes through the addons Worker's API binding and has not been
    run under workerd. Chart `.Capabilities.APIVersions` comes from
    discovery.
  - Deleting a HelmChart uninstalls through a finalizer
    (`wrangler.cattle.io/on-helm-chart-remove`); `crds/` objects are never
    removed, as in Helm. History is trimmed to 10 revisions, never
    dropping the deployed one. A failed apply records a failed revision and
    the queue retries with backoff; retries of the same spec update that
    revision instead of adding new ones.
  - Charts that need `helm dependency build` must be packaged with their
    `charts/` directory; `dependencies` are not fetched.
- Ingress and Gateway API are routed at the edge, covered by Go tests
  only; no workerd or CI run has exercised them.
  - The workloads pass compiles every owned Ingress (IngressClass
    controller `k8flare.com/edge`; the `k8flare` class is packaged as the
    default) and HTTPRoute (GatewayClass with the same controller) into one
    precedence-sorted table at `/k8flare/edge/routes`, rewritten only when
    it changes. The apiserver isolate caches it and the Service and
    endpoint lookups for 2 s, so a routed request costs no store call
    while the cache is fresh and a route change reaches the edge within
    the queue delay plus 2 s.
  - Ingress: host (exact, `*.` wildcard, none), Exact/Prefix/
    ImplementationSpecific paths, `defaultBackend`, Service ports by name
    or number. `resource` backends answer 500. `tls` entries are ignored:
    Cloudflare terminates TLS with its own certificate for the zone, and a
    `tls.secretName` is never read. `status.loadBalancer.ingress` lists the
    rule hosts and stays empty for hostless rules: no CNAME target for
    custom hosts is documented or configured anywhere yet.
  - Gateway API: GatewayClass, Gateway (HTTP and HTTPS listeners,
    `allowedRoutes` `Same`/`All`, `sectionName`, listener hostname
    intersection), HTTPRoute matches (path, headers, query, method),
    weighted `backendRefs`, ReferenceGrant for cross-namespace backends,
    and the RequestHeaderModifier, ResponseHeaderModifier, RequestRedirect
    and URLRewrite filters. Accepted, Programmed and ResolvedRefs are
    reported per the spec. Not handled: `allowedRoutes.namespaces.from:
    Selector` (treated as not allowed), RequestMirror, ExtensionRef,
    per-backend filters, GRPCRoute, TLSRoute, listener `port` matching,
    and TLS `certificateRefs` (never read).
  - The Gateway API CRDs are not shipped. `sigs.k8s.io/gateway-api` is not
    in go.mod or the module cache, so there is no local canon, and the
    standard-install CRDs are large embedded data that wasm-opt cannot
    shrink. Install them with `kubectl apply -f` of the release's
    `standard-install.yaml`; whether `customresources` accepts their CEL
    validations is unchecked.
  - Requests that match no rule fall through to the API, so a
    hostless catch-all Ingress does not shadow `k8flare.com`,
    `api.k8flare.com`, `*.workers.dev`, `*.internal`, `localhost`, IP
    literals, `{name}--{namespace}` hosts or the hosts in `API_HOSTS`
    (`wrangler.jsonc`, `TestConfiguredAPIHostIsNeverAGatewayHost`); the
    `/svc/{ns}/{name}` path form applies only to those control-plane
    hosts. `API_HOSTS` is set by hand: the edge certificate hosts are not
    persisted. What a missing entry costs: full Conformance run 36746667748 showed what the gap costs:
    the Ingress API spec creates Ingresses with a `defaultBackend` and no
    class, admission assigns the default `k8flare` class, the edge pass
    compiles a hostless `/` rule, and from 17:15:09 UTC every request on
    the devtls address `127.0.0.1` answered 500 `service not found` ahead
    of audit and the API (3109 watch responses in devtls.log, plus every
    kubectl and kubelet read), including the deletes that would have
    removed the Ingress; 372 specs then timed out.
  - Changes to any Service also run the edge pass (ResolvedRefs depends on
    Services); backends are looked up per ref rather than listed.
- local-path runs as a resident Deployment, not helper pods only.
- CoreDNS NodeHosts is never written.
- Network policy is off: the supervisor config answers `DisableNPC: true`
  (`apiserver-supervisor/supervisor.go`), so the agent skips `netpol.Run`
  (`k3s/pkg/executor/embed/embed.go`). It has been so since fb37662, with
  no reason recorded.
- The manifests bucket is not bound: `addons.ts` reads `MANIFESTS_R2` and
  no wrangler config declares it. An edit to the bucket does not start a
  pass, and `bucket.list()` reads one page.
- `DISABLE` covers manifest names and `helm-controller` only; metrics,
  the Service load balancer and edge routing cannot be turned off.
- The LoadBalancer hostname suffix is the constant `.k8flare.com`
  (`edgehost/host.go`), not the cluster domain.
- Nothing measures what the platform costs a node.

## Operations

- Certificates:
  - CA rotation: `k8flare certificate rotate-ca` (`POST
    /internal/certificate/rotate-ca`, admin only) replaces server-ca and
    client-ca together. The old certificate stays in the bundle that
    `/cacerts`, `/v1-k3s/server-ca.crt` and `/v1-k3s/client-ca.crt` serve (new
    first, expired ones dropped), and the new CA is cross-signed by the old
    one, which every leaf chain carries, so a node that still trusts only the
    old CA accepts new leaves. Edge client-cert auth verifies against the
    bundle. `k8flare certificate check` lists each CA with its expiry (the
    apiserver keeps no record of issued leaves, so it cannot list them).
    Unit tests only, not run against workerd or a real k3s agent.
  - Old CAs are never retired before they expire; a compromised CA cannot be
    dropped from the bundle yet. The service-account signing key is not
    rotated (k3s does that separately).
  - After a rotation the edge certificate and the Cloudflare mTLS client CA
    are still uploaded by hand: rerun `k8flare edge-certificate`. A K10 token
    embeds the hash of the whole `/cacerts` bundle, so tokens printed before a
    rotation no longer validate for new joins, as in k3s.
  - Leaf lifetimes match k3s (leaves 1 year, CAs 10 years). k3s renews a leaf
    inside 90 days of expiry, and only when the agent restarts: the agent
    asks the supervisor for its kubelet, kube-proxy and k3s-controller
    certificates on every start. The supervisor issues a fresh one each time
    and does not check for expiry. The agent's own rotation is not exercised
    here. The apiserver-to-kubelet client cert is now valid for 24 hours and
    node-tunnel refetches it (and the CA bundle) hourly into the running
    instance without dropping the tunnel; the TypeScript refresh is not
    covered by a test.
- Snapshots: scheduled ones (`SNAPSHOT_INTERVAL_HOURS`, default 12, 0 disables;
  `SNAPSHOT_RETENTION`, default 5) are tested against a fake R2 only, not
  against workerd.
- Cluster store: compaction keeps 5 to 10 minutes of history with a
  100000-revision cap; the cluster-store tests run the Durable Object on
  node:sqlite, not workerd.
- Audit log: the front records API, edge proxy and supervisor requests
  under `AUDIT_POLICY` (default: `DefaultAuditPolicy`), unit tests only.
  Edge and supervisor routes retain their existing authentication and
  authorization. Failed API responses include Status fields; forwarded
  JSON/protobuf Status bodies are decoded up to 1 MiB, with larger or
  non-Status errors retaining only the HTTP code. Authentication failure
  details survive in `k8flare.com/authentication-failure`. Not yet:
  - Request and RequestResponse levels behave as Metadata; bodies and
    upstream's `omitManagedFields` need the group workers, which run the
    REST handlers, to log objects.
  - Not run in a deployed Worker: `tail()` must relay the lines to
    Workers Logs.
- Upgrades: storage migration follows upstream's default at 1.36 (the
  `storagemigration.k8s.io` API and its controller are off by default;
  the deployed binary re-encodes an object the next time it is written),
  plus an in-place sweep in the shape of the StorageVersionMigrator: the
  Cluster DO's deploy stamp (`addons_seed`) starts the addons pass, which
  calls `POST /internal/storage-migrate` on the front; it compares the
  storage version hash (`discovery.StorageVersionHash`) of each stored
  resource with the record at `/k8flare/storageversions`, decodes and
  re-encodes every object of a resource whose hash changed, writes only the
  objects whose bytes differ (CAS on the revision, conflicts skipped), and
  resumes across passes from a per-resource cursor at 200 rewrites per
  pass. `GET /internal/storage-migrate/status` lists pending and failed
  resources. Not covered: an undecodable object marks its resource failed
  and the sweep does not retry it until the key is deleted or the hash
  changes again; a stale Secret encryption key is not rewritten by the
  sweep (`k8flare secrets-encrypt reencrypt` does that); objects written by
  the old version during a gradual rollout after the sweep passed their key
  stay at the old encoding until their next write or the next deploy; the
  deploy trigger is exercised against node:sqlite, not workerd.
- Gradual rollout is not implemented: nothing calls `wrangler versions`,
  a deploy is one cutover. Nothing tests that nodes keep running across one.
- `/livez` is ping only (`apiserver/health.go`); `/readyz` and `/healthz`
  carry the datastore, queue and controller checks.
- Snapshots: the Durable Object side of `save`, `list`, `restore` and
  `--force`, and the `/vault/` exclusion, have no test (the CLI and the
  front's forwarding do). Point-in-time restore rewinds the whole Durable
  Object, `/vault/` included. Snapshots go to `PODS_R2`, the bucket pods
  use. No Logpush is configured.
- Bootstrap-token Secrets now authenticate API bearer requests and k3s agent
  joins through the upstream bootstrap authenticator. Unit tests cover validation
  and supervisor access; a Secret-backed join has not been exercised with a live
  k3s agent under workerd. `k8flare token` continues to manage vault-backed tokens.

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
  - A component token is an HMAC of `ADMIN_TOKEN`
    (`control-plane-worker/src/componenttoken.ts`), so the identities share
    one secret. node-tunnel, the metrics entrypoint and the nodes client
    send `ADMIN_TOKEN` itself.
- Rate limiting is max-in-flight only (`MAX_REQUESTS_INFLIGHT`,
  `MAX_MUTATING_REQUESTS_INFLIGHT`, 400 and 200, watches and streams
  exempt, `system:masters` served when full). The count is per isolate,
  not cluster-wide. API Priority and Fairness does not build against the
  lean client-go (no FlowControl informers) and its controller needs
  goroutines and informers the Workers do not keep; no Workers Rate
  Limiting binding is wired.
- Admission order: LimitRanger, ServiceAccount, Priority and the other
  built-ins of the `admit` phase run before the mutating webhooks and are
  not run again in the validate phase (`admission/chain.go`), so what a
  webhook changes is not rechecked by them. PodSecurity and NodeRestriction
  run in the validate phase.
- Admission: every default-on upstream plugin that acts under upstream's
  default feature gates is present. Default-on but inert here, so not
  added: ClusterTrustBundleAttest (needs the `ClusterTrustBundle` gate, off
  by default, and no `clustertrustbundles` resource is served) and
  PodGroupProtection, PodGroupWorkloadExists and JobValidation (need the
  alpha `GenericWorkload` / `WorkloadWithJob` gates, off by default).
  OwnerReferencesPermissionEnforcement and DenyServiceExternalIPs are in
  upstream's `DefaultOffAdmissionPlugins`, and k3s enables only
  `NodeRestriction`, so they are intentionally not
  enabled. Serving ClusterTrustBundles would need the attest check added.

## Cloudflare features

- Unit tests only (the `Hooks` entrypoint serves only `hookecho`, so no
  dev or CI run can serve an aggregated API from a Worker yet). An
  `APIService` points at a Worker with the `k8flare.com/worker`
  annotation (the same key admission webhooks use); `spec.service` is not
  needed (README's `https://k8flare.com/worker/<name>` form has no field to
  live in: `APIService` has no `url`). Requests reach the Worker as `https://hooks.internal/hook/<name>`
  followed by the original API path; the requesting user travels in
  `X-Remote-*` headers.
  - Aggregated discovery fetches the remote's `/apis` (aggregated v2, or the
    legacy `/apis/<group>/<version>` list when unsupported) as
    `system:kube-aggregator`, caches it for a minute per APIService in the
    apiregistration Worker, and marks it stale when the fetch fails. The
    cache is per isolate and per APIService (upstream shares one per
    service), and group/version priorities are not carried over.
  - Availability of a Worker APIService is always `True`; the Worker is not
    probed.
- The same limit holds for Workers as controllers and as admission or
  conversion webhooks: `Hooks` (`control-plane-worker/src/hooks.ts`) loads
  `hookecho` whatever the name, and `hookecho` answers three echo names.
  A conversion webhook is matched by URL only, not by the annotation.
  `TestAdmissionExtensions`, which drives the delivery, is skipped in
  `unit.yml`.
- No exec credential plugin ships; `k8flare` only accepts the Access token
  as a bearer.
- Traces start in the workloads and queue paths only (`otel.ts`), not per
  API request, and no config sets `OTEL_ENDPOINT`.
- The LoadBalancer hostname and the edge proxy origin are fixed to
  `k8flare.com` (see Packaged components).

## Conformance

- Conformance does not run on every change: `conformance.yml` runs on a
  daily cron and on dispatch; push and pull request run the 21 required
  specs.
- The nightly run is green whatever fails: the cron runs the default
  branch, and main's `scripts/e2e/main.go` exits non-zero for the
  `required` set only. Run 36839051306 (65cd7e8) ended success with
  `101 Passed | 345 Failed`. This branch fails the job for the set it was
  asked to run.
- Production (`k8flare.kooffice.workers.dev`, account KOOFFICE) runs
  b9fff9a plus the observability change since 2026-10-01T17:49Z, deployed
  with `cf` from `packages/control-plane-worker` (branch `work/deploy`,
  cd3bb2c; version d2093b7b, the one before is 9636ffbb of 09-24). No
  container application is declared, so pods on Cloudflare cannot start.
  `/readyz` passes, the old data reads, a namespace and a Secret write
  work. What a quarter of an hour of `wrangler tail` showed, with no node
  and no client:
  - The workloads pass never succeeded: its lists of clusterroles, roles
    and rolebindings ended in `bridge: fetch timed out kind=hold age=30s`
    and the pass was sent again about every 30s, a request a second on the
    Cluster Durable Object. Two causes, neither visible on local workerd
    where a store call takes a millisecond. Every load of a group worker
    into a new isolate ran its bootstrap, 149 sequential creates for RBAC
    (a cold `get roles` took 7.9s, then 5.8s on the next isolate). And the
    pass opened all 36 lists at once, while Cloudflare lets one invocation
    wait on six responses and queues the rest. Fixed in 11e92a4..9c798f1:
    a bootstrap records a marker and costs one read afterwards, and a pass
    holds six lists open. Deployed 2026-10-01T19:3xZ (version 18c4c0fa):
    a cold `get roles` is about 3s, and the pass completes
    (`workloads: clusterroles=73 ...`, no timeout).
  - With the pass working, it ran again every 6s for ever (27 to 31
    requests per 30s on the Cluster Durable Object), and every 3s on a
    local stack with no node. A pass booked its own successor, the
    follow-up re-sent the same `changed` set, and so on. Two rules did it:
    a pass that saw `pods` change asked for another in 2s for the
    StatefulSet controller whether or not a StatefulSet existed; and the
    EndpointSlice controller, built anew for each pass, takes every
    existing slice it is handed as a change it did not make
    (`onEndpointSliceAdd`, `ShouldSync` on an empty tracker) and queues the
    Service again after `endpointSliceChangeMinSyncDelay`, 1s, which the
    pass reported as a deadline. One Service with a selector was enough,
    and kube-dns is always there. Fixed: the 2s re-run needs a StatefulSet
    that has not settled, and EndpointSlice events delivered by a pass
    book nothing, since that pass syncs the Service itself.
    `TestSyncDoesNotRebookAPodChangeWhenNothingIsDue`. On the local stack
    with no node: 5 passes in the first 100s, none in the next 180s.
    Not fixed, same shape: the Job controller queues a Job 1s after every
    pod event of a pod it owns (`enqueueSyncJobBatched`), so a pod of a
    Job, finished or not, keeps the pass running; `replicaGap` books 5s
    for as long as a ReplicaSet or Deployment has `status.replicas` short
    of `spec.replicas`. The 15s metrics message is gone since 1ebcd82 and
    3337b7e: an HPA pass scrapes, and with an HPA in the cluster books the
    next one 15s out (upstream's sync period) in one `deadlines` slot of
    the Cluster object; with none it books nothing.
  - Deployed as 7cad8d15 (2026-10-02). With workloads and every other
    queue but three delivering, a Deployment created with kubectl got its
    ReplicaSet and pod about 30s later and the Cluster Durable Object then
    took no request for minutes (per 30s: 21, 4, 23, 1, 0, 0, 0, 0, 0, 0,
    25, 1, 0, 0; the late burst is the 5-minute alarm). Left paused:
    `k8flare-scheduler`, `k8flare-metrics`, `k8flare-hpa`. The scheduler
    retries unschedulable pods on a timer by design (`RetryDelaySeconds`,
    1s doubling to 60s, then every 60s), and with no node the two add-on
    pods are unschedulable for good: with it delivering the object took 30
    requests per 30s without pause (`scheduler: bound=0 unschedulable=2
    attempt=16`). Upstream moves such pods back on cluster events and
    flushes them every 5 minutes. Metrics is sent every 15s.
  - Conformance on 3b8bfa4, three runs: 446, 445, 445 of 446. The two
    failures are different specs and neither is a controller that missed
    a pass: DNS for Subdomain failed reading the prober pod's log
    (`bridge: no response headers` after 30s on `pods/.../log`), and
    CustomResourceFieldSelectors timed out after 30s waiting for its
    watch events. Both are the load-dependent kind seen before. main moved
    to 3b8bfa4.
  - Review of the bootstrap marker (Codex, 2026-10-02) found two gaps that
    are older than the marker and are still open. A bootstrap only
    creates: an object that exists keeps its old content when a release
    changes the default, and the new hash is recorded all the same
    (upstream reconciles the RBAC defaults at every start,
    `rbac/bootstrap-roles`). And a bootstrap that fails part-way is not
    tried again in that isolate, since it runs under a `sync.Once` that
    drops the error; the next isolate tries again.
  - The switch that keeps EndpointSlice events from booking a pass is one
    counter for the whole pass, not per goroutine. On js/wasm handlers run
    without yielding, so nothing else can book while it is set; in host
    tests another controller's `AddAfter` in that window would be dropped.
  - The live tests failed in CI after the list bound (8 and 11 of 16,
    `Network connection lost` on an early write). Six lists at a time load
    the group workers one after another, so the first pass ends about 9s
    later than it did (`apiserver-rbac` at age 10 to 13s, was 4s), and a
    request that arrives while a module is instantiated in the single
    local isolate is dropped. The harness now waits for the first pass
    (the root CA publisher's annotation on `kube-root-ca.crt`), which also
    keeps that update out of the tests' watches.
  - The metrics pass runs every 15 to 20s with no node, and sends to
    `k8flare-hpa`, which has no consumer in this deployment.
  - The Cluster alarm re-arms every five minutes for compaction whether or
    not anything was written.
  - Delivery is paused on all twelve queues (`wrangler queues
    pause-delivery`), so nothing runs in the background and controllers do
    not act; four minutes then showed one event, the alarm.
  - `k8flare.kooffice.jp` answers 302 to the Access login for a request
    that carries only `Authorization: Bearer`, so `kubectl` with a token
    from `k8flare access-credential` does not get through Access as
    configured.
- Nothing runs the suite against a Cloudflare deployment; every run is
  local workerd in GitHub Actions. `make deploycheck` is a manual probe.
  Issue #4 was filed by `prod-probe.yml`, which exists only at `old-main`.
- There is no release, no tag but `old-main`, and no branch protection, so
  nothing gates a release on the suite.
- The first full pass is 36857951122 (446 of 446, eight shards, two nodes);
  the same tree gave 445 twice.
- CI gates on 21 required specs on one node. The Conformance job runs
  `scripts/ci/e2e.sh up` with `NODES=2`: the host agent joins first, then
  each extra node is a privileged `rancher/k3s` container of the same
  version (`--tmpfs /run --tmpfs /var/run`, the k3d shape) on the default
  Docker bridge, reaching devtls at `host.docker.internal:16443` (devtls now
  listens on every interface and has that name as a SAN). With the default
  `AGENT=k8flare` the container runs the CI-built `k8flare-agent` with the
  host's unpacked `/var/lib/rancher/k3s/data` mounted read-only (it needs
  `data/current/bin`) and `SSL_CERT_FILE` pointing at the devtls CA; with
  `AGENT=k3s` it runs the image's `k3s agent` with the host's
  `agent/images` (the node-proxy tar) mounted. Each node has its own
  network namespace, so kubelet 10250, flannel VXLAN 8472 and NodePorts do
  not collide; the host reaches the container's node IP over `docker0`
  and the container reaches the host's over its default route. `up` waits
  until `NODES` nodes are Ready, not NetworkUnavailable, schedulable and
  untainted before the write check. Run 36807713386 (4208926) checked the
  shape: both nodes went Ready, flannel VXLAN carried a pod on the host
  node to a ClusterIP backed by pods on the container node, and the
  host's own NodePort answered, but the container node's NodePort
  (`172.17.0.2:30000`) timed out from a host-node pod: Docker's FORWARD
  rules drop new connections into `docker0` that no published port
  accepts. `up` now accepts forwarded traffic to `docker0` from
  `DOCKER-USER` before the container nodes join; not yet re-run.
- First full run (run 36698321866, main at 005a405, one node): 290 of 446
  specs ran before the job was interrupted at 76 minutes (cause not yet
  found; the job timeout is 355 minutes): 255 passed, 35 failed.
  - DNS: the four `[sig-network] DNS` specs. Two causes. No CoreDNS pod
    ever existed (`kube-system` had no pods for the whole run): its
    Deployment asks for `system-cluster-critical`, the pod was refused
    with "no PriorityClass ... was found", because the system priority
    classes are only created on the first request that reaches the
    scheduling group's Worker. The Priority admission now resolves the
    two system classes itself. Separately, creating a headless Service
    failed with `spec.clusterIPs: Required value` (`ClusterIPs` was never
    set to `[None]`), which broke "for services" and "for pods for
    Subdomain" before DNS mattered; fixed in `clusterip.go`.
  - Services and proxying: NodePort, session affinity (3), multiport,
    ClusterIP/NodePort to ExternalName and the kubectl guestbook all reach
    the Service by name from an exec pod (`nc ... getaddrinfo: Try
    again`), so they follow CoreDNS. Not yet re-run with CoreDNS up;
    whether the node's CoreDNS can list Services through kube-proxy to
    `10.43.0.1:443` is unverified.
  - Proxy through a service and a pod: 1 of 320 requests returned 502
    "sync from client". The remotedialer client lists its live connection
    IDs every 60 s and the server closes any it does not list, so a
    connection dialed while the list is in flight is killed. The mirror
    now closes a connection only when two consecutive syncs miss it. No
    unit test: the state is unexported in the mirror.
  - Endpoints latency: 8 s to 60 s per Service under 200 creations (median
    28 s against a 20 s limit) with the same cluster taking 30 s to turn a
    Deployment into a pod. Load-dependent on `wrangler dev --local` with
    four spec processes (see `plans/remaining.md`); not addressed.
  - Addon deploy failed on the Helm CRD: server-side apply of
    `helmcharts.helm.cattle.io` returned "no authorizer provided, unable to
    authorize a create on update", so `addons: ok=false` and the queue
    retried. Fixed (see below); not yet re-run against the addons deployer.
  - StatefulSet: five specs fail in BeforeEach.
  - EndpointSliceMirroring, Events API lifecycle, Job
    backoffLimitPerIndex, OrderedNamespaceDeletion, pod generation.
  - AdmissionWebhook (deny attaching pod, mutate pod with defaults),
    Aggregator sample API server, ServiceAccountIssuerDiscovery.
  - Storage: CSI PV/PVC lifecycle, VolumeAttributesClass lifecycle.
  - "at least two untainted nodes": the Conformance job now joins a
    second node in a container (`NODES=2`, above); not yet run.
- Fixed from that run, each with a unit test but not yet re-run in CI (start
  with the workflow_dispatch `focus` input):
  - StatefulSet (5 specs): a headless Service created with only
    `clusterIP: None` failed upstream validation with `spec.clusterIPs:
    Required value`; the create strategy now fills `clusterIPs` first.
  - Events API: `reportingController=` on events.k8s.io was converted to
    `reportingComponent` and looked up in events.k8s.io JSON, which has no
    such key; the selector now maps back to the events.k8s.io names.
    The patch, update and delete steps of that spec never ran in CI.
  - OrderedNamespaceDeletion: the deleter never called upstream's
    condition update while a pod with a finalizer remained, so
    `NamespaceDeletionContentFailure` never appeared; the conditions are
    now published while pods remain.
  - Pod generation: DefaultTolerationSeconds skipped UPDATE, so replacing
    `spec.tolerations` dropped the defaulted ones and validation refused it.
    The later steps of that spec (generation bumps, observedGeneration)
    were not reached in CI.
  - CRD discovery (run 36784977844 attempt 1, `required (0)`): `/apis`
    answered 200 without `apiextensions.k8s.io` after 30.4 s. The front
    took the whole group list, `apiextensions.k8s.io` included, from the
    customresources Worker's `/apis` and dropped all of it when that call
    failed; here the CRD Worker answered in 134 ms (`crd-diag`) but the
    front's fetch promise never resolved (`bridge: fetch timed out ...
    binding=CUSTOMRESOURCES ... GET /apis`, a `STORAGE GET /kv` timed out
    in the same second). The front now serves `apiextensions.k8s.io/v1`
    from its own table in both the APIGroupList and the aggregated
    document and only takes CRD groups from the Worker. The 30 s stall
    of every fetch in one dispatch window is not explained.
- Not fixed:
  - Job backoffLimitPerIndex: the run made 10 pods where 6 are correct
    (indexes 0 and 2 ran twice, index 1 four times, Failed=6). The job
    controller counts an index as done from `status.completedIndexes` or a
    pod that still has the tracking finalizer, so it acted on a Job status
    older than its pods. A passing fake-client test over repeated Sync
    passes shows the controller logic is right when the state is
    consistent. Suspects: the Job snapshot is not updated by the
    controller's own status writes while the pod snapshot is re-listed live
    after a failed pod create (`catchUp`), and an overlapping pass on
    another isolate. Needs the workloads log of a rerun with `focus`.
  - EndpointSliceMirroring: not the 12 s window. See the focused run below.
- Focused run 36716240747 (65cd7e8, 16 of 20 passed). The four failures
  share two causes, found in `dev.log` (a workloads line reading
  `workloads:  drained=false` with nothing between the spaces is a busy
  answer: empty objects, `syncLockWait` = 10 s, matching the 10.1-10.6 s
  batch durations):
  - VolumeAttributesClass, EndpointSliceMirroring, PV/PVC lifecycle: every
    workloads batch inside each spec's window was turned away as busy. One
    pass listed at 13:03:49.6 and drained until 13:05:23.8 (1m33s) while
    the Endpoints write (13:04:15.4), the VAC DeleteCollection (13:03:49.8)
    and the PVC delete (13:04:38.5) waited. The finalizer releases
    (`releaseVolumeAttributesClasses`, `releaseStorageProtection`) sat
    behind that lock although they only list and update; they now run
    before it (conflicts tolerated). A busy sync now also asks the running
    pass to yield: `drain` stops waiting for queued work once another sync
    has waited and the pass has run for `yieldGrace`, still waiting for
    in-flight calls. The mirroring controller still needs a pass, so it
    depends on the yield and on the shorter passes below.
  - Long passes: 26 controllers and 200+ `ss2` status updates by the
    controller-manager in 95 s. The StatefulSet controller's status write
    conflicted forever: its snapshot copy of the StatefulSet kept the
    resourceVersion from the pass start, the update retry re-reads the
    lister (the same stale copy), and the requeue repeats until the pass
    ends (`Error syncing StatefulSet ... the object has been modified`).
    The same loop explains the StatefulSet scale spec: the first status
    write was made before ss-0 existed (`status.replicas=0`) and every
    later one conflicted, so `GET /scale` returned `status.replicas=0`
    while the pod was Ready. Status and spec writes by the StatefulSet
    controller now enter its snapshot (`observeStatefulSets`).
  - Write observers now cover every controller-owned type with a snapshot
    (`observe.go`, `observe_types.go`): Jobs, CronJobs, ReplicaSets,
    Deployments, DaemonSets, ControllerRevisions, Endpoints,
    EndpointSlices, ResourceQuotas (status), Namespaces (update, status,
    finalize) and PodDisruptionBudgets (status), on top of pods, RCs,
    StatefulSets, PVs and PVCs. Create, update, status update and patch go
    through `observed` into the snapshot, delete through `removed`; pod and
    StatefulSet patches were added too (the Job controller removes tracking
    finalizers with `Pods().Patch`, which the pod snapshot never saw).
    `TestControllerWritesEnterTheSnapshotWithinThePass` covers each verb.
    A fake-client Sync of an indexed Job with `backoffLimitPerIndex`
    (`TestSyncRunsEachFailingIndexOnce`) creates exactly 3 pods, but it
    also passes with the observers off: a fake pass is too short to
    requeue the same Job against a stale snapshot, so the 10-pods run
    stays unconfirmed until a CI rerun of that spec.
  - Not observed: Services, ConfigMaps, Secrets, ServiceAccounts, Nodes
    and the other snapshot types whose controllers write them (root CA
    publisher, nodelifecycle, service-account controllers); no conflict
    loop was seen there.
  - The interruption at 76 minutes: GitHub reports the step as cancelled
    with only "The operation was canceled." (no runner shutdown, timeout
    or out-of-memory message), the job's later `always()` steps were
    skipped, and the run's actor was the dispatching user. Disk had 106 GB
    free at the start. The logs artifact was never uploaded, so nothing
    more is known; `scripts/ci/e2e.sh test` now prints memory, disk and
    workerd RSS every minute so the next occurrence leaves evidence in the
    step log.
  - Root causes found for the API-machinery, auth and storage failures:
    - AdmissionWebhook deny attaching pod: kubectl tries WebSocket, any
      bad handshake makes client-go fall back to SPDY, and the front
      answered SPDY with 426 before admission ran, so the denial was never
      reported. The front now runs the stream-locate (authn, authz,
      admission) for a non-WebSocket upgrade and returns its denial;
      an allowed connect still gets the 426.
    - AdmissionWebhook mutate pod with defaults: the remote admission
      plugin decoded the mutated object without defaulting it (upstream
      calls `GetObjectDefaulter().Default` after the patch,
      `webhook/mutating/dispatcher.go`), so an added init container failed
      validation on imagePullPolicy and terminationMessagePolicy.
    - Aggregator: `extension-apiserver-authentication` stored the
      request-header lists as bare strings; kube-apiserver stores JSON
      arrays and every extension apiserver `json.Unmarshal`s them in
      `RunOnce` at start, so the sample apiserver exited (restart count 5).
      Existing clusters kept the old ConfigMap because it was only created
      when missing. The aggregator dialed the extension over the node
      tunnel without a client certificate, so request-header
      authentication of the proxied user could not work. Now the vault has
      a `request-header-ca` (rotated with the others), `/internal/proxy-client`
      (admin only) issues a `system:auth-proxy` client certificate from it,
      node-tunnel fetches it next to the kubelet client certificate and
      presents it on every `X-Dial-TLS` dial, and the ConfigMap publishes
      the request-header CA and `requestheader-allowed-names`
      `["system:auth-proxy"]`. The ConfigMap now reconciles at core-worker
      startup and kube-system provisioning, publishes the client CA bundle
      as `client-ca-file`, and repairs stale server-CA and bare-string
      entries. It merges other writers' CA bundles and header lists and
      skips unchanged writes; unit tests cover migration and CA rotation.
      Open: the sample-apiserver spec was not re-run.
    - Create on update (server-side apply or PATCH of a missing object): the
      CRD group in `customresources` had no `Authorizer` in its
      `APIGroupVersion`, and the custom-resource handler was given an
      allow-all authorizer, so a create by apply was either an internal
      error or unchecked. Both now use `authz.New` (privileged group, node,
      RBAC), as the group workers already did; apply-create needs the
      `create` verb. It adds about 3.9 MB to customresources (59.65 MB to
      63.53 MB after wasm-opt, 3.5 MB below the cap).
    - VolumeAttributesClass lifecycle: writes under
      `/registry/volumeattributesclasses/` were not routed to the workloads
      queue, so the `vac-protection` finalizer was never released after a
      delete. The PVC lifecycle spec has the same symptom (finalizer not
      released within 30 s) but its key is routed; the cause is not found
      (suspect: a busy or slow workloads consumer under load).
    - ServiceAccountIssuerDiscovery: the discovery and JWKS handlers, the
      bootstrap `system:service-account-issuer-discovery` role and the
      binding work; the pod failed on the DNS lookup of
      `kubernetes.default.svc.cluster.local` (both the in-cluster and the
      fallback path), which is the DNS failure above.
    - `FailedMount ... kube-api-access ... failed to sync configmap cache`
      is not TokenRequest: it is the kubelet's 1 s wait for the
      `kube-root-ca.crt` reflector to sync (`watch_based_manager.go`),
      four occurrences in the run, each retried within seconds.
  - Run 36743032029: three Job specs could not create a Job whose template
    has `securityContext.privileged: true` ("disallowed by cluster
    policy"). Upstream validation reads `capabilities.Get().AllowPrivileged`,
    which kube-apiserver sets from `--allow-privileged`
    (`capabilities.Setup`, `cmd/kube-apiserver/app/server.go`) and k3s
    passes as `true` (`pkg/daemons/control/server.go`); no worker ever
    called it, so the default `false` applied to every upstream strategy.
    `apiserver-registry` now runs the same setup for all of them.
    `ServiceAccounts should mount an API token into pods` failed on the
    TokenReview groups: upstream never adds `system:authenticated` inside
    an authenticator; `group.NewAuthenticatedGroupAdder` wraps the whole
    chain (`pkg/kubeapiserver/authenticator/config.go`) and TokenReview
    goes through that same request authenticator. Here each authenticator
    appended the group itself except the service-account one. The
    apiserver's request chain now has the upstream adder and both token
    unions (apiserver, group workers' TokenReview) get the same rule.
- Runs 36698321866 (76 min) and 36721106686 (95 min, 322 passed / 27
  failed of 349) were interrupted by the 16 GB runner running out of
  memory: `/dev/shm` (the Durable Object state, `STATE` in
  `scripts/ci/e2e.sh`) grew 0.15 GB to 2.9 GB and workerd's RSS 7.2 GB to
  10.9 GB before `mem_available_mb` reached 242. The `/dev/shm` growth is
  wrangler's local observability trace store
  (`v3/observability/miniflare-wobs-trace-store`, one SQLite DO that
  records every span and log of every invocation and never prunes: its
  only delete is a `clear` RPC nothing calls). `wrangler.dev.jsonc`
  setting `observability.enabled: false` never turned it off: wrangler
  4.131 enables the local collector from the `X_LOCAL_OBSERVABILITY`
  environment variable (default true), not from the config. A local
  checkout that ran with that config for a day had a 3.85 GB trace store
  next to a 3 MB Cluster DO. Measured with 10 min of kubectl churn
  (ConfigMap create/label/delete plus an unschedulable Pod per round):
  default, 356 rounds, trace store 4.7 MB to 447 MB, Cluster DO 0.6 MB
  to 14.5 MB; `X_LOCAL_OBSERVABILITY=false`, 593 rounds in the same
  10 min, no trace store, Cluster DO 0.3 MB to 23.7 MB. The Cluster DO
  is bounded: compaction leaves 47 % of its pages on the freelist
  (1975 of 4234) and SQLite reuses them, so the file stops growing at
  about ten minutes of writes. `make dev` and `e2e.sh up` now start
  wrangler with `X_LOCAL_OBSERVABILITY=false`, and the per-minute
  sampler prints each `STATE/v3/*` subtree size. Not explained: the
  workerd RSS. Locally it grows about the same with the collector off
  (user workerd 1.7 GB to 2.1 GB in 10 min, noisy) and the dynamic
  workers are loaded once each (the Worker Loader key is
  `name@sha256@isolate`, `loaded=` stops at the number of distinct
  workers), so it is neither the trace store nor isolates piling up; the
  Go heaps inside the wasm workers, which never shrink, are the next
  suspect and there is no memstats endpoint to read them.
- Run 36743032029 (c860062, 43 specs focused, 10 failed): the slow
  convergence behind EndpointSliceMirroring, Service endpoints latency,
  the Deployment lifecycle and the StatefulSet scale subresource has one
  shape, read from `dev.log`, `procs.log` and the audit events.
  - workerd's main thread ran at 100 % (`main_ms` 1030-1040 per 1 s
    sample) for every minute of the run. Every isolate shares it, so a
    controller write cost 1-5 s under the endpoints latency spec (audit
    p50 0.13 s, p90 1.8 s, max 12 s) and a pass's lists and build+fill
    took 1-6 s each.
  - A pass listed its sources once and its snapshot then froze: a write
    landing after the lists waited for the pass (30 s to 2m43s under
    load, drain held open by in-flight handlers doing sequential slow
    writes, `endpoint_slice=154/5` queued at one give-up), then the
    follow-up pass, then the next pass's lists. The mirroring spec's
    Endpoints update (16:30:13.5) fell in a pass listed at 16:30:11; the
    next lists ran at 16:30:44, 31 s later, against a 12 s window. The
    Deployment patch (16:34:12) reached the ReplicaSet controller at
    16:37:25, and each rollout step needed a pass of its own. The
    StatefulSet's status write with `replicas=1` was still in flight when
    the spec read `/scale` 11 s after the pod became Ready, and after the
    spec's own update the controller's status writes conflicted five
    times because its snapshot never saw the external write.
  - Every busy consume waited 10 s for the lock and was turned away; the
    yield could not help because a handler was always in flight.
  - The garbage collector ran 533 collects in 24 minutes, one every 2.7 s,
    each relisting the whole registry, because every modification of an
    object with ownerReferences (pod status patches, ReplicaSet status,
    EndpointSlice rewrites) routed to its queue.
  - 44 of the 200 endpoints latency trials failed with `ipaddress X is
    already allocated`: parallel Service creates all picked the lowest
    free address and the losers returned the conflict (error ratio 0.22,
    the spec allows 0.05).
  - Fixed: the pass now follows the Cluster DO's watch from the list
    revision and notes every decoded registry write into the snapshot
    informer of its type (`live.go`), so a controller sees a write within
    the pass it lands in; with the feed live the follow-up pass and the
    yield are skipped, since the running pass sees what a new one would
    list. The gc queue hears about creates, deletes, owner changes and
    finalized deletions only. A Service whose ClusterIP was picked here
    retries from a fresh pick after a collision, and the pick starts at a
    random offset. Unit tests cover each; the local dev stack measured
    the write-to-result latencies below.
  - Measured on the local dev stack (this Mac, no node, the front and
    the workloads and core group workers rebuilt): a custom Endpoints is
    mirrored 1.5 s after the create and 1.4 s after the update, a
    selector Service has its Endpoints 1.2-1.6 s after the create, and
    with 40 Services being created in parallel the Endpoints update is
    mirrored 0.6 s after the patch while the running pass applied 117
    and 125 feed events instead of waiting for the next lists; no busy
    answer and no drain give-up in 23 passes. Not measured here: the
    CI runner, where the same passes ran on a saturated thread. Still
    open there: the write cost itself (1-5 s per controller write at
    100 % main-thread CPU), the `abortingBody.Close` hop that every
    completed fetch pays through `window.Run`, and a pass that ends with
    rate-limited requeues pending still relists on the retry instead of
    waiting for them.
- Run 36754088851 (c7fef11, focused): two specs left, both root-caused
  and fixed with unit tests; the aggregator one was reproduced on the
  dev stack (OrbStack node, sample-apiserver 1.29.2, `-v=6`).
  - Job podFailurePolicy ignoring DisruptionTarget: the eviction
    subresource added the condition through the `pods` store, whose
    update strategy (upstream `pod.Strategy` and `podUpdateStrategy`)
    keeps the old status, so nothing was persisted; the kubelet later
    reported the pod Failed without the condition, the policy did not
    match, three counted failures exceeded `backoffLimit: 2` and the Job
    failed (`SuccessfulDelete` of the replacement pod at 18:25:18). The
    condition is now written through a status-strategy copy of the
    store, as upstream's `EvictionREST` does with its `statusStore`.
  - Aggregator: every proxied `flunders` request got 403 from the
    sample apiserver (audit: `decision: allow` at the front, `code: 403`
    from the extension; the `Available: True` in the dump is computed at
    read time and means only that endpoints existed). The extension's
    log showed the proxied user as `admin` with groups
    `["system:masters, system:authenticated", "system:authenticated"]`:
    the apiregistration worker adds one `X-Remote-Group` line per group,
    the Workers fetch hop into the node-tunnel binding merges repeated
    lines into one comma-joined value, and the tunnel wrote that line to
    the extension, whose request-header authenticator takes each line as
    one group. Its SubjectAccessReview for that group was denied. The
    tunnel now splits the joined value into one line per group before
    writing the request (the front already splits it when reading). The
    request-header CA, the `system:auth-proxy` client certificate and the
    ConfigMap were all correct in the run (the extension started without
    restarts and read the ConfigMap). After the fix the same request on
    the dev stack returns a `FlunderList`. Not changed: `X-Remote-Extra-*`
    values travel the same hop and would merge the same way if a user had
    a multi-valued extra.
  - Seen on the way, not part of the failure: an existing dev cluster
    kept the pre-request-header `extension-apiserver-authentication`
    (server CA, bare-string allowed names), making the sample apiserver
    crash-loop. Now repaired by reconciliation at core-worker startup and
    kube-system provisioning, without deleting the ConfigMap; unit tested.
- Run 36759859775 (c5ec39f, full): two deterministic failures.
  - DRA CRUD `resource.k8s.io/v1 ResourceClaim`: the apply patch of an
    existing claim got `no corresponding type for resource.k8s.io/v1,
    Kind=ResourceClaim` from the server-side apply type converter.
    `genopenapi` skipped `resource.k8s.io/` and `apiregistration.k8s.io/`
    when collecting root models, `genresources` left both packages out of
    the host-side spec closure, and the installer skipped `resource.k8s.io`
    when no Kine client was given, which is how `bakeopenapi` runs. So the
    baked documents had no models for either group and each worker's
    `openapi.json` held only the meta and Scale schemas, while every other
    group's converter was schema-aware. Now both groups are generated,
    baked (`/openapi/v3/apis/resource.k8s.io/v1` and
    `.../apiregistration.k8s.io/v1` exist) and embedded; `DeleteOptions`
    and `WatchEvent` gained the two groups' GVK entries in every worker.
    Cost: +18 KB raw on the resource worker, +11 KB on apiregistration,
    no new functions (the definitions package is linked only into the
    host-side spec builder). `TestApplyPatchOfExistingResourceClaimHasATypedSchema`
    reproduces the run's error with the old `openapi.json`, and
    `TestEveryServedKindHasATypeInTheTypeConverter` checks every served
    kind against the converter on the host.
  - Watchers "should receive events on concurrent watches in same order":
    not a store or client ordering bug. In `dev.log` every one of the 14
    watches the spec had opened by 19:38:39.8 was dialed at the revision of
    the previous event (14009 ... 14048); the producer's next write, `DELETE
    .../watch-8955/configmaps/cm-4`, reached the front at 19:38:39.827
    (devtls start 19:38:39.665) and never produced an audit
    `ResponseComplete`; devtls logged it as 502 at 19:38:49.877 when the
    spec's 10 s wait gave up and cancelled it. The claim was gone by the
    namespace sweep at 19:40:17, so the delete was applied without its
    response ever leaving the core worker. The same window shows other
    core requests taking 4-16 s (`GET .../statefulsets/ss2` 7.7 s, `DELETE
    /api/v1/namespaces/projected-3054` 16.2 s), the scheduler and
    workloads getting `bridge: response body did not finish in time`, and
    the runner at 1.1 GB available with workerd at 11.8 GB RSS. Storage
    level tests now cover the spec's shape: `watchorder.test.ts` opens a
    watch from the revision of every event of a producer stream with
    node-lease progress interleaved and checks all streams match, and
    `TestConcurrentWatchesFromEachRevisionSeeTheSameOrderAcrossRedials`
    does the same against the kine client with sockets that close
    mid-stream. Both pass, so the missing event was a stalled response in
    the core isolate under memory pressure, not a gap in the store, the
    replay window or the redial. Open: why a single unary DELETE in the
    core worker can stall for more than 10 s while neighbouring requests
    complete; the loader drives each request's window every 25 ms, so the
    stall is inside the isolate, not the pump.
- Run 36759859775, where the 8444 s went (14 of the 20 failures were
  timeouts), read from `procs.log`, the audit events and the pass logs.
  Every audit line is printed twice in `dev.log` (223 735 distinct
  requests, 26/s); `wasmcpu` is 0 under wrangler dev, so CPU is attributed
  by request and pass counts.
  - workerd's main thread was at 100 % (`main_ms` 1035 per 1 s sample)
    from 18:50 to the end; RSS 0.9 GB at start, 8.8 GB after ten minutes,
    12.1 GB at the end.
  - The workloads pass: 362 passes, drain 7212 s in total (p50 1.5 s, p90
    100 s, max 164 s), 34 drains gave up at the 160 s limit, 22 passes
    left controllers behind. From 19:00 to 20:40 there were one to four
    passes per ten minutes, each 160 s, and every give-up named
    `root_ca_cert_publisher=N/1`: the publisher was creating
    kube-root-ca.crt in Terminating namespaces (3216 refused creates, one
    worker, one at a time), so a handler was always in flight and the
    drain could not end early. Lists and build+fill cost 0.7 s and 0.4 s
    per pass; the live feed applied 12 602 events.
  - The namespace deleter: 5.5k rounds, each 35 namespaced lists, a get,
    up to 35 delete-collections and a finalize, were 100k of the 224k
    requests. 198 invocations were aborted by the 90 s RPC timeout, so a
    namespace needed a median of 16 rounds and 36 minutes (p90 100 min),
    the first round came a median of 104 s (p90 54 min) after the delete,
    and up to 116 namespaces were Terminating at once. The deleter walked
    its batch one request at a time and, when the batch named any
    namespace, processed only those: its own status writes name the
    namespaces it is already working on, so the listed ones starved. Inside
    a round the isolate issued its next request 7-20 s after the previous
    answer while the server answered in 0.01-0.2 s and the kubelet's
    requests in the same seconds paced at 0.2 s.
  - The pile of Terminating namespaces fed two more storms: the scheduler
    retried binding their pods every pass (4778 `Error scheduling pod`,
    4356 refused bindings and 2050 events for one namespace's nine pods,
    1.6 s per write) and the publisher retries above.
  - Lease checks leaked: every lease-check follow-up sent a new check to
    the queue and the Cluster DO started another chain after each 50 s of
    lease writes, so checks grew linearly from 56 to 2904 per ten minutes
    (11.5k checks, 23k reads, one follow-up and one queue send each).
  - The garbage collector ran 1988 collects (one per 4 s, 600-1400 items)
    with a discovery round trip each; not changed.
  - Fixed: one pending lease check per node, scheduled through the DO by
    both the lease write path and the follow-up (`POST /lease-check`);
    the publisher sees only Active namespaces (`Deps.ActiveNamespaces`);
    the deleter clears eight namespaces at a time under a 60 s budget,
    reports the rest instead of being aborted, and fills its batch from
    the listed terminating namespaces after the hinted ones; every dynamic
    worker prints `mem worker=<name> heap_alloc_mb ... gc_cpu_pct` once a
    minute so the next run says which Go heaps grew.
  - Local dev stack (this Mac, no node; 40 namespaces each with a
    Deployment of 2, a Service and a ConfigMap, deleted at once; before is
    the c5ec39f build): before, 8 of 40 namespaces were gone after 441 s,
    the deleter fetched the same three deleted namespaces on every call
    and never reached the other 35, 6 invocations hit the 90 s timeout, 57
    refused root-CA creates for the 8; after, all 40 were gone in 215 s
    (19 by 120 s), 16 deleter rounds, no timeout, 38 refused creates (one
    per namespace, from the configmap delete the deleter itself makes).
    Requests inside the deleter still paused 4-5 s every few requests on
    the idle Mac while the server answered in 0.01-0.3 s, so the isolate
    stall is not only runner saturation.
  - Seen on the way: upstream's `NewNamespacedResourcesDeleter` calls
    `klog.FlushAndExit` when discovery returns nothing, which ends the
    workloads Go program (`Go program has already exited` on every later
    RPC until the isolate is replaced); it happened on the dev stack when
    the deleter's first discovery got 401 from mismatched assets, not in
    CI.
  - Open: the 7-20 s (CI) and 4-5 s (local) stalls inside an isolate
    between consecutive requests; the scheduler's per-pass retry of pods
    whose bind is refused; the memory the mem lines will attribute.
- Fetches that a prompt callee answered but the Go side never saw (run
  36784977844 attempt 1: `bridge: fetch timed out ... binding=CUSTOMRESOURCES
  inflight=1 pending=0 turn=true: GET /apis` 30 s after `crd-diag` answered
  in 134 ms, with a `STORAGE GET /kv` in the same second; the unary DELETE
  in run 36759859775 that was applied but got no `ResponseComplete`; the
  "30 s stall of every fetch in one dispatch window" above). Root cause in
  the bridge, not the pump or the callee.
  - Mechanism: `currentTurn` was set by the dispatch goroutine for its whole
    life and *restored* when a later dispatch returned, so it named the
    request whose goroutine last entered, not the JS entry actually on the
    stack. On js/wasm every `resume()` runs all goroutines that became
    runnable until they block, so a goroutine of request A woken during
    request B's entry (a channel or mutex handoff, B's setTimeout(0), a
    pump) runs on B's JS stack; once B's dispatch had returned and put
    `currentTurn` back to A, `Owns()` said true and `Run` issued A's fetch
    inline, in B's I/O context. workerd ties a subrequest to the I/O context
    that issued it and cancels it when that request finishes, so the promise
    never settles (the callee may not even receive the request) and A waits
    out the 30 s header timeout. Timers of a finished request are dropped
    the same way (checked with a `setTimeout` in a request that returns at
    once: it never fires).
  - Reproduced under workerd 2026-09-08 with the loader bootstrap and a tiny
    Go handler: `/wait` blocks on a channel and then fetches a binding that
    answers after 500 ms; `/kick` closes the channel. `/direct` completed in
    511 ms, `/wait` printed exactly the CI line (`inflight=1 pending=0
    turn=true`) after 30 s and the echo service never logged the request.
    After the fix `/wait` completes in 502 ms.
  - Fixed: the turn follows the JS entry. `handleRequest` and `pump` tag
    their window on entry and nothing restores a previous one; promise
    callbacks (`settle`), the post-dispatch `setTimeout` and the bound
    stream/abort/websocket callbacks tag the window whose I/O context created
    them. A goroutine that runs on another request's stack now sees
    `Owns()` false and queues its fetch for its own pump (at most 25 ms
    later). `TestRunWokenOnAnotherRequestsEntryWaitsForItsOwnPump` drives
    `binding.handleRequest` and `binding.pump` the way the bootstrap does
    and failed before the fix.
  - Not proven the same cause: the 7-20 s and 4-5 s pauses between a
    controller's consecutive requests; a fetch orphaned this way costs 15 s
    (apiserver body) or 30 s (headers) plus the client's retry, which fits
    the shape but was not observed directly there. Go runtime timers
    scheduled at the end of a request's entry die with that request too;
    the pumps of other in-flight requests cover it, an idle isolate has no
    timers until its next request.
- Run 36785007337 (501e64d, before the turn fix) wedged workerd 15 s after
  the node registered and never recovered: `dev.log` ends at 23:51:09.225
  with three concurrent `front iso=ca7429b3 GET /api/v1/nodes/runnervm8df0l`
  (the kubelet's lease owner lookup, flannel's `?timeout=15m0s` and the
  agent's label write path, all node-certificate clients), no `apigroups`
  line followed for them, and `procs.log` shows the main thread at
  `main_ms` 1020-1040 per 1 s sample, state R, RSS frozen at exactly
  4 927 100 kB for the next 15 min; devtls only reported 502s as the
  clients' deadlines passed and no later request was logged by the front.
  Not seen in any other run. It is a synchronous spin, not a hang: a
  hung request leaves the event loop free, so the front would still have
  logged the kubelet's retries and the once-a-minute `mem` lines would have
  kept coming. The thread was already at 100 % for the 13 s of start-up
  before it, so the spin began under load, in whichever isolate was
  running at 23:51:09.225 (the front, the apiserver worker handling the
  three GETs, or the core group worker).
  - Not identified. Read for a loop that never blocks (on js/wasm there
    is no sysmon, so any goroutine that does not block or yield holds the
    thread, and the JS event loop only runs when every goroutine is
    blocked): the bridge (`pump` drains a bounded queue, `drain` sleeps,
    `CurrentWindow` waits on a channel), the apiserver's request path for
    a node GET (edge-cert authentication, node authorizer, RBAC, the
    `forwardTo` copy loop, the vault and `InstallServiceAccountKey`
    Get-then-Put loops, which all block on a fetch per round), the front's
    TS and the loader bootstrap, and the Cluster DO's synchronous SQL
    loops (`compactTo` deletes a shrinking set until `rowsWritten` drops
    below the batch; the outbox and R2 loops await). The only
    `select`-with-`default` loops in the tree are the pump, the WebSocket
    enqueue/finish paths and the tunnel push; none spins. The turn fix
    (5ec6a28) changes where a fetch is issued, not whether anything
    blocks, so it neither causes nor removes a spin. Not reproduced: the
    shape needs the CI node joining under a saturated runner.
  - What the next occurrence will record: `scripts/ci/e2e.sh`'s sampler
    now takes the dev log path, and when workerd's main thread stays at
    900 ms or more per sample for 20 consecutive samples while `dev.log`
    does not grow, it attaches gdb once (installing it if the runner has
    none) and prints the main thread's backtrace into `procs.log` as
    `stack pid=... #N ...`, then detaches. Checked in an Ubuntu 24.04
    container against a shell spin: detection at 20 s, backtrace printed,
    process left running. Checked on this Mac with `sample` what the two
    spin kinds look like: a JS `for (;;)` shows
    `Builtins_InterpreterEntryTrampoline` frames above
    `ServiceWorkerGlobalScope::request`; a Go `for {}` reached from a
    `js.FuncOf` callback shows `Builtins_JSToWasmWrapper` followed by
    unnamed frames above `jsg::Lock::runMicrotasks`. So the dump says
    JS or wasm, and which entry kind (request, microtask, alarm, SQLite
    or V8 GC by the named C++ frames beneath) without naming the Go
    function: the shipped wasm has no name section (`-ldflags=-s` drops
    it and `wasm-opt --strip-debug` would too) and gdb sees only JIT
    addresses. The inspector is no help while spinning: a `Debugger.pause`
    sent through wrangler's inspector port during the JS spin never even
    opened the socket in 8 s, so nothing on the inspector side runs off the
    busy thread. Local workerd has no CPU limit, so a spin wedges the
    whole process until it is killed (the client's disconnect does not
    stop it); production would end the request at its CPU limit and only
    that isolate would be lost.
- Sharded Conformance run 36807713386 (4208926, 8 clusters of 2 nodes): 3
  shards failed before the tests, 5 ran with about 6 transient failures
  each, and every shard logged a capnp break 20 s after `Ready`. Read from
  `dev.log`, `devtls.log`, `procs.log` and the agent logs; reproduced on
  the dev stack where noted.
  - `RPC connection broken for non-DISCONNECTED reason ... expected
    expectedSizeInWords <= options.traversalLimitInWords [46780765050 <=
    8388608]` is `scripts/ci/e2e.sh`'s own `user_worker_port` probe. It
    sent `GET /livez` to every port the user workerd listens on; one of
    them is workerd's `--debug-port`, a Cap'n Proto RPC listener miniflare
    opens for the dev registry ("exposes a privileged interface that
    allows access to all services in the process. For use by miniflare and
    local development only"). capnp read the HTTP request as a segment
    table: `expectedSizeInWordsFromPrefix` sums the uint32 words of the
    first read, and `GET /livez HTTP/1.1\r\nHost: 127.0.0.1:<port>\r\n
    User-Agent: curl/8.5.0\r\nAccept: */*\r\nAuthorization: Bearer <64 hex>`
    predicts 4.4e10 to 4.7e10 words for random tokens; the five runs
    reported 4.34e10 to 4.68e10. Each break is logged in the same
    millisecond as the probe's next `front GET /livez`, and on the dev
    stack the port that answers curl with an empty reply is the
    `debugPortAddress` miniflare writes to the registry file. The break
    only ends curl's own connection; the `dynexc GET /livez Error: Network
    connection lost.` beside it, and the 771 such lines over the run, are
    the dynamic worker's exception for a client that went away (every one
    checked was a `?watch=true` request, the kubelet's per-namespace
    ConfigMap and ServiceAccount watches). `e2e.sh` now starts wrangler
    with `WRANGLER_REGISTRY_PATH` under `.build/ci`, takes the debug port
    from the registry, finds the user workerd as the process listening on
    it, probes only its other ports and starts the sampler before probing.
    Not explained: the dev stack on this Mac closes the connection the same
    way but prints no warning for it.
  - The transient `an error on the server ("")` failures (post
    namespaces, pods, resourcequotas, replicationcontrollers, put secrets,
    patch namespaces, delete validatingadmissionpolicies) are devtls 502s
    whose `proxy error` is `read tcp ... connection reset by peer` from the
    user workerd, 14 to 22 per shard. kj's `HttpServer` closes a keep-alive
    connection idle for 5 s (`pipelineTimeout`, workerd uses the default
    settings) while Go's transport keeps it for 90 s. When the next request
    on such a connection and the expired timer reach the same event-loop
    turn, `exclusiveJoin` takes the timer, the socket is closed with the
    request unread and the kernel answers RST. In shards 3 and 4 three
    requests written at the same second were all reset 6 to 10 s later in
    the same millisecond, with the main thread at 1030 to 1060 ms per
    second and `dev.log` still flowing, so the loop had not polled the
    sockets for that long. Go replays only idempotent requests after a
    reset on a reused connection, so GETs passed and writes failed.
    Reproduced on the dev stack: a POST on a connection idle 4.5 s, then
    workerd stopped with SIGSTOP for 1.5 s across its deadline, returns
    `read: connection reset by peer`; the same GET is replayed by Go and
    answers; the same POST with `IdleConnTimeout` 2 s answers. devtls now
    replays a request whose RoundTrip failed with ECONNRESET as it already
    did for EOF: a reset before any response byte means the server closed
    with the request still unread, so it was never parsed; a response that
    started is returned as is (`TestReplaysARequestTheBackendResetWithoutReading`,
    `TestDoesNotReplayAfterResponseBytesArrived`). Through the fixed devtls
    the SIGSTOP case logs `lost its connection before wrangler dev
    answered, retrying` and the POST gets its real answer.
  - Shard 5 (`timed out waiting for a Ready node`): the agent's own
    `POST .../selfsubjectaccessreviews` got that reset at 02:56:34.677 (the
    one `connection reset by peer` in its `devtls.log`), k3s's
    `startNetwork` called `RequestShutdown("failed to start networking:
    failed to check if RBAC allows node list: an error on the server
    (\"\")")`, and the agent exited 20 s after joining, taking the tunnel
    (`GET /v1-k3s/connect status=0`) and every later `failed to find
    Session for client` with it. Same cause as above.
  - Shard 0 (`timed out waiting for devtls`): the first gdb dump. workerd's
    main thread went to 1020 ms per second at 02:56:04.5, right after
    `loader ... load=admission loaded=6 ms=926` with the Helm CRD `POST
    .../customresourcedefinitions` in flight, and never came back (RSS
    frozen at 2.0 GB for 33 min). The dump taken 20 s in shows the thread
    inside V8's Oilpan sweeper: `cppgc::internal::MutatorThreadSweeper::
    FinalizeAndSweepWithDeadline` < `Sweeper::SweeperImpl::
    PerformSweepOnMutatorThread` < `IncrementalSweepTask::Run` <
    `v8::platform::DefaultPlatform::PumpMessageLoop` < the `Deferred` at
    the end of `IoContext::runImpl`, reached from `IoContext::runSingle<
    awaitIoImpl<HttpClient::Response ... fetchImplNoOutputLockAttempt>>`
    (a `fetch()` response continuation) under `kj::TaskSet::Task::fire`.
    So the spin is workerd pumping V8 platform tasks after a JS
    continuation, and the incremental sweep task never finishing; no JS or
    wasm frame is on the stack, which rules out a loop in our code for this
    occurrence. Not identified further: whether the sweep is stuck or
    re-posting itself forever, and whether the 60 MB `admission` worker
    that had just been compiled is what it sweeps. One dump; the sampler
    takes only one per quiet period.
    A second dump (run 36811705086, commit 00038bf, shard 0, mid-run at
    03:49 rather than at startup) shows the same loop one frame up:
    `DefaultForegroundTaskRunner::PopTaskFromQueue` <
    `DefaultPlatform::PumpMessageLoop` < the `Deferred` in
    `IoContext::runImpl`. Both are workerd draining V8's foreground task
    queue after a continuation, so the likelier reading is a task that
    re-posts itself, not one sweep that never ends. The shard sat until it
    was cancelled; a Conformance shard is now capped at 90 minutes.
    Identified and mitigated (sources: workerd v1.20260926.1, the one in
    wrangler 4.144.0, and its V8 15.4.80.5). The spin is a deadlock between
    workerd's main thread and every V8 worker thread, closed by a
    busy-wait:
    - `IoContext::runImpl`'s `KJ_DEFER` runs `while (!gotTermination &&
      js.pumpMsgLoop())` after each continuation (`io/io-context.c++`),
      and `pumpMsgLoop` is `v8::platform::PumpMessageLoop(..., kDoNotWait)`
      on V8's `DefaultPlatform` (`jsg/setup.c++`). The loop only stops when
      the foreground queue is empty or `limitEnforcer->getLimitsExceeded()`
      says so; local workerd has no limits.
    - cppgc's sweeper (`heap/cppgc-internal/sweeper.cc`) posts a
      "low priority" `IncrementalSweepTask` through
      `GetForegroundTaskRunner(kForegroundLowPriority)`, but
      `DefaultPlatform::GetForegroundTaskRunner` ignores the priority and
      returns the isolate's one queue (`libplatform/default-platform.cc`),
      so the pump pops it at once. `SweepForLowPriorityTask` calls
      `SweepInForegroundTaskImpl`, which, while
      `IsConcurrentSweepingDone()` is false, runs the mutator in
      `kOnlyFinalizers` mode; `PerformSweepOnMutatorThread` then returns
      false by construction (`if (sweeping_mode != kAll) return false`),
      the result is `kInProgress`, and the task re-posts itself with no
      delay (`ScheduleLowPriorityIncrementalSweeping()`). That is the loop
      the six main-thread dumps caught at `~IncrementalSweepTask`,
      `IncrementalSweepTask::Post`, `PopTaskFromQueue` and
      `PerformSweepOnMutatorThread`. It ends when the `ConcurrentSweepTask`
      job becomes inactive (`DefaultJobState::IsActive`:
      `GetMaxConcurrency != 0 || active_workers != 0`, and
      `GetMaxConcurrency` is 1 until the job's `Run` completes).
    - The job never runs. The all-thread dumps (`thread apply all bt`,
      added to the sampler; four spins in runs 36819201396 and
      36819206349, identical shape) show all three `V8 DefaultWorker`
      threads (the runner has 4 vCPUs, `NewDefaultPlatform(0)` makes
      `NumberOfProcessors - 1` workers) parked in
      `CollectionBarrier::AwaitCollectionBackground` under
      `LocalHeap::AllocateRawWith` < `HeapAllocator::
      CollectGarbageAndRetryAllocation`: two Maglev concurrent compile
      jobs (`MaglevCodeGenerator::GenerateDeoptimizationData` allocating a
      `ProtectedFixedArray`) and one Sparkplug batch
      (`ConcurrentBaselineCompiler::JobDispatcher::Run` allocating a
      `TrustedByteArray`). A background allocation that fails requests a
      GC and parks until the main thread performs it
      (`heap/collection-barrier.cc`), and the main thread is in the loop
      above waiting for one of them to free up. `procs.log` shows it:
      `main_ms=1020..1040 other_ms=0 threads=5` for the whole spin. The
      cycle needs every worker thread blocked at once, so it is a small
      machine's failure mode (3 workers shared by ~20 isolates compiling
      JS at startup); it was never reproduced on this Mac, which also has
      more cores and, more to the point, runs workerd with
      `--single-threaded-gc` already (`jsg/setup.c++` sets it under
      `#ifdef __APPLE__`).
    - Mitigation: `--single-threaded-gc`. `CppHeap::sweeping_support_` is
      `kIncremental` instead of `kIncrementalAndConcurrent` under that
      flag (`heap/cppgc-js/cpp-heap.cc`), so no `ConcurrentSweepTask` is
      posted, the mutator sweeps in `kAll` mode, each 5 ms slice makes
      progress and the task finishes; it also implies
      `--no-concurrent-marking`, `--no-parallel-*` and
      `--no-cppheap-concurrent-marking` (`flags/flag-definitions.h`),
      which costs main-thread GC time but removes every GC-side wait on
      the worker pool. It is the only V8 flag that reaches cppgc's
      sweeping type; `--no-concurrent-sweeping` only affects the V8 heap.
      The flag reaches the local workerd through miniflare's
      `MINIFLARE_WORKERD_V8_FLAGS` (space-separated, copied into the
      config's `v8Flags`; a wrong flag aborts at `jsg/setup.c++:149
      unrecognized V8 flag`, checked). `make dev` and `scripts/ci/e2e.sh
      up` set it (`up` only when the variable is unset, so the workflow's
      `v8_flags` input, for example `--no-single-threaded-gc`, can measure
      the baseline again). The workflow's `probe_minutes` input stops the
      suite after that many minutes and passes, so one dispatch is eight
      startup-plus-load samples in about 20 minutes; a spin is a `main
      thread busy` dump in `procs.log`.
    - Measured on ci/spin-probe (this branch plus the inputs), 8 shards per
      run, 6 min of suite after `up`: baseline runs 36819201396 (3 of 8)
      and 36819206349 (1 of 8), all four at 30 to 50 s after wrangler
      started; `--single-threaded-gc` runs 36819203707 (0 of 8) and
      36819208502 (0 of 8). Unmitigated main-branch runs the same day:
      36818846178 5 of 8 and 36814575388 4 of 8. So 13 of 32 without the
      flag against 0 of 16 with it (Fisher one-sided p about 0.002).
    - Production is not the local `DefaultPlatform`, so this exact cycle
      cannot be asserted there; the pump loop and cppgc are the same code,
      but the production runtime enforces CPU limits, and the loop breaks
      on `getLimitsExceeded()`, so the worst case is the request dying at
      its CPU limit rather than the process. Open: whether the production
      platform's task runner honors `kForegroundLowPriority`, and whether
      workerd would accept the flag on Linux (an upstream issue would
      carry the dumps).
  - Shard 6 (`no workerd port answers /livez` after 8 min): `dev.log`
    stopped at 02:54:14.340, 17 s after `Ready`, with the addons pass
    (`addons: ok=false ... helm=true`), `/internal/loadbalancer/provision`
    and `POST /api/v1/namespaces/kube-public/configmaps` the last lines and
    eleven dynamic workers loaded in the previous 9 s. No `procs.log`: the
    sampler started only after the port probe, which never succeeded
    against the wedged process (30 rounds of `curl -m 5` per port). Same
    shape as shard 0 and run 36785007337; the sampler now starts as soon
    as the user workerd's pid is known from the registry, so the next one
    leaves a dump.
- Run 36807713386 (4208926, sharded, two nodes; 5 of 8 shards finished,
  263 of 279 passed). Of the 16 failures, 10 were `an error on the
  server ("")` (the transient 502 under investigation elsewhere; DNS
  Subdomain and ConfigMap binary data also had their pod wait aborted by
  it after 46 s and 28 s). The other six, read from the junit bodies and
  `dev.log`:
  - ReplicationController failure condition: the quota admission listed
    the stored pods and admitted anything that still fit, with no
    reservation, so the RC controller's two concurrent pod creates
    (03:04:26.651 and .652, after the first at .560) both saw one stored
    pod and three pods ran under `pods: 2`; no ReplicaFailure condition
    could appear. Fixed: a matching quota's `status.used` is raised with a
    CAS write before the request is admitted (upstream's plugin does the
    same through `UpdateStatus`), retried on conflict, dry runs skipped,
    and the check moved to the end of the validating chain.
    `TestResourceQuotaReservesUsageBeforeTheObjectIsStored`.
  - DaemonSet rollback and StatefulSet scaling order share a cause: the
    controllers' pod `Delete` dropped the pod from the pass snapshot at
    once, although the graceful delete only stamped `deletionTimestamp`.
    The DaemonSet controller created `daemon-set-lsljm` at 03:07:59.999
    while `daemon-set-pkv4s` on the same node was terminating (its
    kubelet delete came at 03:08:01.110), so the spec's "existing" pod
    was gone after the rollback; the StatefulSet controller deleted ss-2,
    ss-1 and ss-0 within a second (03:00:57.7, 58.1, 58.5) instead of one
    at a time, and the kubelets removed them as ss-0, ss-1, ss-2
    (03:01:03, 04, 06), which the spec's DELETED-order watch rejected.
    Fixed: the delete reads the object the apiserver returns and keeps a
    pod that still has a grace period or finalizers in the snapshot (fake
    clientsets, which delete at once, keep the old path).
    `TestGracefulPodDeleteKeepsTheTerminatingPodInTheSnapshot`.
  - NoExecuteTaintManager minTolerationSeconds: 58 `Cancelling deletion`
    events in two minutes and no eviction. Upstream's controller keeps
    its schedule in memory (`CreatedAt`, a timer per pod) and cancels and
    re-adds a pod processed at the same instant; each pass built a new
    controller, so the timer restarted from each pass's own start and
    never fired (the booking of the next pass through the workqueue delay
    hook was correct, which is why a single pass looked right in
    `TestSyncBooksTheTolerationSecondsOfAUserNoExecuteTaint`). Fixed:
    the pass evicts directly, with the toleration start persisted under
    `/k8flare/tainteviction/<ns>/<pod>` in the store (an in-memory map
    when the worker has no store) and the next pass booked for when it
    elapses; intolerant pods are deleted at once, with the
    DisruptionTarget condition and the TaintManagerEviction event as
    upstream. The upstream controller is no longer linked.
    `devicetainteviction` has the same per-pass shape and is not changed.
  - NodePort session affinity: the CI network, fixed in `e2e.sh` (above).
  - Not re-run; the `[Serial]` pod spreading specs did not run in this
    run's finished shards.
- Run 36814963545 (7f9b369): the quota reservation above regressed three
  ResourceQuota specs (a configMap next to `kube-root-ca.crt` denied with
  `used: configmaps=2` of 2, a pod denied with `used` equal to its own
  requests, the RC spec's second pod denied so `ReplicaFailure` never
  cleared). Cause: the remote admission plugin's `Admit` ran the
  validating chain as a second `validate` phase after mutation, and the
  apiserver then called `Validate`, so every write reached the quota
  check twice and the second call saw the first's reservation on top of
  the stored objects. The Admit-time pass is now the `check` phase, which
  the quota skips; only the validate phase reserves
  (`TestRemoteRunsTheValidatePhaseOncePerRequest`,
  `TestResourceQuotaReservesOnlyInTheValidatePhase`). Usage is released
  the upstream way: the resourcequota controller recomputes `status.used`
  on the pass a pod delete or a quota write triggers, and the admission's
  live list is only a floor, so a stale-high `status.used` lasts until
  that pass. CustomResourceFieldSelectors also failed in that run; see the
  next entry, which corrects the first reading of it.
- Run 36837666083 (9cc3bd1): 444 of 446. Both failures have a unit test
  and a fix; neither has been re-run in CI yet.
  - CustomResourceFieldSelectors (also in 36814963545). The event the
    `host=host1` watch missed is the DELETED of the object removed by
    DeleteCollection, not of the one updated to `host2`: in both runs the
    name in the received `deleted` set is the one the webhook converted as
    `host2:8080` at generation 2. `v1` is the storage version, so the four
    `v2` watchers (two watches, two informers) convert every store event
    through the webhook, and upstream's converter calls it with
    `context.TODO()`. The bridge gave a fetch without a window to the
    newest request, here the DeleteCollection that caused the event. That
    request answered before its next pump, the loader stops pumping a
    request once its response resolves, and the four queued calls sat until
    the window closed: `bridge: drain timeout kind=dispatch age=5s
    in-flight: 4` and again at `age=11s`/`12s`, both runs. The decode then
    failed, `decodeValue` in `apiserver-kine` turns a failed decode into
    "no object", and `watchEvent` dropped the event. The watchers were
    stuck for those ten seconds, which is why the update's conversions
    reach the webhook 13.5s after the PUT. Fixed in `worker-bridge`: a
    fetch without a window goes to the request whose event woke the
    goroutine when that request is still open (a held window still comes
    first), so a watch issues its conversions on its own streaming request
    (`TestFetchWithoutAWindowUsesTheRequestWhoseEventWokeIt`, js/wasm).
    Left as they are: a window that has answered and is only draining is
    still handed out as the newest one, and nothing pumps it; and a watch
    event whose decode fails is dropped silently, where upstream's etcd3
    watcher ends the watch with an error event so the client relists.
    The webhook pod log in the report is cut to its last 42 lines, so it
    cannot show which conversions never arrived.
  - Endpoints lifecycle (the earlier variant, `unable to find Endpoint
    Service in list of Endpoints`, is the same cause). The audit log has
    `system:kube-controller-manager` deleting `endpoints/testservice`
    50ms after a pass finished building its controllers. Upstream's
    endpoints controller deletes
    the Endpoints of a key whose Service is missing, and the only thing
    that queues a key without a Service is `checkLeftoverEndpoints`, which
    upstream runs once when the controller manager starts. A pass builds
    and runs the controller afresh, and the spec's own Endpoints writes
    trigger a pass, so the startup sweep ran within seconds of the create.
    Fixed in two parts. The sweep only sees Endpoints carrying the
    controller's `endpoints.kubernetes.io/managed-by` label, the ones it
    can have left behind (`TestSyncKeepsEndpointsWrittenByHandWithoutAService`,
    `TestSyncRemovesEndpointsTheControllerLeftBehindADeletedService`); a
    once-per-isolate gate was not used because an isolate restart would
    bring the sweep back at an arbitrary moment. And because that sweep
    was also the only thing removing a deleted Service's Endpoints between
    passes, the Service store now deletes them in `AfterDelete` as
    upstream's `afterDelete` does (`TestDeletingAServiceDeletesItsEndpoints`).
    Not changed: that hook still releases the Service's IPs on a dry-run
    delete, which upstream skips.
- Runs 36840583147 (443 of 446) and 36840587032 (444 of 446), both 9cc3bd1
  again: CustomResourceFieldSelectors failed in all three runs of this
  commit, Endpoints lifecycle in two. Two more specs failed once each in
  36840583147. Both looked like one write arriving twice through the
  `devtls` replay; neither is.
  - CustomResourceConversionWebhook, non homogeneous list (`"cr-instance-2"
    already exists`). The audit log has five creates with five audit IDs:
    the first answers 500 after 11.4s and the other four 409; the spec
    retries a failed create up to five times. The first create stored the
    object and then failed converting it back for the response:
    `conversion webhook ... failed: ... kine GET /kv ...: bridge: pump
    window closed`, with `drain timeout kind=dispatch age=5s in-flight: 1`
    and `age=10s` before it. It is the borrowed window of the
    FieldSelectors failure, this time inside a unary request whose
    conversion was handed to a newer request that had already answered.
    The same bridge fix covers it
    (`TestHandlerFetchWithoutAWindowStaysOnItsOwnRequestWhenANewerOneIsOpen`).
  - ControllerRevision lifecycle (`Failed to delete ControllerRevision ...
    not found`). The audit log has `system:kube-controller-manager`
    patching the two daemon pods and deleting the initial revision 600ms
    before the spec's own delete arrives. The spec creates a second
    revision with the initial one's `Data`, so both match the DaemonSet and
    upstream's `dedupCurHistories` keeps the higher revision, relabels the
    pods and deletes the other. That is upstream behaviour; the spec
    depends on its list and delete, two round trips, beating the
    controller's event, two pod patches and a delete. Here the spec's list
    took 320ms and its DELETE reached the front worker 920ms after the step
    began, while the pass's writes took about 100ms each. Not fixed; the
    lever is the latency of an external request, not the controller.
  - The replay itself: the `devtls` logs of the four shards read hold 49
    replays. Every replayed write with a path of its own (a namespace
    DELETE, a pod POST, a pod status PATCH, a rolebinding POST) reached the
    front worker once, after the replay, and the three checked against the
    audit log have one entry each, so those were requests workerd never
    read, as the replay assumes. `devtls` is not changed.
- Runs 36844656269 (443), 36844659450 (442) and 36844662872 (445 of 446)
  on a1687e5. Endpoints lifecycle passes in all three. What is left is
  one cause, the workerd main thread at 100%, seen three ways. None of it
  is fixed, and none of it comes from the bridge change.
  - Shard 4 in the first two runs (Watchers `Watch closed unexpectedly`,
    a liveness pod `not found`, a webhook deployment never ready, two
    pods not Succeeded after 539s). The MutatingAdmissionPolicy spec
    creates `marker-deployment`, which its policy mutates to 1337
    replicas, and deletes it at once. In the passing runs the delete
    lands 0.4 to 0.7s after the create and before a pass has acted. In
    the failing ones a pass created the ReplicaSet first (in 36844659450
    0.5s after the Deployment was already deleted, from a list taken
    before the delete) and its ReplicaSet controller created all 1337
    pods inside one pass of 57s and 67s: the slow start doubles its
    batches up to 245 concurrent creates, the client is not rate limited
    (QPS 1000), and
    `drain` does not end a pass while a sync is in flight. The main
    thread sat at 1000ms or more per second in 517 of 536 and 683 of 712
    samples for nine to thirteen minutes while the pods were scheduled,
    reported on and collected; `devtls` answered 133 and 189 requests
    with 502, node leases among them, and the Watchers spec's PUT waited
    from 10:03:33 to 10:12:27 to reach the front worker. In that run the
    garbage collector deleted the ReplicaSet a second after the pass
    ended. Upstream
    creates pods at the same 20 a second (the controller manager's
    client QPS), but its apiserver has room left, so the delete and the
    collector get through within a second and a few dozen pods exist.
    Here 20 pod creates a second is the whole capacity. Limiting every
    call of the workloads client to 20/30 was tried on a throwaway branch
    and made three runs worse (439, 444, 441): it throttles the namespace
    deleter. Done instead, in 5f188fe: the pod client the
    controllers create through takes one token per create from a bucket
    of 20 a second, burst 30, shared by the isolate (the controller
    manager's client defaults, `pkg/controller/apis/config/v1alpha1/defaults.go`),
    and gives up when the pass is cancelled; every other call keeps its
    rate. The limiter alone does not bound the count, a host test with
    it still made 1337 pods in one 65s pass, so the same wrapper refuses
    a create whose controller, or that controller's own controller, has
    left the pass's snapshot: the live feed removes a deleted Deployment
    or ReplicaSet mid-pass, the slow start stops at the first refused
    batch and the pass drains
    (`TestSyncStopsCreatingPodsOnceTheDeploymentBehindTheReplicaSetIsDeleted`,
    `TestSyncStopsCreatingPodsForAReplicaSetDeletedDuringThePass`,
    `TestPodCreatesAreSpacedOnceTheBurstIsSpent`). Upstream's apiserver
    would accept such a pod and the collector would delete it. What
    still depends on the cluster: the spec's Deployment delete has to
    reach the store while the pass creates at 20 a second.
  - CustomResourceFieldSelectors, once (36844659450 shard 5), now at
    line 305 instead of 293: both plain v2 watches received both DELETED
    events. The spec gives its informers a 30s context from registration.
    With the main thread saturated in all 38 samples of that window, the
    creates took 5s, the lists 9s, the DeleteCollection 4.5s and the
    update's GET and PUT 3.5s and 9.2s, so the PUT finished 1.8s after
    the informers had been cancelled. No event was dropped and no
    `drain timeout` or `pump window closed` appears.
  - ControllerRevision, once (36844662872 shard 1), now `failed to count
    required ControllerRevisions`: the same `dedupCurHistories` race from
    the other side. The spec's create answered at 10:26:59.143, the pass
    listed at .335, patched the two pods and deleted the initial revision
    at .882, and the spec's first list reached the front worker at .930,
    790ms after it was sent, so it never saw two revisions.
- Runs 36857943990 (445), 36857947190 (445) and 36857951122 (446 of 446,
  the first full pass) on the tree of 1a8bacb, the pod create limiter and
  the owner check. Shard 4 passes in all three, the MutatingAdmissionPolicy
  spec with it, and no shard reports a workerd spin. The two failures are
  one request each that waited past its client's deadline. Neither is
  fixed.
  - LimitRange (36857943990 shard 5), `limit_range.go:185`: the GET of
    `limit-range` reached `devtls` at 12:10:00.978 and was given up at
    12:10:18.979 with no byte back (502). Two node lease PUTs with a 10s
    timeout were answered 502 in the same window.
  - Watchers concurrent watches (36857947190 shard 0), `watch.go:454`:
    the watch from resourceVersion 2178 and the DELETE of `cm-0` reached
    `devtls` at 12:27:50.780 and .800 and both were given up at
    12:28:00.780 with no byte back.
  - The workerd main thread was at 1000ms or more per second in 21 of 22
    and 13 of 13 samples of those windows. That is not what sets the two
    runs apart: the same holds for 421 to 785 samples of every shard but
    one of the run that passed (205 of 939 in shard 6). What made these
    two requests wait 10s and 18s inside workerd while others were served
    was not looked at.
- Runs 36948221088, 36948227017 and 36948233022 on 699ba58 (the scheduler
  flush-period retry, the Job pod events, network policy on the agents
  and the admission policy wake): 446 of 446 in all three, the first
  three consecutive full passes. Unit and E2E (live 16 of 16) pass on the
  same tree; main is at 699ba58.
- The batch before it (b804967: the same without the scheduler and Job
  commits, plus the HPA consumer in the main Worker and the Gateway API
  CRDs) failed `Watchers ... concurrent watches in same order` in all
  three Conformance runs and in the required E2E set, and
  CustomResourceFieldSelectors in all three. Each of the four changes
  alone passed the Watchers spec once. What the failing shard shows
  (E2E 36938910357, required 2):
  - From 23:30:19.4 to 23:30:34.5 no request from outside reached the
    front worker (`devtls` has the POST of a configmap waiting from
    19.628 until the client gave up at 28.867; a node lease PUT and two
    more requests waited 10 to 15s), while requests between workers in
    the same isolate were served throughout: 334 in those 10s, all from
    the namespace deleter walking three `netpol-*` namespaces. Timers
    inside workers were late by the same amount (an HPA pass that sleeps
    10s took 15.2s).
  - Such windows exist without these changes too (52 requests released
    together at 00:41:38 in the run with only the network policy
    commits); the batch made them longer and more frequent. Two things
    it added: an HPA pass that slept 10s every 15s with no HPA in the
    cluster, and four more namespaced custom resource types for every
    namespace deletion to DELETE and GET (250 to 450ms each through the
    `customresources` worker, against about 20ms for a built-in type).
  - Not established: why requests entering through the dev proxy starve
    while in-process requests are served. Not looked at: why a custom
    resource request costs ten times a built-in one.
- Runs 36954014600 (444), 36954020327 (446) and 36954025939 (445 of 446)
  on 3337b7e (`ci/batch6`: 699ba58 plus the node taint retry, the HPA
  consumer in the main Worker, the HPA pass that returns at once with no
  HPA, and the removal of the 15s metrics loop). Unit and E2E pass. The
  three failures are in shard 4 and none comes from the batch.
  - 36954014600, `AdmissionWebhook should honor timeout` and `ConfigMap
    optional updates` (both `bridge: no response headers in time` after
    18 minutes): the 1337-pod pass again. The MutatingAdmissionPolicy
    spec's create of `marker-deployment` answered at 02:57:26.587 and its
    DELETE entered `devtls` at 02:57:26.700. A pass that began at
    02:57:26.395 created the ReplicaSet at 27.168 and 1338 pods by
    02:58:28 at the limiter's 20 a second. The DELETE reached the front
    worker at 03:06:50, after one `lost its connection` replay at
    02:58:48, so the owner check of 5f188fe had no deletion to see. Up to
    02:57:26 the shard looked like the three passing runs on 699ba58 (3
    replays and no 502 in `devtls`, 9 to 12 passes a minute), and the
    taint retry fired once in the whole run (`nodes "e2e-fake-node-g26j2"
    not found`), so the new retry is not what loaded it. The same thing
    happened in one of the three runs of the Gateway batch (36954041871,
    DELETE sent 03:30:12, answered 03:37:48). In the other seven shard 4
    logs read (four with this batch's code, three on 699ba58) no pod was
    created in that namespace. This is the caveat recorded with 5f188fe:
    the delete has to get into workerd while the pass runs, and a request
    from outside can wait minutes there.
  - 36954025939, `ServiceAccounts should mount an API token into pods`
    (`kubectl exec` exit 1, `close 1011 ... close 1006 (abnormal closure):
    unexpected EOF`): two exec sessions to the same node 50ms apart.
    `packages/node-tunnel/tunnel.go` keeps one package-level `stream` for
    a node's tunnel object; the second session's dial replaced it at
    02:49:40.833, the first ended normally at .837 (`kubelet read done`),
    its client socket closed, `upgradeClosed` closed whatever `stream`
    was, and the second session read EOF at .843. Stdin bytes and bytes
    queued before the dial share the slot the same way. One occurrence in
    nine shard 4 logs (15 to 18 exec sessions each). Being fixed on
    `work/stream-id`.
- The Gateway batch (`ci/batch7`, e994b7b: the above plus the Gateway API
  CRDs, the harness wait for packaged add-ons and the ConfigMap count
  fix) failed the required E2E set and all three Conformance runs
  (36954041871, 36954047059, 36954052382), with 55 to 127 `devtls`
  replays and 13 to 256 502s in shard 4 against 1 to 17 and 0 to 1 on
  699ba58. Not merged. The CRDs stay out until the cost of a custom
  resource request and the wait of outside requests are understood.
- Why a request from outside waits while requests between workers are
  served (the open question of the two entries above). Reproduced on the
  dev stack of this Mac on 3337b7e, no nodes: a probe GET of
  `/api/v1/namespaces/default` every 250ms through port 18787 (p50 8ms
  idle), then a Deployment with 1337 replicas. For the 68s of pod creates
  273 probes had p50 60s, 221 took over 2s and 168 failed; in six of the
  seven 10s windows no probe reached the front worker while it logged 270
  to 370 in-process requests, and the once-a-minute `mem` lines came in
  bursts when the creates paused. One run.
  - Cause, from the source: kj's `waitImpl` (`kj/async.c++`) calls
    `loop.wait()`, which is what reads sockets and fires timers, only when
    `loop.turn()` finds the event queue empty, and polls in between only
    every `busyPollInterval` turns, which defaults to `kj::maxValue`
    ("if busyPollInterval is kj::maxValue, we never poll"); workerd's
    server does not set it (the only `setBusyPollInterval` in its tree is
    a test). Every worker of the dev stack is in one workerd on one
    thread, a service-binding fetch between them is not socket I/O, and
    the Durable Object's SQLite is synchronous, so a controller pass can
    keep the queue non-empty for as long as it has work. Production runs
    each of them in its own request context; this is the dev stack and CI
    only.
  - Done, 497255e on `ci/batch8`: the front worker makes a request whose
    host is `k8flare.internal` (every component's client) await its own
    `setTimeout(0)` when 50ms have passed since one last fired. With every
    in-process chain passing the front worker, they all end up waiting on
    a timer, the queue empties and the loop polls. Same procedure, one run
    each: 297 probes during the creates, p50 232ms, max 3.2s, 7 over 2s,
    none failed, creates 74s; for the 120s after the Deployment's delete
    none failed against 466 of 480 without it. 10ms measured about the
    same (p50 213ms, creates 69s), 0ms worse (p50 1.1s, creates 82s). A
    first version that shared one promise between requests also removed
    the failures but left p50 at 4s; a promise resolved from another
    request's context is not something workerd promises outside the local
    stack, so it was not used.
  - What the probes failing in 256ms for minutes after the creates were:
    wrangler's ProxyWorker gives up after three attempts at the user
    worker (`failed after 3 attempts: Network connection lost`), and
    recovers by itself when the user worker's queue empties (`recovered
    on attempt 3`). CI's `devtls` talks to the user workerd directly.
- Runs on `ci/batch8` (3337b7e plus the per-stream slot in the node tunnel,
  the front worker's yield and the garbage collector's twenty workers;
  merged as 33cad95). Unit and E2E pass on each step.
  - With the yield at 50ms (497255e) and at 200ms (6cd10bc), three runs
    each: every shard but 0 and 1 passed in five of the six, and two
    `[Serial]` Garbage collector specs failed in all six (`failed to
    delete the rc ... context deadline exceeded`). The collector deleted
    the 100 pods of an rc in 92s and 98s against 23s on 699ba58. The
    interval was not the lever. The workerd main thread is at 1000ms a
    second either way; the yield lets requests from outside in, each
    gated in-process request then waits for everything in flight to
    finish (81 front requests released in one millisecond, then 8s of
    work), and `packages/gc/collect.go` synced one item at a time, so it
    advanced one request per cycle.
  - 5ece01d: the collector syncs with twenty goroutines, upstream's
    `ConcurrentGCSyncs`; `item.OwnerReferences`, which a foreground
    owner's sync rewrites on its dependents, is read and written under a
    per-item mutex (`TestConcurrentGCSyncs`, `TestRaceOnOwnerReferences`
    under `-race`). Not checked: two goroutines can now patch the same
    dependent's ownerReferences from one snapshot (the owner unblocking
    it, the dependent dropping a stale owner); the later patch wins and
    the next pass sees the result.
  - Runs 36978505166 (446), 36978512349 (445), 36978519214 (446), then
    36983344668, 36983352526 and 36983360809 (446 each): five full passes
    of six, three in a row.
  - The one failure, `ReplicaSet Replace and Patch tests` (also once on
    6cd10bc, run 36970625324): the controller did its work (three pods
    Ready by 07:46:43, four status updates written), and the spec's watch
    (`labelSelector=test-rs=patched`, bookmarks allowed, opened 07:46:26.083)
    delivered the replayed event and one live event and then nothing for
    five minutes. The store still held the watcher: it closed it on its
    6-minute lease at 07:52:26.099 and kine redialed 1ms later, and kine
    was not blocked (more than the 64 buffered bookmarks went through).
    So the events are lost at or after kine's filter: dropped by
    `watchEvent`, or lost in the response stream between
    `responseWriter.Write` in the `apiserver-apps` worker (which enqueues
    and never blocks) and the client. It was the only apps-group watch of
    the run in both failures. Not established which; being reproduced on
    the dev stack (`work/watch-repro`).
  - Read from the same logs, not yet reproduced: a client that goes away
    does not end its watch. The watch above has no `ResponseComplete`,
    eight of ten admin watches without `timeoutSeconds` never complete,
    the leaked one redials every 6 minutes, and goroutines grew from
    14352 to 19168 in `apiserver` and from 118 to 3202 in
    `apiserver-apps` over the shard. Suspected place, by reading:
    `abort` in `BindingTransport.RoundTrip` runs through `window.Run`,
    which queues the job until the request's window is pumped, and the
    cancelled front request no longer pumps it.
- What comparing with upstream on the host showed (tests only, nothing
  fixed; `work/diff-nodeauth` 7bd12b8, `work/diff-admission` aad3e4e).
  - Node authorizer (`apiserver-authz/node.go`) against upstream's
    `NodeAuthorizer` with `NodeRules`, 124,903 questions over one cluster
    state: 55,927 answers differ, in 21 named classes. Allowed here where
    upstream has no opinion (so RBAC would decide): a token for any
    service account, list and watch of every PVC and of leases, reads
    without a namespace, subresources on reads, resources of another API
    group with the same name, mirror pod references, PVs bound through
    the PVC's `volumeName`, the default service account. Denied here and
    allowed upstream: secrets and configmaps of ephemeral containers, CSI
    volume secret refs, PV secret refs, PVCs of generic ephemeral
    volumes, PVs bound through `claimRef`, `endpoints` get. Denied here
    where upstream has no opinion: about 18,000 questions a request can
    reach, which never get to RBAC. `node.go` consults no feature gate.
    Replacement by upstream's authorizer over a per-request graph is on
    `work/nodeauth-upstream`.
  - Admission, 18 plugins, about 750 cases against the upstream plugin
    built over a fake clientset: 59 cases differ. Outcome differences,
    by plugin: NodeRestriction does not check a mirror pod's owner
    references or its secret, configmap, claim and service account
    references, and has no audience restriction on node token requests;
    ServiceAccount ignores `kubernetes.io/enforce-mountable-secrets`,
    does not special-case mirror pods, admits a pod whose default
    service account does not exist yet, and appends a second token
    volume to a pod that has one; ResourceQuota charges a pod's full
    usage on every update, skips the resize subresource, treats a
    resource missing from `status.used` as zero and falls back to
    `spec.hard`; PodSecurity evaluates updates upstream calls
    insignificant and treats a missing namespace as unlabeled;
    LimitRanger ignores pod-level resources and copies a defaulted limit
    into a missing request; RuntimeClass rejects a class being deleted
    and accepts an empty name; Priority resolves a system class that is
    not in the cluster; PodTopologyLabels also labels on update;
    PersistentVolumeClaimResize ignores the beta class annotation. Not
    covered: ServiceAccount's volume paths of `limitSecretReferences`,
    PodSecurity on namespace label changes, NodeRestriction on
    `podcertificaterequests` and `resourceslices`.
- Writes that woke nothing (`work/wake-check` 9cde004, on `ci/batch8`, not
  merged): Role, RoleBinding, LimitRange and NetworkPolicy writes now
  start a workloads pass, the only thing that recomputes a
  ResourceQuota's `count/*` usage here (upstream's quota monitor is not
  built: no `DiscoveryFunc`), and so do ResourceClaimTemplate writes,
  which upstream answers by requeueing the pods that reference the
  template. Left: `deviceclasses` (upstream registers no handler and reads
  no lister) and `ipaddresses`: `claimServiceIPs` stores the IPAddress
  before the Service, and a pass woken by that write runs
  `ensureServiceIPAddresses`, which deletes an address whose Service is
  not listed. What that leaves: a ServiceCIDR being deleted keeps its
  finalizer after its last IPAddress goes until another ServiceCIDR
  write.
- State on 2026-10-02: main and `feat/minimal-rewrite` are at 9ecdff1
  (`ci/batch10`: Unit, E2E, Conformance 3 of 3). What went in since the
  entries above:
  - The ReplicaSet spec failure was not a watch stall. Objects applied
    from the live feed had no resourceVersion, so a controller's status
    update was unconditional and a stale one overwrote labels written in
    between. `packages/workloads/live.go` now sets the revision the object
    was stored at (13 of 45 failing under load before, 0 of 45 after).
  - Idle wake-ups of the Cluster object: the compaction alarm fired every
    five minutes with nothing to compact, the constructor swept namespaces
    on every start, and a progress alarm stayed booked with no deadline.
    Fixed in `cluster.ts`; production still runs the old version.
  - The wake tables are generated (`scripts/genwake`), and Role,
    RoleBinding, LimitRange, NetworkPolicy and ResourceClaimTemplate
    writes start a pass. `ipaddresses` stays withheld.
  - A review of the node tunnel stream change found three defects (attach
    to a missing slot, writes under a lock from js callbacks, stream
    sockets surviving a rebind); fixed with one writer goroutine per
    stream.
  - The front worker yields to the socket poll at most every 200 ms for
    internal requests. It exists for local workerd, where every worker
    shares one thread; it ships to production too and costs a zero-delay
    timer there.
- Known and open:
  - `ci/batch9` had one Conformance run where shard 4 failed with no
    failed spec in its log. Not investigated; it did not repeat in the
    six runs since.
  - Namespace termination retries every 2 s without backoff; a namespace
    that cannot finish keeps the Cluster object awake.
  - A client disconnect does not end its watch (reproduced locally; the
    abort is suspected never to reach the handler).
  - `ci/batch11` (the admission swaps) failed Conformance 3 of 3 on every
    spec that needs cluster DNS: upstream's Priority plugin refuses
    `system-cluster-critical` until the scheduling API has created the
    object, and CoreDNS is created first. Unit and E2E did not notice.
    The fallback to the built-in list is back (ae337c4) as a known
    difference; the rerun (69d206e) passed Unit, E2E and Conformance 3 of
    3.
  - A log read sent the caller's `Authorization` header on to the node:
    the front worker copied it into the request the tunnel proxies to the
    kubelet (`index.ts`, the HTTP log path only; exec, attach and
    port-forward build a fresh header). Found by review, traced in the
    code, not observed on a node. Fixed in 21ced4d: the front worker no
    longer copies it and both node-bound requests drop it. Unit, E2E and
    Conformance 3 of 3, the third after a rerun of one shard: "Garbage
    collector should keep the rc around until all its pods are deleted"
    timed out with 100 pods still terminating. Not investigated; it did
    not fail in the other two runs or in the rerun.
  - The audit of 2026-10-03 (`audit/readme-b`, `audit/security`) is not
    folded into this file yet: 221 README claims, 152 with code and a
    test, 9 the code contradicts, 5 with no implementation; the raw
    `ADMIN_TOKEN` is the bearer of at least eleven internal calls and the
    key of every component token. Its entries about this file being out
    of date are unverified.
