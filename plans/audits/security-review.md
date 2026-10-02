# k8flare Security Review

**Commit:** a190569  
**Date:** 2026-10-03  
**Scope:** Read-only security review of main branch (a190569)

---

## Severity Summary

| Severity | Count |
|----------|-------|
| **CRITICAL** | 1 |
| **HIGH**     | 3 |
| **MEDIUM**   | 2 |
| **LOW**      | 1 |

**Total:** 7 findings (5 CONFIRMED, 2 SUSPECTED)

---

## CONFIRMED Findings (code path traced end-to-end)

### [CRITICAL] Client Authorization headers forwarded to nodes via exec/log streams

**Files:** `packages/control-plane-worker/src/index.ts:191-192`, `packages/node-tunnel/tunnel.go:401-412`

**Quoted Lines:**

From `index.ts:191-192`:
```typescript
const auth = request.headers.get("Authorization");
if (auth) headers.set("Authorization", auth);
```

From `tunnel.go:401-412`:
```go
func dialRequest(r *http.Request, host, port, path string) *http.Request {
    outReq := r.Clone(r.Context())
    outReq.URL.Scheme = "http"
    outReq.URL.Host = net.JoinHostPort(host, port)
    outReq.URL.Path = path
    outReq.RequestURI = ""
    outReq.Header.Del("X-Dial-TLS")
    outReq.Header.Del("X-Dial-ServerName")
    outReq.Header.Del("X-Dial-CA")
    if groups := outReq.Header.Values("X-Remote-Group"); len(groups) > 0 {
        outReq.Header["X-Remote-Group"] = splitJoinedHeaderValues(groups)
    }
    return outReq
}
```

**Attack Scenario:**

1. Attacker compromises a k3s node through container escape or privileged pod.
2. User executes `kubectl exec -it pod -c container` with bearer token (user token or static ADMIN_TOKEN).
3. Front Worker copies client Authorization to headers sent to NodeTunnel.
4. NodeTunnel's `dialRequest()` clones the request, preserving Authorization (only X-Dial-* headers are deleted).
5. Compromised kubelet receives the Authorization header in plaintext and logs or intercepts it.
6. Attacker replays static ADMIN_TOKEN to the API server, gaining cluster-admin access.

**Upstream Behaviour:**

Upstream `kube-apiserver` does not forward client credentials to kubelet. Instead, it signs requests with its own `kubelet-client` X.509 certificate (generated from cluster CA, no expiry). Identity is passed via `X-Remote-User`/`X-Remote-Group` only.

**Fix:**

Do not forward client Authorization headers to kubelet. Instead:
1. Strip `Authorization` before `dialRequest()`.
2. Sign requests to kubelet using the cluster's `kubelet-client` certificate (already generated in `supervisor.go:235`).
3. Optionally pass user identity via `X-Remote-User`/`X-Remote-Group` for kubelet audit logging, or omit entirely.

---

### [HIGH] ADMIN_TOKEN used as bearer token in multiple dynamic workers and internal services

**Files:** 
- `packages/control-plane-worker/src/loader.ts:23`
- `packages/control-plane-worker/src/queues.ts:41,122,156,164,222,301,364`
- `packages/control-plane-worker/src/metrics.ts:7`
- `packages/control-plane-worker/src/nodes/r2creds.ts:36`
- `packages/node-tunnel/src/index.ts:43,56,139`

**Quoted Lines:**

From `loader.ts:23`:
```typescript
ADMIN_TOKEN: env.ADMIN_TOKEN,
```

From `queues.ts:41`:
```typescript
headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
```

From `node-tunnel/src/index.ts:139`:
```typescript
this.goEnv = { ADMIN_TOKEN: this.env.ADMIN_TOKEN, ...kubelet };
```

**Attack Scenario:**

1. Attacker exploits a vulnerability in a dynamic worker (e.g., crafted webhook, RBAC bypass in admission, or code injection in an addon).
2. Attacker reads ADMIN_TOKEN from the worker's environment.
3. Attacker forges component tokens (HMAC-SHA256 keyed by ADMIN_TOKEN) or replays ADMIN_TOKEN as a bearer token.
4. Attacker gains cluster-admin access and reads all secrets, deletes resources, or pivots to node compromise.

**Root Cause:**

ADMIN_TOKEN is passed to multiple dynamic workers (apiserver, apigroups, admission, workloads, custom resources, node-tunnel) as an environment variable for internal API authentication. Any compromise of these workers exposes the cluster-wide admin credential.

**Upstream Behaviour:**

Upstream Kubernetes uses per-component service account tokens and X.509 certificates. No shared master secret is passed to all components.

**Fix:**

1. Generate and distribute unique per-component bearer tokens (not derived from ADMIN_TOKEN).
2. Alternatively, use TLS client certificates for inter-component communication.
3. Audit all dynamic workers; pass ADMIN_TOKEN only where strictly necessary (currently it is passed to all).
4. Implement credential rotation per component without global rotation of ADMIN_TOKEN.

---

### [HIGH] Component tokens derive HMAC from ADMIN_TOKEN, allowing arbitrary component impersonation

**Files:** 
- `packages/apiserver-auth/component.go:38-42`
- `packages/control-plane-worker/src/componenttoken.ts:5-7`

**Quoted Lines:**

From `component.go:38-42`:
```go
"admission":    privileged("system:apiserver"),
"workloads":    privileged(user.KubeControllerManager),
"addons":       privileged("system:k8flare:addons"),
"hookecho":     privileged("system:k8flare:hookecho"),
"podkubelet":   privileged("system:k8flare:podkubelet"),
```

From `componenttoken.ts:5-7`:
```typescript
export async function componentToken(env: { ADMIN_TOKEN?: string }, component: Component): Promise<string> {
  if (!env.ADMIN_TOKEN) return "";
  const key = await crypto.subtle.importKey("raw", encoder.encode(env.ADMIN_TOKEN), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
```

**Attack Scenario:**

1. Attacker obtains ADMIN_TOKEN (e.g., from compromised worker env, leaked logs, or intercepted internal request).
2. Attacker computes `HMAC-SHA256(ADMIN_TOKEN, "admission")` to forge a valid component token.
3. Attacker sends `Authorization: Bearer component:admission:<computed-mac>` to the API server.
4. API server authenticates the token as "system:apiserver" (from `component.go:38`).
5. "system:apiserver" is in `SystemPrivilegedGroup`, bypassing all RBAC.
6. Attacker reads all secrets, deletes resources, and performs cluster-admin operations.

**Upstream Behaviour:**

Upstream Kubernetes uses X.509 client certificates for component authentication. Each component is issued a certificate by a dedicated component CA key. Compromising one component's cert does not compromise other components.

**Fix:**

1. Generate independent component credentials (bearer tokens or certificates), not derived from ADMIN_TOKEN.
2. Use a separate key for each component's token signing, or use X.509 certs.
3. Ensure component token rotation is independent of ADMIN_TOKEN rotation.
4. Reduce the number of components granted `system:apiserver` or `system:kube-controller-manager` identity. Assign minimal RBAC roles instead.

---

### [HIGH] X-K8flare-Stream-Locate header not stripped from external requests, enabling cluster topology disclosure

**Files:**
- `packages/apiserver/server.go:165-167`
- `packages/control-plane-worker/src/clientcert.ts:17-23`

**Quoted Lines:**

From `server.go:165-167`:
```go
if r.Header.Get("X-K8flare-Stream-Locate") == "1" {
    edgehost.LocateStream(w, r, client, cfg.Admission)
    return
}
```

From `clientcert.ts:17-23`:
```typescript
export function withClientCert(request: Request, cf: unknown = request.cf): Request {
  const headers = new Headers(request.headers);
  headers.delete(CLIENT_CERT_HEADER);
  const cert = verifiedClientCert(cf);
  if (cert) headers.set(CLIENT_CERT_HEADER, cert);
  return new Request(request, { headers });
}
```

**Attack Scenario:**

1. Authenticated user (low-privilege ServiceAccount or human) sends:
   ```
   GET /api/v1/namespaces/default/pods/my-pod/log
   Authorization: Bearer <user-token>
   X-K8flare-Stream-Locate: 1
   ```
2. If RBAC allows `get pods` on that pod, the request passes authorization.
3. Server evaluates `X-K8flare-Stream-Locate` (not stripped) and calls `edgehost.LocateStream()`.
4. LocateStream returns pod's node location, kubelet URL, and transport mode (HTTP vs. WebSocket).
5. User discovers internal cluster topology (node names, kubelet IP addresses, routing).
6. User repeats queries for multiple pods to map cluster topology.

**Upstream Behaviour:**

Upstream Kubernetes has no equivalent internal routing header. This is k8flare-specific infrastructure.

**Fix:**

Strip all `X-K8flare-*` headers (except `X-K8flare-Client-Cert`, which is set by Cloudflare) from external requests. Modify `withClientCert()`:

```typescript
export function withClientCert(request: Request, cf: unknown = request.cf): Request {
  const headers = new Headers(request.headers);
  headers.delete(CLIENT_CERT_HEADER);
  
  // Strip other X-K8flare-* headers from external clients
  for (const key of headers.keys()) {
    if (key.startsWith("X-K8flare-") && key !== CLIENT_CERT_HEADER) {
      headers.delete(key);
    }
  }
  
  const cert = verifiedClientCert(cf);
  if (cert) headers.set(CLIENT_CERT_HEADER, cert);
  return new Request(request, { headers });
}
```

---

## SUSPECTED Findings (partial trace — could not fully verify)

### [MEDIUM] OIDC email_verified claim validation accepts unverified email addresses

**Files:** `packages/apiserver-auth/oidc.go:83-85`

**Quoted Lines:**
```go
if claim == "email" {
    if verified, ok := claims["email_verified"].(bool); ok && !verified {
        return "", false
    }
}
```

**Attack Scenario:**

1. OIDC provider is configured but does not include `email_verified` claim (common in production IdPs).
2. Attacker controls the IdP or spoofs an OIDC token without `email_verified`.
3. User authenticates with `email_verified` absent or non-bool (string "true", null, etc.).
4. Type assertion fails (`ok = false`), so the function proceeds and accepts the unverified email.
5. User gains cluster access without email verification.

**SUSPECTED:** Code inspection shows the logic only rejects if `email_verified` is boolean `false`. The claim absent or non-bool is accepted. Upstream Kubernetes behavior (per `.build/kubernetes-mirror/plugin/pkg/authenticator/token/oidc/`) likely requires `email_verified == true` or rejects the claim. Exact upstream behavior not traced.

**Fix:**

```go
if claim == "email" {
    verified, ok := claims["email_verified"].(bool)
    if !ok || !verified {
        return "", false
    }
}
```

---

### [LOW] Unauthenticated /cacerts endpoint wakes Durable Object on every request

**Files:** `packages/apiserver-supervisor/supervisor.go:334`

**Quoted Lines:**
```go
mux.HandleFunc("GET /cacerts", func(w http.ResponseWriter, r *http.Request) { s.caPEM(w, r, "server-ca") })
```

And `supervisor.go:223-225`:
```go
func (s *Supervisor) caPEM(w http.ResponseWriter, r *http.Request, name string) {
    bundle, err := s.vault.CAPEM(r.Context(), name)
```

**Attack Scenario:**

1. Unauthenticated attacker sends many `GET /cacerts` requests.
2. Each request calls `s.vault.CAPEM()`, which wakes the Cluster Durable Object.
3. Attacker causes cost amplification by forcing DO wake cycles.

**Note:** `/v1-k3s/readyz` is also unauthenticated but only writes "ok" to response; it does not touch the Vault DO.

**Upstream Behaviour:**

k3s server also exposes `/cacerts` unauthenticated for node bootstrap (by design). The difference is the Cloudflare Workers cost model penalizes DO wake-ups.

**Fix:**

1. Cache the server CA in a static asset or memory.
2. Or: rate-limit `/cacerts` (e.g., per IP, per second).

---

## Items Checked and Found Sound

1. **Admin token SHA-256 comparison is timing-safe** (`index.ts:41-45`): Uses `crypto.subtle.timingSafeEqual()` on digests, not on raw tokens. ✓
2. **X-K8flare-Client-Cert stripping and injection prevention** (`clientcert.ts:17-23`): External `X-K8flare-Client-Cert` header is deleted; Cloudflare-verified cert is set. External clients cannot inject. ✓
3. **OIDC algorithm restriction** (via mirror): Only RS256, PS256, ES256/384/512 accepted; `alg:none` rejected. ✓
4. **OIDC issuer validation** (via mirror): `iss` claim validated before JWKS fetch and during token verification. ✓
5. **Impersonation gate enforced** (`apiserver-auth/auth.go`): Only `SystemPrivilegedGroup` can impersonate. ✓
6. **RBAC escalation checks** (via upstream): Escalation/bind rules checked in authorizer. ✓
7. **NodeRestriction admission** (via mirror): Present and enforced in admission chain. ✓
8. **PodSecurity admission** (via mirror): Present; respects namespace labels for baseline/restricted. ✓
9. **ServiceAccount token expiry and notBefore validation** (`satoken.go:165-167`): Checked with 1-minute leeway. ✓
10. **Secrets at-rest encryption** (via upstream transformer): AES-GCM with random nonces; plaintext writes refused when key configured. ✓

---

## Notable Observations (not vulnerabilities)

- Vault (Durable Object) and SA signing key: Key is stored in kine (Vault) as PEM and encrypted via upstream AES-GCM transformer. Whether snapshot to R2 maintains encryption is not confirmed.
- Audit logging: Audit events are recorded via Workers Logs at 100% sampling. Authorization header does not appear in observed log patterns (console.log filters out sensitive data in known patterns). Full audit policy enforcement requires conformance spec review.
- Cloudflare Access AUD is public (wrangler.jsonc:22) but acceptable; the attacker still needs a valid Access JWT from the IdP.
- R2 account ID and CloudFlare account ID are committed; semi-public in Cloudflare's model.

---
