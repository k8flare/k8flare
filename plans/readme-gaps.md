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
- Custom resource instances were already schema-aware for SSA (the upstream
  handler builds the type converter from each CRD's structural schema); the
  `customresourcedefinitions` resource itself now uses one built from the
  apiextensions OpenAPI models instead of the deduced converter.
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
- Conformance run 36737225707 (cda859a) aborted with the node
  NetworkUnavailable: flannel logged "Starting flannel" and nothing more.
  Not the podCIDR order, not the `metadata.name` watch: four E2E shards
  of the same day had the podCIDR assigned before flannel started and
  saw "Flannel found PodCIDR" within seconds, and in the failing run
  flannel's SelfSubjectAccessReview and node watch never reached the
  Worker (`front` and audit logs). What happened: after flannel's
  `GET /api/v1/nodes/<name>` (9 s, devtls) its `PATCH` of the address
  annotations got `http: proxy error: EOF` from wrangler dev at
  15:36:09.364 (a 502 to k3s; the kubelet's lease PUT got the same at
  15:36:21), `flannel.Run` returned, and k3s's `startNetwork` goroutine
  called `signals.RequestShutdown(err)`, which k8flare-agent dropped
  because it never installed k3s's shutdown handler
  (`signal.NotifyContext` instead of `signals.SetupSignalContext`), so
  the agent ran on with no CNI and no error line. Now the agent uses
  `signals.SetupSignalContext`, so such a failure is logged
  ("Shutdown request received") and ends the agent like the stock one;
  and devtls replays a request whose backend connection returned EOF
  before any response (the body is already buffered for the
  "Network connection lost" retry). k3s itself does not retry the
  annotation sync; upstream relies on systemd restarting the agent.
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
    literals or `{name}--{namespace}` hosts. A custom cluster domain
    (`k8flare.kooffice.jp` in wrangler.jsonc) is still not excluded, and
    the `/svc/{ns}/{name}` path form applies only to those control-plane
    hosts. Full Conformance run 36746667748 showed what the gap costs:
    the Ingress API spec creates Ingresses with a `defaultBackend` and no
    class, admission assigns the default `k8flare` class, the edge pass
    compiles a hostless `/` rule, and from 17:15:09 UTC every request on
    the devtls address `127.0.0.1` answered 500 `service not found` ahead
    of audit and the API (3109 watch responses in devtls.log, plus every
    kubectl and kubelet read), including the deletes that would have
    removed the Ingress; 372 specs then timed out. The edge certificate
    hosts (`k8flare edge-certificate --hosts`) are the natural source for
    the cluster domain but are not persisted, so a hostless Ingress on a
    custom-domain deployment still hijacks the API until it is deleted
    through an excluded host.
  - Changes to any Service also run the edge pass (ResolvedRefs depends on
    Services); backends are looked up per ref rather than listed.
- local-path runs as a resident Deployment, not helper pods only.
- CoreDNS NodeHosts is never written.
- attach/detach runs from a separate Worker config that CI does not load.

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
- Rate limiting is max-in-flight only (`MAX_REQUESTS_INFLIGHT`,
  `MAX_MUTATING_REQUESTS_INFLIGHT`, 400 and 200, watches and streams
  exempt, `system:masters` served when full). The count is per isolate,
  not cluster-wide. API Priority and Fairness does not build against the
  lean client-go (no FlowControl informers) and its controller needs
  goroutines and informers the Workers do not keep; no Workers Rate
  Limiting binding is wired.
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

- `kube-system/extension-apiserver-authentication` publishes the server CA as
  `client-ca-file`; kube-apiserver publishes the client CA there. Existing
  clusters also keep an old copy of the ConfigMap, which is only created
  when missing.

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

## Conformance

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
  untainted before the write check. Unverified until a Conformance run:
  flannel VXLAN between the host node and the container node (pods on
  different nodes reaching each other and NodePorts across nodes), and
  the container node going Ready under `wrangler dev` load. The
  `[Serial]` specs that behave differently with one node (taint
  eviction, DaemonSet rollback, pod spreading) have not been run on two.
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
      Existing clusters keep the old ConfigMap because it is only created
      when missing. The aggregator dialed the extension over the node
      tunnel without a client certificate, so request-header
      authentication of the proxied user could not work. Now the vault has
      a `request-header-ca` (rotated with the others), `/internal/proxy-client`
      (admin only) issues a `system:auth-proxy` client certificate from it,
      node-tunnel fetches it next to the kubelet client certificate and
      presents it on every `X-Dial-TLS` dial, and the ConfigMap publishes
      the request-header CA and `requestheader-allowed-names`
      `["system:auth-proxy"]`. Open: the ConfigMap is only created when
      missing, so an existing cluster keeps the server CA and empty allowed
      names until the ConfigMap is deleted and re-provisioned (and
      `client-ca-file` still holds the server CA rather than the client CA);
      the sample-apiserver spec was not re-run.
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
    keeps the pre-request-header `extension-apiserver-authentication`
    (server CA, bare-string allowed names) until the ConfigMap is deleted
    and kube-system re-provisioned, which nothing does automatically; the
    sample apiserver crash-loops on the bare string.
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
