# S18: real RBAC enforcement in the WASM apiserver — findings

Goal (task #17): what it actually takes to enforce RBAC (and mint real
ServiceAccount tokens) with **upstream code, not a reimplementation**,
inside the `workers/apiserver` Go WASM binary, without breaking the
10MiB-gzip Workers deploy cap (baseline today: 43,387,694 raw /
8,168,465 gzip).

All numbers below are measured builds of the real `./workers/apiserver`
target with the candidate packages force-linked (temp file, deleted
after measurement), Go 1.26.4, GOOS=js GOARCH=wasm, 2026-07-06.

## Gate 1 — upstream RBAC authorizer compiles AND runs on js/wasm: PASS

`k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac` (RBACAuthorizer) +
`pkg/registry/rbac/validation` compile for js/wasm and **run correctly**
(`main.go` here, executed under Node wasm_exec): fed the bootstrap
policy, `admin` in group `system:masters` gets
`Allow — RBAC: allowed by ClusterRoleBinding "cluster-admin" of
ClusterRole "cluster-admin" to Group "system:masters"`. So the existing
cluster token (mapped to `system:masters` by `auth.go`) keeps full
access the day enforcement turns on — no migration cliff for kubectl,
KCM, or the scheduler (all authenticate as that identity).

The authorizer's inputs are four tiny interfaces (`GetRole`,
`GetClusterRole`, `ListRoleBindings`, `ListClusterRoleBindings`) —
implementable directly over `pkg/apiserver`'s own `ResourceStore`s, no
client-go informers needed.

Size: **+241KB raw / +31KB gzip** over baseline. Negligible.

`k8s.io/apiserver/pkg/endpoints/request.RequestInfoFactory` (URL →
verb/resource/namespace attributes, the same code the real apiserver
uses) adds another **+2KB gzip**. Also negligible.

## Gate 2 — linking `bootstrappolicy` directly: FAIL (size)

`plugin/pkg/auth/authorizer/rbac/bootstrappolicy` alone blows the cap:
61,167,145 raw / **10,790,982 gzip (> 10,485,760 cap)** — +2.62MiB gzip
for what is conceptually just data. Cause (dep diff, 227 new packages):
it imports `pkg/controlplane/controller/legacytokentracking`, which
drags the full `k8s.io/client-go/kubernetes` clientset (every
alpha/beta API group), go-restful, etc.

**Viable path instead**: run `bootstrappolicy.ClusterRoles()/
ClusterRoleBindings()/NamespaceRoles()/NamespaceRoleBindings()` in
`cmd/k8flare-gen` (host build — size irrelevant there), emit the result
as generated data, and seed it in `BootstrapCluster` exactly like the
baseline namespaces/ServiceAccounts already are. Upstream data, upstream
evaluator, zero WASM bloat.

## Gate 3 — linking `pkg/serviceaccount` (JWT) directly: FAIL (size)

`k8s.io/kubernetes/pkg/serviceaccount` alone: **+2.68MiB gzip → over
the cap** (10,879,090 combined with the Gate-1 pieces). Cause: it
imports `k8s.io/client-go/kubernetes/typed/core/v1`,
`client-go/applyconfigurations`, `component-base/metrics/legacyregistry`
(prometheus) and `apiserver/pkg/audit`.

**Viable path instead**: `gopkg.in/go-jose/go-jose.v2` directly — the
exact JWT library upstream `pkg/serviceaccount` itself uses, already in
go.mod. Mint/verify tokens with the upstream claim shape
(`iss`/`sub`=`system:serviceaccount:<ns>:<name>`/`aud`/`exp` +
`kubernetes.io` private claims) — a small data-shape port, same
precedent as the Job `generateSelectorIfNeeded` port. Measured:
authorizer + RequestInfoFactory + go-jose sign/parse =
44,318,602 raw / **8,353,946 gzip = +185KB total, ~2MiB headroom**.

## What enforcement actually requires (wiring survey, verified in source)

1. **Go apiserver (CRUD)**: authz middleware after `AuthMiddleware`
   (`workers/apiserver/main.go:172`), RequestInfoFactory → RBACAuthorizer
   backed by the rbac ResourceStores. `SelfSubjectAccessReview` /
   `SubjectAccessReview` handlers (`selfsubjectaccessreview.go`) stop
   hard-coding `Allowed: true` and consult the same authorizer — their
   doc comments already mark them as the plug-in point.
2. **Watch does NOT go through the Go apiserver** — `?watch=true` is
   served entirely by gateway TS (`workers/gateway/src/index.ts:83` →
   `packages/k8s/src/watch.ts:126`), authenticated by `dwAuth` (token
   equality) with **no authorization**. Same for the `workers/runtime`
   CRD path. Fix shape: before opening the stream, gateway/runtime POST
   a `SubjectAccessReview` to the apiserver over the existing service
   binding — the real webhook-authorizer pattern, already proven here by
   the kubelet bridge (tokenreview.go, observed live 2026-07-06, kubelet
   caches verdicts ~2m). One SAR per watch-open is the only added cost.
3. **`system:nodes` needs an explicit binding**: upstream
   bootstrappolicy deliberately does NOT bind the `system:node`
   ClusterRole to the `system:nodes` group (the Node authorizer took
   over upstream; there is no Node authorizer here). k3s ships its own
   bindings for its agent identity — k8flare must do the same for
   `auth.go`'s `node` identity (`k3s:agent`, `system:nodes`) or agents
   break the moment enforcement turns on.
4. **TokenRequest subresource** (`POST .../serviceaccounts/<name>/token`)
   for kubelet-projected SA tokens (stock kubelet on per-Pod Containers
   nodes will call it) and CoreDNS (Phase 4 step 2). RequestInfoFactory
   already parses the `token` subresource; signing key belongs next to
   the CA material (Cluster DO ca-vault facet). TokenReview
   (tokenreview.go) then validates minted JWTs in addition to the
   cluster token.
5. **Cost note (invariant #5)**: the authorizer getters read RBAC
   objects from DO storage — naive per-request reads multiply
   rows-read on every API call. Needs an in-isolate cache invalidated
   by kine revision before shipping; record actuals in
   docs/cost-model.md at implementation time.

Prereqs already in place (no work needed): rbac.authorization.k8s.io/v1
CRUD+watch (`apidef/table.go`, `TestRBACGroup`), ServiceAccount / Secret
/ ConfigMap stores (`apidef/table.go:159,167,232`), baseline
namespace+SA seeding (`BootstrapCluster`), identity model with groups
(`auth.go` `UserInfo`).

## Addenda (parallel source survey, 2026-07-06)

- **Mirror reproducibility**: `k8s.io/kubernetes` resolves via the
  `.build/k8s-js-mirror` replace (go.mod:71). Non-issue for this spike:
  `scripts/gen-k8s-js-mirror.sh` copies the FULL upstream module (only
  two overlay files swapped, no package pruning), so the RBAC packages
  survive regeneration; the compile runs above also prove they exist in
  the current on-disk mirror. (CLAUDE.md's OPEN REGRESSION concerns the
  separate, pruned clientgo-lean-mirror used by the KCM build.)
- **`SelfSubjectRulesReview` is not implemented** (no handler, not in
  discovery) — `kubectl auth can-i --list` needs it. Cheap once the real
  authorizer exists: `rbacregistryvalidation.AuthorizationRuleResolver`
  (already linked, Gate 1) is exactly what computes the rule list.
- **Implementation shape notes** from the resource-coverage survey:
  TokenRequest is a subresource on a stored type → goes through
  `subresource.go`'s dispatch (gated by `apidef.HasSubresource`) plus a
  scheme.go registration for the TokenRequest type; the SAR/SSAR/
  TokenReview trio are compute-on-request types needing the 3-piece
  pattern (dedicated handler + scheme.go + hand-written discovery.go
  entry) — all three pieces already exist, only their verdicts change.
- Gateway double-gates auth (TS `dwAuth` at watch/proxy entry, Go
  `AuthMiddleware` for CRUD); pods/proxy and nodes/proxy
  (`workers/gateway/src/index.ts:33,57`) are additional TS-only paths
  that need the same SAR treatment as watch when enforcement lands.
