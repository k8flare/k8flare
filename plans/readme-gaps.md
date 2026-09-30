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
  - EndpointSliceMirroring: one `Sync(["endpoints"])` creates the slice in
    a unit test, and the queue plan maps the Endpoints key to `endpoints`,
    so the controller is fine. The spec allows 12 seconds; a pass lists
    every source the selected controllers need and the batch waits behind
    any running pass. Treat it as the same latency problem as the Services
    endpoints-latency spec.
  - The interruption at 76 minutes: GitHub reports the step as cancelled
    with only "The operation was canceled." (no runner shutdown, timeout
    or out-of-memory message), the job's later `always()` steps were
    skipped, and the run's actor was the dispatching user. Disk had 106 GB
    free at the start. The logs artifact was never uploaded, so nothing
    more is known; `scripts/ci/e2e.sh test` now prints memory, disk and
    workerd RSS every minute so the next occurrence leaves evidence in the
    step log.
