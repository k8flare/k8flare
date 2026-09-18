# Security Policy

## Reporting a vulnerability

Report privately. Do not open a public issue with a reproducer.

Use GitHub's **Security** tab → **Report a vulnerability**. If that is
unavailable, open an issue that says only that you have a security report
and asks for a private channel.

There is one maintainer and no response SLA. Fixes land on `main`. There
are no releases and no backport process.

## Supported versions

| Version | Supported |
|---|---|
| `main` | Yes |
| anything else | — |

## Current security posture

k8flare is **pre-production**. Treat a deployment as trusted-operators-only.
Do not put data on it you would mind losing. The authentication model is
deliberately simple and is not multi-tenant.

**One Worker, one cluster, three secrets.** `ADMIN_TOKEN`, `READONLY_TOKEN`,
and `JOIN_TOKEN` are Worker-wide (`wrangler secret put`). A valid
`ADMIN_TOKEN` authenticates as `admin` in `system:masters` and short-circuits
RBAC. `READONLY_TOKEN` is an authenticated user with no privileged group;
it only sees what RBAC grants. There is no per-tenant vault.

**`JOIN_TOKEN` is nearly as powerful as admin for join.** It unlocks
`/v1-k3s/*` (except `/v1-k3s/connect`). Kubelet CSRs require a registered
node password. `client-kube-proxy.crt` and `client-k3s-controller.crt` are
signed for anyone who holds the join token. `/cacerts` and `/ping` are
unauthenticated (k3s-compatible).

**Node identity is a password hash in the Cluster Durable Object**, outside
the `/registry` keyspace kubectl can list. The cluster CA private keys live
in the same store in PEM. Compromising the Worker or the Durable Object is
total compromise. The `Storage` entrypoint is an unauthenticated proxy to
that store; it must stay a Service Binding and must never be routed
publicly.

**Group workers trust `X-Remote-User`.** The front apiserver authenticates
and forwards identity over a Service Binding. Do not expose the `APIGroups`,
`CustomResources`, `Workloads`, `Scheduler`, `GarbageCollector`, or
`Storage` entrypoints on the public Internet.

**Kubelet access defaults to insecure.** If `KUBELET_CLIENT_CERT` /
`KUBELET_CLIENT_KEY` are unset, logs and exec fall back to
`InsecureSkipVerify` and `ADMIN_TOKEN`. Set the kubelet client material
before any real deployment.

**The default `*.workers.dev` URL is on the public Internet.** Protection
is the strength and rotation of the three secrets. Cloudflare Access in
front of the Worker is recommended for anything beyond a lab.

**Local development is HTTP.** `wrangler dev` sends tokens in the clear on
localhost.

None of the above is a vulnerability report. A report that one of them is
worse than described, or a way to authenticate or escalate that is not
covered here, is what this policy is for.
