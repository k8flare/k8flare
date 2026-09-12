# Security Policy

## Reporting a vulnerability

Please report privately — do not open a public issue, and do not include
details in a pull request.

Use GitHub's **Security** tab → **Report a vulnerability** to open a private
security advisory. If that entry isn't available to you, open a normal issue
that says only that you have a security report and asks for a private
channel — no details, no reproducer.

This project has a single maintainer and no response SLA. Expect an
acknowledgement on a best-effort basis. Fixes land on `main`; there is no
backport process (see below).

## Supported versions

| Version | Supported |
|---|---|
| `main` | Yes |
| anything else | — |

There are no releases and no tags yet. `main` is the only thing that
exists, and it is the only thing that gets fixes.

## Current security posture

k8flare is **pre-production**. The authentication model is deliberately
simple and has not been hardened for multi-tenant or hostile use. Treat a
deployment as trusted-operators-only, and don't put anything on it you
would mind losing. The specifics, so you can judge for yourself:

**One token, full admin.** A valid cluster token authenticates as `admin`
in the `system:masters` group (`pkg/apiserver/auth.go:98`). That group
short-circuits authorization before any RBAC rule is consulted
(`pkg/apiserver/rbac.go:258`), the same bypass upstream kube-apiserver has
for its privileged group. RBAC therefore constrains only *derived*
identities — ServiceAccount tokens and `X-Remote-User` from a TLS-terminating
proxy. The `k8flare:cluster-admin` / `k8flare:cluster-secret-reader` roles
exist for those derived identities; they do not confine a token holder.

**The node-join credential is the same credential.** The k3s agent joins
with HTTP Basic `node:<token>` and is authenticated into the `system:nodes`
group (`pkg/apiserver/auth.go:112-118`) — but the token it presents is the
cluster token, so anyone who can read a node's configuration holds an admin
credential. There is no per-node identity yet; kubelets are authorized by
the pre-1.8 group bindings (`system:node`, `system:node-proxier`) rather
than the Node authorizer plus NodeRestriction admission
(`pkg/apiserver/rbac.go`). TLS client certificates per node are a
follow-up, not a shipped feature.

**Nothing strips `X-Remote-User` / `X-Remote-Group` from an inbound
request.** Those headers are honoured as the caller's identity
(`pkg/apiserver/auth.go:88-97`), on the stated assumption that a
TLS-terminating proxy sets them from a client certificate. No component in
this repository sets them, and the gateway only reads them
(`packages/k8flare-worker/src/gateway/index.ts:100,117`) — so today any
holder of a cluster token can name themselves any user in any group. That is
not an escalation *at present*, because a cluster token is already
`system:masters`; it becomes one the moment tokens carry distinct roles, so
it has to be closed before that work, not after (`TODO.md` P0-8). If you put
a TLS-terminating proxy in front of this, it must overwrite both headers on
every request rather than pass them through.

**A publicly documented dev token can be live.** The *default* cluster
falls back to accepting the well-known token `k8flare-dev-token` when its
vault holds no minted token **and** the `K3S_TOKEN` Worker secret is unset
(`packages/k8flare-worker/src/clusters/tokens.ts:100-102`). The Go side and
CI depend on this fallback for local development. Set `K3S_TOKEN` before
exposing any deployment, and verify it took effect — with neither a secret
nor a minted token, your cluster is wide open to anyone who has read this
file.

**Revocation lags up to ~60 seconds.** Accepted token secrets are cached
per isolate with a 60s TTL (`TOKEN_CACHE_TTL_MS`,
`packages/k8flare-worker/src/clusters/tokens.ts`). Deleting a token stops
new authentications within that window, not immediately. The documented
emergency measure is deleting the cluster.

**Cluster tokens are stored in plaintext**, as one value in the cluster's
`ca-vault` facet. This is deliberate: the NodeVM path injects the real token
into microVMs and the kubeconfig endpoint re-serves it, and the same vault
already holds the CA private keys — so it adds no new exposure class, but it
does mean vault access is total access.

**Local development runs over plain HTTP.** `wrangler dev` serves HTTP, so
tokens travel unencrypted on localhost. Any real deployment is behind
Cloudflare's TLS termination.

None of the above is a vulnerability report — they are known, documented
properties of the current design. A report that one of them is *worse than
described*, or a way to authenticate or escalate that isn't covered here,
is exactly what this policy is for.
