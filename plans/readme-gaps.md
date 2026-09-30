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
    `api.k8flare.com`, `*.workers.dev` or `{name}--{namespace}` hosts.
    A custom cluster domain is not excluded, and the `/svc/{ns}/{name}`
    path form now applies only to those control-plane hosts.
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

- CI gates on 21 required specs. Multi-node networking is not exercised.
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
  - "at least two untainted nodes" needs a second node in CI.
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
