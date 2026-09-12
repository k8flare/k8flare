# Can the apiserver dynamic worker link upstream's real authn/authz pieces?

**Investigation, 2026-09-13.** Measured, not read: every claim below that says
"builds" or "does not build" came from an actual `GOOS=js GOARCH=wasm` build
against `go.wasm.mod`, and every byte figure from `wc -c` on a real artifact.
Nothing in `pkg/` or `packages/` was changed — the probes were injected with
`go build -overlay`, so the repo tree is byte-for-byte untouched.

Context: TODO.md P0-8 / P0-9. A valid cluster token authenticates as
`system:masters` (`pkg/apiserver/auth.go`) and short-circuits RBAC
(`pkg/apiserver/rbac.go`), the k3s agent presents that same token, and watch
has no working RBAC. Inviolable rule #3 says reach for upstream's real
implementation first. This document answers **whether each upstream piece can
physically be linked**, and what it costs.

## Method

Baseline and every probe were built exactly the way `Makefile`'s
`$(ASSETS)/apiserver.manifest.json` recipe builds the shipped chunk:

```
GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm go build -tags leanwidth \
  -ldflags="-s -w" -trimpath -o <out> ./pkg/apiserver/cmd/apiserver-wasm
wasm-opt -Oz --strip-debug --strip-producers --enable-bulk-memory \
  --enable-nontrapping-float-to-int --enable-sign-ext --enable-mutable-globals \
  <out> -o <out>.opt
```

The gated number is the **wasm-opt output size** against the Worker Loader's
`CAP := 67108864` (Makefile). The raw size is reported too because it is what
you get back in seconds, but it is not what the manifest recipe checks.

Probe programs are one extra `package main` file added to
`pkg/apiserver/cmd/apiserver-wasm` through `-overlay`, each constructing the
upstream type into a package-level var and calling one method on it, so the
linker cannot drop it. Mirror files (`.build/clientgo-lean-mirror/...`,
`.build/component-base-mirror/...`) were likewise replaced via overlay, never
edited — overlay's GOMODCACHE restriction does not apply to these local
`replace` directories.

**Baseline, re-measured on this checkout** (not taken from the brief, which
said 48,769,800 — that figure is stale):

| | bytes |
|---|---|
| raw | 57,791,960 |
| wasm-opt -Oz | **48,784,426** (matches `assets/wasm/apiserver.manifest.json`) |
| headroom under the 64MiB cap | **18,324,438** (17.47 MiB) |

wasm-opt on this binary takes ~35 s; the ratio opt/raw is 0.844.

## Results at a glance

All sizes in bytes. Δ is against the baseline's **opt** size.

| Build | raw | opt | Δ opt | headroom left |
|---|---|---|---|---|
| baseline (`-tags leanwidth`) | 57,791,960 | 48,784,426 | — | 18,324,438 |
| \+ bootstrap token authenticator | 57,816,760 | 48,806,647 | **+22,221** | 18,302,217 |
| \+ 4 clientset groups, no auth code (control) | 57,804,407 | 48,793,601 | +9,175 | 18,315,263 |
| \+ bootstrap + Node authorizer + graph populator | 59,131,512 | 49,853,717 | **+1,069,291** | 17,255,147 |
| \+ NodeRestriction as well (full stack, narrow width) | 61,386,429 | 51,716,450 | **+2,932,024** | 15,392,414 |
| baseline **without** `-tags leanwidth` (full clientset width) | 73,901,568 | 60,760,047 | +11,975,621 | 6,348,817 |
| full stack at full clientset width | 76,172,191 | 62,632,317 | +13,847,891 | 4,476,547 |
| `endpoints/filters` (WithAuthorization only) | 80,483,157 | 68,542,039 | +19,757,613 | **−1,433,175 — OVER CAP** |

Raw-only side measurements (no wasm-opt pass), all at full clientset width, to
separate the packages from each other: Node authorizer alone +1,092,434;
\+ graph populator +1,297,882; NodeRestriction alone +1,881,926; all three plus
bootstrap +2,270,623. The combined figure is smaller than the sum because the
internal API packages (`pkg/apis/{certificates,coordination,resource,storage}`)
and `pkg/auth/nodeidentifier` are shared.

## Package by package

### 1. Bootstrap token authenticator — builds today, costs 22 KB

**Import path correction.** The brief says
`k8s.io/apiserver/plugin/pkg/authenticator/token/bootstrap`. That does not
exist: the apiserver staging module ships only `oidc`, `tokentest` and
`webhook` under `plugin/pkg/authenticator/token/`. The bootstrap authenticator
is at **`k8s.io/kubernetes/plugin/pkg/auth/authenticator/token/bootstrap`**.

Builds unmodified under `-tags leanwidth` with **no mirror changes at all**.
The whole new dependency set is `k8s.io/cluster-bootstrap/{token/api,
token/util,util/secrets,util/tokens}` plus `k8s.io/client-go/listers/core/v1`
(already linked). `k8s.io/cluster-bootstrap` already has a `replace` in
`go.wasm.mod` (line 42). 22,221 opt bytes — 0.12% of the remaining headroom.

**What it needs that this build does not have.**
`bootstrap.NewTokenAuthenticator` takes a `corev1listers.SecretNamespaceLister`
over `kube-system`. That interface is just `List(labels.Selector)` +
`Get(name)` (`SecretNamespaceListerExpansion` is `interface{}`), so it can be
satisfied by a synchronous adapter over the existing secrets `ResourceStore` —
**no informer, no cache, no background goroutine**, which matters because
`pkg/apiserver` has none of those today (`grep -n informer pkg/apiserver/*.go`
returns 15 hits, all of them comments) and cost invariants #1/#3 forbid adding
a polling one.

**What it does and does not solve.** It authenticates
`bootstrap.kubernetes.io/token` Secrets into user
`system:bootstrap:<token-id>`, group `system:bootstrappers`. That is a
*join-time* credential whose entire point is to be traded for a client
certificate via CSR + kubelet TLS bootstrap. This stack has no kubelet client
certificate path — `cmd/agent` joins with the cluster token over HTTPS — so
adopting the authenticator without the CSR half gives a second short-lived
shared secret, not per-node identity. It is still worth having (P0-8's step 2
wants a node-scoped credential distinct from the admin token), but it is the
front half of a mechanism whose back half does not exist here.

### 2. Node authorizer — does NOT build under `-tags leanwidth`; buildable at +1.07 MB with a narrow-width mirror change

First error, verbatim, plain `-tags leanwidth`:

```
# k8s.io/client-go/informers/storage/v1
.build/clientgo-lean-mirror/informers/storage/v1/csidriver.go:75:19: client.StorageV1 undefined (type kubernetes.Interface has no field or method StorageV1)
# k8s.io/client-go/informers/resource/v1
.build/clientgo-lean-mirror/informers/resource/v1/deviceclass.go:75:19: client.ResourceV1 undefined (type kubernetes.Interface has no field or method ResourceV1)
# k8s.io/client-go/informers/certificates/v1beta1
.build/clientgo-lean-mirror/informers/certificates/v1beta1/certificatesigningrequest.go:75:19: client.CertificatesV1beta1 undefined (type kubernetes.Interface has no field or method CertificatesV1beta1)
```

The cause is `graph_populator.go`, which lives in the same package as
`node_authorizer.go` and imports `client-go/informers/{core/v1, storage/v1,
resource/v1, certificates/v1beta1}`. Each of those packages' generated
`NewFilteredXInformer` calls `client.StorageV1()` etc. on a
`kubernetes.Interface` that `pkg/clientgo-lean-overlays/kubernetes/
clientset_leanwidth.go` narrows to seven groups. Go type-checks the whole
`node` package, so referencing only `NewGraph`/`NewAuthorizer` does not help —
probed, same error.

**This is the failure mode the repo has been bitten by before**, and
`clientset_leanwidth.go`'s own doc comment records a `leanwidth,schedwidth`
widening attempt that was reverted 2026-07-07 because widening the Interface
cascaded into every unpruned sibling API version.

**Measured this time, that cascade does not happen for these four groups.**
Adding `StorageV1()`, `ResourceV1()`, `CertificatesV1beta1()` and `EventsV1()`
to the leanwidth `Interface` (overlay on the mirror, panic-stub
implementations) makes the `node` package compile with no sibling breakage in
*this* binary, and the width change on its own costs **9,175 opt bytes** —
essentially free — **and the reason bounds how far this generalises**. All four
API type packages the widened accessors reference are *already* in the baseline
link graph, because this apiserver serves those groups:

```
$ grep -E '^k8s.io/api/(storage/v1|resource/v1|certificates/v1beta1|events/v1)$' deps-baseline.txt
k8s.io/api/resource/v1
k8s.io/api/storage/v1
k8s.io/api/events/v1
k8s.io/api/certificates/v1beta1
```

So the widening adds four generated typed-client packages over API types that
were paid for already. The 2026-07-07 finding is not wrong; it was measured on
the KCM/scheduler binaries, which do *not* serve these groups and therefore
would pay for the API types as well as the clients, and whose import graphs
reach far more sibling versions. Recording the difference rather than
overwriting it (rule #4).

The cascade *does* reappear the moment a group-level accessor is added — see
NodeRestriction below.

With that width change, bootstrap + `node.NewGraph` + `node.NewAuthorizer` +
`node.AddGraphEventHandlers` links at **49,853,717 opt (+1,069,291)**.

**Semantics: compatible.** `node_authorizer.go` imports
`k8s.io/kubernetes/pkg/apis/{core,storage,resource,coordination,certificates}`
but uses them only for `GroupResource` constants (`api.Resource("pods")`,
`coordapi.Resource("leases")`, `api.NamespaceNodeLease`). It never type-asserts
an object. It decides purely on `authorizer.Attributes` — strings — so the fact
that this apiserver registers external versions only does not break it.

**What it needs that this build does not have — and this, not bytes, is the
blocker.** A `node.Graph` is meaningless until it is populated, and
`AddGraphEventHandlers` populates it from six shared informers (Nodes, Pods,
PersistentVolumes, VolumeAttachments, ResourceSlices, PodCertificateRequests).
The apiserver dynamic worker has no informer machinery, no watch client, and
per `pkg/apiserver/cmd/apiserver-wasm/main.go` deliberately no background
goroutines. Resident informers over six resources would also be exactly the
"fixed-interval resident work" cost invariants #1 and #3 prohibit. Linking the
package is cheap; **feeding it is an architecture change**, and no amount of
byte budget answers that question.

### 3. NodeRestriction admission — builds at +1.86 MB more, but is semantically wrong here

First error, verbatim, under `-tags leanwidth` (with the width fix from §2
already applied — the width fix does not help):

```
# k8s.io/kubernetes/plugin/pkg/admission/noderestriction
.build/k8s-js-mirror/plugin/pkg/admission/noderestriction/admission.go:118:19: f.Core undefined (type informers.SharedInformerFactory has no field or method Core)
.build/k8s-js-mirror/plugin/pkg/admission/noderestriction/admission.go:121:25: f.Storage undefined (type informers.SharedInformerFactory has no field or method Storage)
```

`SetExternalKubeInformerFactory` calls `f.Core().V1()...` and
`f.Storage().V1()...`, and under `leanwidth`
`pkg/clientgo-lean-overlays/informers/factory_leanwidth.go` replaces the whole
`SharedInformerFactory` with a two-method stub (`ForResource`, `Start`).

Adding `Core()` and `Storage()` accessors to that stub reproduces the 2026-07-07
cascade exactly:

```
# k8s.io/client-go/informers/storage/v1alpha1
.build/clientgo-lean-mirror/informers/storage/v1alpha1/csistoragecapacity.go:76:19: client.StorageV1alpha1 undefined (type kubernetes.Interface has no field or method StorageV1alpha1)
# k8s.io/client-go/informers/storage/v1beta1
.build/clientgo-lean-mirror/informers/storage/v1beta1/csidriver.go:75:19: client.StorageV1beta1 undefined (type kubernetes.Interface has no field or method StorageV1beta1)
```

The group-level `informers/storage/interface.go` exposes V1 / V1alpha1 /
V1beta1. The repo already owns the fix for this shape: a build-tagged,
version-narrowed group interface, exactly like
`pkg/clientgo-lean-overlays/informers/storage/interface_schedwidth.go`. Adding a
`leanwidth` twin of that file (V1 only) makes the full stack link:
**61,386,429 raw / 51,716,450 opt (+2,932,024 over baseline, 15.4 MiB
headroom left)**.

**Semantics: broken as-is.** `admitPodCreate` does

```go
pod, ok := a.GetObject().(*api.Pod)   // api = k8s.io/kubernetes/pkg/apis/core (INTERNAL)
if !ok {
    return admission.NewForbidden(a, fmt.Errorf("unexpected type %T", a.GetObject()))
}
```

and the same assertion pattern runs for PVCs, Leases, CSRs and
ServiceAccounts. This apiserver's admission chain handles **external** objects —
`pkg/apiserver/admission.go` works on `*corev1.Namespace` and friends, and
`gen-apiserver-js-mirror.ts`'s installer patch exists precisely because the
scheme has no internal kinds. So every NodeRestriction path would take the
`!ok` branch and return `Forbidden: unexpected type *v1.Pod`. It would not
restrict kubelet writes; it would **deny all of them**. Making it work needs
internal↔external conversion at the admission boundary (i.e. registering the
internal scheme this build deliberately does not register — the thing the
installer patch was written to avoid), and the byte cost of that was not
measured here.

Also note NodeRestriction needs the same informer-backed listers the Node
authorizer needs (pods, nodes, PVs, PVCs, CSIDrivers, ServiceAccounts), so it
inherits §2's real blocker on top of its own.

### 4. `k8s.io/apiserver/pkg/endpoints/filters` — cannot be linked. Over the cap.

Two independent blockers, both hit before any width question:

```
# k8s.io/apiserver/pkg/util/webhook
.build/apiserver-js-mirror/pkg/util/webhook/authentication.go:63:23: undefined: tracing.WrapperFor
# k8s.io/client-go/tools/events
.build/clientgo-lean-mirror/tools/events/event_broadcaster.go:418:39: client.EventsV1 undefined (type kubernetes.Interface has no field or method EventsV1)
```

`tracing.WrapperFor` is deliberately absent from
`pkg/k8s-js-overlays/component-base/tracing_utils.go` (its doc comment: "a
caller that wants a real exporter should fail to compile here"), because
upstream's `tracing/utils.go` links the OTLP-over-gRPC exporter. This is the
same wall `pkg/apiserver/server.go` already documents at the point where it
inlines `WithRequestInfo`'s six-line body instead of importing the package.

Referencing only `filters.WithAuthorization` does not avoid it — Go compiles
the whole package, and `authentication.go` is in it.

**Measured what it would cost if both blockers were removed.** With `EventsV1`
added to the leanwidth Interface and a no-op `WrapperFor` stub (which does *not*
link the exporter), `filters.WithAuthorization` links at **80,483,157 raw /
68,542,039 opt** — **1,433,175 bytes over the 67,108,864 cap.** The dynamic
worker would not load. The weight is `util/webhook` dragging
`plugin/pkg/authorizer/webhook`, `server/egressselector`, the konnectivity
client and `clientcmd`.

**Conclusion: `endpoints/filters` is out of reach for this binary**, and
`pkg/apiserver`'s hand-written `AuthzMiddleware` is not a rule-#3 violation to
be corrected — it is the only shape that fits. The authorizer *inside* it is
already upstream's (`rbacauthorizer.New`, see §6).

### 5. Small upstream helpers that ARE free

Checked by dependency-set diff against the baseline (`go list -deps`):

| package | new packages pulled in |
|---|---|
| `k8s.io/apiserver/pkg/authorization/union` | 1 (itself) |
| `k8s.io/apiserver/pkg/authentication/request/union` | 1 (itself) |
| `k8s.io/apiserver/pkg/authentication/request/bearertoken` | 1 (itself) |
| `k8s.io/apiserver/pkg/authorization/authorizerfactory` | **~180** — cel-go, antlr, grpc, konnectivity, clientcmd, `util/webhook` |

So a Node+RBAC authorizer union, and a bearer-token/union authenticator chain,
cost nothing. `authorizerfactory` (which is only needed for the webhook and
"always allow/deny" constructors) must be avoided — it is the same chain that
puts §4 over the cap.

### 6. Two claims in the brief that the code contradicts

**"The RBAC authorizer proper" is already in use.** `pkg/apiserver/rbac.go`
imports `rbacauthorizer "k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac"`
and `.../rbac/bootstrappolicy`, and `NewRBACAuthorizer` returns
`rbacauthorizer.New(p, p, p, p)` over this apiserver's own stores unioned with
upstream's bootstrap policy. There is nothing to adopt here. The defect P0-8
describes is not a reimplemented authorizer — it is that `auth.go` hands the
real authorizer a `system:masters` identity, which `authorizeRequest` then
short-circuits with upstream's own `SystemPrivilegedGroup` bypass. **The fix is
in identity minting, not in the authorizer.**

**`APIGroupVersion.Authorizer` is not the watch-RBAC lever.** The field exists
(`.build/apiserver-js-mirror/pkg/endpoints/groupversion.go:92`) and
`installer.go:667` wires it to `scope.Authorizer`, but its own doc comment says
"The Handler does a **preliminary** authorization check using the request URI
but it may be necessary to make **additional** checks, such as in the
create-on-update case". Grepping the mirror, `scope.Authorizer` is read in
exactly three places: `update.go` (create-on-update admission attributes),
`patch.go`, and `delete.go` (the unsafe-delete path, which errors out with "no
authorizer provided" when it is nil). Setting it buys correct behaviour on
those three paths. **It does not gate ordinary requests, and it cannot gate
watch** — the per-request gate is `filters.WithAuthorization`, which §4 shows
cannot be linked.

**And watch never reaches this binary anyway.**
`packages/k8flare-worker/src/gateway/index.ts:398-400` handles watch entirely in
TypeScript (`handleWatch` from `src/k8s`), so no Go-side authorizer wiring can
affect it. Note what `authorizeWatchRBAC` already does, though: it builds a
SubjectAccessReview and posts it to the apiserver. That path is sound — the SAR
is answered by the real `rbacauthorizer` via `authorizeRequest`. Its only defect
is the identity: it reads `X-Remote-User`, which nothing sets. **P0-9 needs an
identity source, not a new upstream package.**

## Recommendation, in order

1. **Bootstrap token authenticator first** (22 KB, no mirror changes, no
   architecture change). Back it with a synchronous `SecretNamespaceLister`
   adapter over the secrets `ResourceStore`. Pair it with
   `authentication/request/{union,bearertoken}` (free) so the cluster token, SA
   JWTs and bootstrap tokens become three peers behind one chain instead of
   three branches in `AuthMiddleware`. This is the cheapest real step toward
   P0-8: a credential that is *not* the admin token.
2. **Then stop minting `system:masters` from the node credential** — pure
   `pkg/apiserver/auth.go` + vault work, zero bytes, and it is what actually
   closes P0-8. Everything below is optional hardening on top of it; this is
   not.
3. **Then P0-9, with no new upstream packages and no identity logic in TS.**
   The gateway already relays one review call to Go (`authorizeWatchRBAC`
   posts a SubjectAccessReview); the missing half is the identity, and Go
   already owns an endpoint for that: `pkg/apiserver/tokenreview.go` serves
   `POST /apis/authentication.k8s.io/v1/tokenreviews`. Today it answers only
   the cluster token (`tokenMatches` → `admin` / `system:masters`); extend it
   to run the same chain `AuthMiddleware` runs — the ServiceAccount JWT
   authenticator, and the bootstrap authenticator from step 1 — so one Go call
   resolves any presented bearer token to a user/groups pair. `handleWatch`'s
   `dwAuth` gets SA-JWT support from the same call. The gateway then relays
   two reviews (TokenReview → SubjectAccessReview) and parses no tokens
   itself, which is what Go-first requires (ビジネスロジックを TS に書かない).
   Re-enable the two `TestRBACEnforcement` watch assertions against that
   identity. Cost of the extended TokenReview handler was not measured, but it
   adds no packages: `authenticationv1` and the SA authenticator are already
   linked.
4. **Node authorizer: only if informers arrive.** The width change is
   measurable and modest (+1.07 MB opt, 17.2 MiB headroom left), and it is
   semantically compatible with external-typed storage. But the graph needs six
   shared informers that this dynamic worker does not have and — under cost
   invariants #1/#3 — cannot simply grow. Decide the informer question on its
   own merits first; the byte budget is not the constraint.
5. **NodeRestriction: not as-is.** Even at +2.93 MB total it would deny every
   kubelet write, because it asserts internal `pkg/apis/core` types against an
   apiserver that serves external ones. Revisit only together with a decision
   about registering internal kinds — which the installer patch in
   `gen-apiserver-js-mirror.ts` exists to avoid.
6. **Never `endpoints/filters` or `authorizerfactory`** in this binary, unless
   `util/webhook` is severed the way DRA/CEL was for the scheduler. Measured:
   68,542,039 opt, 1.4 MB over the hard cap.

## What was NOT verified

- **Nothing was run.** Every result is a compile/link/size measurement. No
  `wrangler dev`, no `go test`, no conformance run (a conformance harness owned
  this checkout during the investigation). "It links at 51.7 MB" is not "it
  loads and serves"; the Loader's behaviour at 51.7 MB — three chunks rather than
  today's two, since `chunk-wasm.ts` splits at 25,165,824 bytes — is untested
  here.
- The claim that a synchronous `SecretNamespaceLister` over `ResourceStore`
  satisfies the bootstrap authenticator is from reading the interface (two
  methods, empty expansion) — no adapter was written or exercised.
- The +9,175-byte "widening is free" result is specific to *this* entrypoint's
  import graph. It says nothing about kcm/gc/sched, where the 2026-07-07
  cascade was originally measured.
- NodeRestriction's cost *with* internal-type registration was not measured;
  only the (semantically broken) external-scheme link was.
- The extended TokenReview handler in recommendation 3 was not built or sized.
  It is asserted to add no packages because `k8s.io/api/authentication/v1` and
  the ServiceAccount authenticator are already in the baseline dependency set —
  checked with `go list -deps`, not by building the handler.
- Mirror overlays were applied at build time only. If `make gen-mirrors` runs
  concurrently the mirrors are regenerated from the committed tree, which does
  not invalidate these numbers but would break a probe mid-build.
