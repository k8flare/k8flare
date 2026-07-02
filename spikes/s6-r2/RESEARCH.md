# S6 — R2 as a PV/PVC backend: desk research

Research spike, July 2026. Desk research only (docs + one read-only account
check). No bucket, token, or credential was created against the real
account — see [Residual items](#residual-items-only-answerable-against-a-real-account)
for what still needs a live test.

## Verdict up front

R2 has a purpose-built mechanism for exactly this problem —
**Temporary Access Credentials, scoped to a prefix, minted from a parent
token** — and Cloudflare's own docs steer integrators toward it for
multi-tenant delegated access. Containers can reach R2's S3-compatible
endpoint with zero extra egress configuration (internet access is on by
default), and can even FUSE-mount a bucket if an app needs POSIX-style file
access instead of raw S3 calls. The open problem is not "can we isolate
PVCs on R2" (yes) — it's **credential refresh for long-running Pods**,
since temporary credentials are deliberately short-lived and nothing in the
FUSE example refreshes them in place. See
[Recommendation](#recommendation-for-phase-8).

## 1. PVC-level access isolation

Three mechanisms exist. They are not equivalent — picking the wrong one for
v1 is the main risk here.

| Mechanism | Scope | Lifetime | Prefix-level isolation? |
| --- | --- | --- | --- |
| R2 API token (dashboard/API) | One or more whole buckets | Long-lived (until revoked) | **No** |
| Temporary Access Credentials | One bucket, optionally narrowed | Short-lived (`ttlSeconds`, caller-chosen) | **Yes** (`prefixes` / `objects`) |
| Presigned URL | One object, one S3 operation | Up to 7 days, min 1s | N/A (single object, not a session) |

### R2 API tokens — bucket-scoped only

Long-lived tokens created via dashboard or the Cloudflare API have four
permission levels (Admin Read & Write, Admin Read-only, Object Read &
Write, Object Read-only) and can be scoped to a specific set of buckets,
but **not to a prefix within a bucket**. This is stated by omission — the
token-creation docs describe bucket scoping in detail and never mention a
path/prefix restriction anywhere.
[Source](https://developers.cloudflare.com/r2/api/tokens/).

### Temporary Access Credentials — the mechanism that fits

[Docs](https://developers.cloudflare.com/r2/api/s3/temporary-credentials/) /
[worked example](https://developers.cloudflare.com/r2/examples/authenticate-r2-temp-credentials/) /
[API reference](https://developers.cloudflare.com/api/resources/r2/subresources/temporary_credentials/methods/create/).

- Derived from a parent R2 API token; **cannot exceed the parent's
  permissions**, and revoking the parent immediately kills every credential
  minted from it.
- Bound to exactly **one bucket** (no cross-bucket credentials).
- `permission` (a preset: `object-read-only` / `object-read-write` /
  `admin-read-only` / `admin-read-write`) or an explicit `actions` list
  (e.g. `["GetObject","HeadObject"]` to allow read but deny listing).
  **`actions` is currently local-signing only — the REST API doesn't accept
  it yet** ("Support in the Temporary Credentials API is coming soon").
- **`prefixes` and `objects`** (top-level request fields) restrict the
  credential to keys under given prefixes or to exact keys. This is the
  PVC-isolation primitive: mint a credential scoped to `prefixes:
  ["pvc-<uid>/"]` and R2 itself returns 403 for anything outside it — the
  docs' own worked example demonstrates exactly this (200 for
  `data/file.bin`, 403 for `other/file.bin` under a `data/`-scoped
  credential).
- Two ways to mint:
  1. **Call the Temporary Credentials API** (`POST
     /accounts/{account_id}/r2/temp-access-credentials`, a Cloudflare
     control-plane REST call). Cloudflare signs the session token for you.
  2. **Sign locally**: build a JWT (`bucket`, `scope`/`actions`, `paths`),
     HS256-sign it with the parent's secret access key, derive
     `secretAccessKey = SHA256(jwt)` and `sessionToken =
     base64("jwt/"+jwt)`. Docs give a runnable TypeScript helper using
     `jose`. Cloudflare's own guidance for *why* to do this: "issuing many
     short-lived credentials and want to avoid per-mint API latency,"
     "need to mint credentials in an environment that cannot reach the
     Cloudflare API," or need `actions`-level scoping.
- **TTL**: `ttlSeconds` is caller-supplied. Docs say to "set `ttlSeconds`
  to the shortest value that fits your use case" and show examples of 900s
  and 3600s (the local-signing helper defaults to 3600). **No documented
  minimum or maximum** — checked the concept page and both API reference
  variants; none states a bound. Flag as unverified; a real account is
  needed to find the actual ceiling (or confirm there isn't one).
- Resulting 3-tuple (`accessKeyId`, `secretAccessKey`, `sessionToken`) is
  standard AWS STS-shaped and works with any S3 client that supports
  session tokens (`AWS_SESSION_TOKEN` / `X-Amz-Security-Token`) — boto3,
  aws-sdk-*, aws4fetch, aws-cli all shown as examples in the docs.
- Security guidance from Cloudflare directly: never ship the parent secret
  to a client; local signing must happen in a trusted environment ("such
  as your backend or a Worker"). This maps cleanly onto k8flare's existing
  model — `workers/gateway` or `workers/storage` already centralizes trust
  and already holds secrets, so minting fits the existing trust boundary
  without adding one.

### Presigned URLs — wrong shape for a PV

Single S3 operation on a single object, generated by local SigV4 signing
(no Cloudflare API call), max expiry 7 days. No prefix or multi-object
scope, and not usable as a general "session" the way a mounted volume
needs. [Source](https://developers.cloudflare.com/r2/api/s3/presigned-urls/).
Good fit for one-off features (e.g. "download this PVC's contents as a
link") — not for the PV backend itself.

### Bucket-per-PVC vs. prefix-in-shared-bucket

The task framing asked me to weigh these. Findings that bear on it, all
from [R2 limits](https://developers.cloudflare.com/r2/platform/limits/)
unless noted:

- **Bucket cap**: 1,000,000 buckets/account. Generous, but bucket-per-PVC
  spends down a finite, account-wide resource on something (PVC count)
  that has no natural ceiling for a multi-tenant platform; prefix-based
  isolation spends nothing from it beyond the handful of buckets the
  platform itself owns (e.g. one per cluster or per tenant).
- **Bucket management op rate limit**: 50/sec, and explicitly **does not
  apply to object read/write** — only to bucket create/delete/list/config.
  A bucket-per-PVC provisioner is the only one of the two designs that
  touches this limit at all (every PVC create/delete is a bucket
  create/delete); at plausible PVC churn rates this individual limit is
  unlikely to bind, but it's a limit that scales with tenant count for one
  design and not the other.
- **Cloudflare REST API budget**: 1,200 requests/5 min, **shared across all
  R2 REST operations on the account** (bucket management, and
  Temporary-Credentials-API minting if using that path). Object GET/PUT
  through the S3-compatible API is explicitly carved out of this and
  recommended instead "for high-throughput object operations." Bucket-
  per-PVC's create/delete calls compete for this shared budget across
  every tenant on the account; prefix-based PVC bind/unbind can avoid this
  budget entirely by minting locally (no Cloudflare API call at all).
- **Secret sprawl**: bucket-per-PVC needs a durable, bucket-scoped R2 API
  token stored *somewhere* per PVC (or the provisioner must mint one via
  the R2 API-token-creation endpoint per PVC, itself another R2 REST call
  against the same shared budget). Prefix-based isolation needs exactly
  one durable parent secret (per cluster, say) and mints disposable,
  self-expiring credentials on demand — smaller durable-secret surface,
  and it's the pattern Cloudflare's own docs are visibly steering toward.
- **Bucket naming**: confirmed via the primary
  [bucket creation docs](https://developers.cloudflare.com/r2/buckets/create-buckets/):
  lowercase letters/digits/hyphens, 3–63 chars, can't start/end with a
  hyphen. **Whether names must be globally unique (across all Cloudflare
  customers) or merely unique within the account is not stated on that
  page**, and I could not find a primary-source page that says either way
  — secondary/blog sources claim "globally unique" but that's unconfirmed
  against `developers.cloudflare.com`. If bucket-per-PVC is ever
  reconsidered, this needs a real test (attempt to create a common bucket
  name and see whether the error is name-taken vs. account-scoped
  success) before relying on any naming scheme.
- **Cleanup cost**: `DeleteObject`, batch `DeleteObjects`, and
  `DeleteBucket` are all in R2's documented free-operations set — neither
  design pays per-delete, only for storage already accrued.
  [Pricing source](https://developers.cloudflare.com/r2/pricing/).

## 2. Containers → S3 API access

### Egress: works by default — but I initially got conflicting answers

Two sources disagreed on the default: an LLM-summarized fetch of
`cloudflare/containers`' `docs/egress.md` claimed internet access is
**blocked** by default; the summarized fetch of the official docs page
claimed the opposite. Per this repo's own "verify, don't trust a read"
rule, I went to primary sources instead of trusting either summary:

- `curl`'d the **raw markdown** of the official docs page directly:
  "By default, a Container will allow internet access, and you can set
  `deniedHosts` to disallow specific hosts or IPs."
  ([source](https://developers.cloudflare.com/containers/platform-details/outbound-traffic/))
- `curl`'d the **actual SDK source** on GitHub and grepped it:
  `enableInternet: ContainerStartOptions['enableInternet'] = true;`
  ([source](https://github.com/cloudflare/containers/blob/main/src/lib/container.ts),
  line ~499).

Both primary sources agree: **internet access is on by default**. The
"blocked by default" claim was wrong — I'm noting the discrepancy rather
than silently dropping it, per this repo's correction-tracking rule.

Practical upshot: a container process making plain HTTPS calls to
`https://<account_id>.r2.cloudflarestorage.com` needs **no special
config** — `interceptHttps=true` and `outbound`/`outboundByHost` handlers
are only needed if the *Worker* wants to inspect or rewrite that traffic,
not for the request to succeed. If k8flare later locks Pod egress down by
default for other (tenant-isolation) reasons, `allowedHosts = ["*.r2.cloudflarestorage.com", ...]`
is the documented way to keep R2 reachable while denying everything else.

### FUSE mounting: real, documented, no privileged-mode config exposed

Contrary to my working assumption going in, Cloudflare Containers **do**
support FUSE-mounting an R2 bucket — this isn't a workaround, it's an
official example.
[Source](https://developers.cloudflare.com/containers/examples/r2-fuse-mount/),
`curl`'d raw. Full worked Dockerfile installs `fuse` via `apk`, downloads
`tigrisfs` (an S3-compatible FUSE adapter,
[github.com/tigrisdata/tigrisfs](https://github.com/tigrisdata/tigrisfs)),
and mounts at container startup:

```
tigrisfs --endpoint "https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com" -f "${R2_BUCKET_NAME}" /mnt/r2
```

Notably, **nothing in the Container class config or wrangler.jsonc exposes
a `privileged`/capabilities/`--cap-add SYS_ADMIN`/`--device /dev/fuse`
knob at all** — the example just installs the `fuse` package and runs the
binary, implying Cloudflare's container runtime grants whatever FUSE needs
transparently in production. This is inference from absence of a config
knob, not a documented guarantee — worth a real deployed-Container test to
confirm rather than assume.

Three caveats worth flagging explicitly:

1. **No native prefix-scoped mount.** Every FUSE adapter mentioned mounts
   the *whole bucket name* at a mount point; accessing only a prefix means
   mounting the bucket and then reading `/mnt/r2/<prefix>/...` — the
   adapter has no "mount just this prefix" flag. Combined with a
   prefix-scoped temporary credential, **it's unverified whether the
   mount itself succeeds** — `tigrisfs -f bucket /mnt/r2` may attempt a
   root-level `ListObjectsV2` at mount time, which a prefix-scoped
   credential may reject or return empty for. This is a concrete,
   real-account test item, not just a documentation gap.
2. **Session-token support in the FUSE adapter is unconfirmed.** tigrisfs's
   own README documents `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` env
   vars and standard `~/.aws/credentials`/`~/.aws/config` files, but
   doesn't explicitly mention `AWS_SESSION_TOKEN` anywhere I could find.
   It's Go-based and very likely rides the standard AWS SDK credential
   chain (which supports session tokens in the credentials file format
   without an integrator needing to special-case it), but that's an
   inference, not a confirmed fact — needed before combining FUSE with
   temporary (as opposed to long-lived) credentials.
3. **Can't be tested in local `wrangler dev`.** Per a Cloudflare community
   thread, local Docker needs `--cap-add SYS_ADMIN --device /dev/fuse` to
   even attempt FUSE, and even with those flags the mount reportedly still
   fails inside `wrangler dev` — FUSE "works great when deployed to
   Cloudflare" but not locally. This is exactly the kind of local-dev trap
   this repo's CLAUDE.md already tracks a section for — worth adding there
   once Phase 8 actually starts touching Containers, since it means FUSE
   verification can only happen against a real deployment.

Object-storage-is-not-POSIX performance caveats are called out directly in
the docs too (no SSD-like performance, fine for "reading shared assets,
bootstrapping agents... persisting user state," explicitly not a
high-performance I/O story) — consistent with using this for general PV
content, not as a database's primary storage.

### A third option I hadn't been asked to check, but should flag: `outboundByHost` binding proxy

Containers can define `outboundByHost` handlers that intercept a **plain
HTTP request to a virtual hostname** (e.g. `http://my.r2/path`) and
resolve it inside the Worker using a real Workers **R2 binding** —
no AWS-style credentials involved at all. The docs' own example scopes
the key by `ctx.containerId`, which is almost exactly the per-tenant
isolation model this task is looking for:

```js
"my.r2": async (request, env, ctx) => {
  const url = new URL(request.url);
  const path = `${ctx.containerId}${url.pathname}`;
  const object = await env.R2.get(path);
  return new Response(object?.body ?? null, { status: object ? 200 : 404 });
},
```

[Source](https://developers.cloudflare.com/containers/platform-details/workers-connections/),
`curl`'d raw.

This is architecturally interesting — zero credentials ever leave the
Worker/DO trust boundary, and it reuses the R2 binding k8flare's Workers
already have — but I'm **not recommending it for general-purpose PV/PVC**:
it's a bespoke HTTP scheme, not the real S3 protocol (no SigV4, no XML
`ListObjectsV2` responses, no multipart, not FUSE-mountable). Any tenant
Pod that expects a genuine S3-compatible endpoint or a FUSE filesystem
would need k8flare to hand-roll enough of the S3 REST surface to satisfy
it, which runs against this repo's "reuse upstream real binaries/protocols,
don't reimplement" rule for a lot less benefit than the temp-credentials
path (which needs zero protocol reimplementation and works with any
existing S3 tool immediately). Worth keeping in mind for k8flare's *own*
internal component storage where a bespoke protocol is fine — not for the
tenant-facing PV backend.

## 3. Cost model inputs (for `docs/cost-model.md` — not written there, reported here only)

All figures from the primary
[pricing](https://developers.cloudflare.com/r2/pricing/) and
[limits](https://developers.cloudflare.com/r2/platform/limits/) pages,
`curl`'d raw (page `dateModified`: pricing 2026-05-28, limits 2026-06-08).

| Item | Standard | Infrequent Access |
| --- | --- | --- |
| Storage | $0.015 / GB-month | $0.01 / GB-month (30-day minimum duration) |
| Class A ops (PutObject, PutBucket, ListObjects, multipart, etc.) | $4.50 / M requests | $9.00 / M requests |
| Class B ops (GetObject, HeadObject, HeadBucket, etc.) | $0.36 / M requests | $0.90 / M requests |
| Data retrieval | none | $0.01 / GB |
| Egress | **free**, always | **free**, always |
| Free tier (Standard only) | 10 GB-month, 1M Class A, 10M Class B /month | not eligible |

Other inputs relevant to a PV/PVC cost model:

- `DeleteObject`, batch `DeleteObjects`, `DeleteBucket`,
  `AbortMultipartUpload` are **free**, uncapped — PVC teardown costs
  nothing beyond storage already accrued.
- Unauthorized (401) requests are never billed — a rejected out-of-prefix
  access attempt from a scoped credential costs nothing.
- No documented minimum billable object size for Standard storage (unlike
  classic S3 IA's small-object floor) — many-small-files PV workloads
  (config dirs, etc.) aren't penalized beyond normal per-GB storage
  billing.
- Minting a temporary credential is **not** one of the listed Class A/B
  operations (it's a Cloudflare control-plane call, not an S3 data-plane
  op) — no direct R2 charge either way. If minted via the REST API it
  consumes the shared 1,200 req/5 min account-wide budget; if minted via
  local JWT signing it's a pure local HMAC computation with zero network
  calls to Cloudflare and zero effect on that budget.
- R2 Data Catalog pricing (separate line item in the pricing page) is
  irrelevant here — that's for Iceberg tables, not an object PV backend.

## 4. Account state (read-only check)

- `npx wrangler r2 bucket list` **succeeded** against the currently
  authenticated account (`k2wanko`, account ID
  `8bc057591d6f5d562d24025c02c721a4`) and returned 17 existing buckets
  (all pre-existing, unrelated personal/other-project buckets — e.g.
  `chibaer`, `flaremdm`, `k3s-data`; nothing k8flare-specific). This
  confirms **R2 read access works today** with the current OAuth session.
- `npx wrangler whoami`'s printed scope list does **not** show an explicit
  `r2` grant (it lists `account (read)`, `workers*`, `d1`, `containers
  (write)`, `cloudchamber (write)`, etc., but no `r2` line) — yet the
  bucket list call above worked anyway. Read this as: the OAuth
  login flow's fixed scope bundle covers R2 listing implicitly (likely
  under the account-read grant), which is a different mechanism from a
  purpose-built API token, where R2 access requires explicitly selecting
  one of the four R2 permission groups at creation time (see §1). Don't
  assume this OAuth session can create buckets or mint R2 API
  tokens/temporary credentials just because listing worked — that's
  untested, and testing it would require a write operation this spike
  was told not to perform.
- `containers (write)` and `cloudchamber (write)` **are** present on this
  token, so Containers deploys are plausible from this session if ever
  needed — not exercised here.
- No bucket, R2 API token, or temporary credential was created. No
  `wrangler secret put` or `wrangler deploy` was run.
- The repo has **zero existing R2 references** anywhere outside this
  spike directory (`grep` across `*.jsonc`/`*.toml`/`*.ts`/`*.go` for
  `r2_buckets`/`R2Bucket`/`R2_BUCKET` came back empty) — Phase 8 is
  greenfield, nothing to reconcile with a partial prior implementation.

## Recommendation for Phase 8

**v1: prefix-scoped Temporary Access Credentials, minted via local JWT
signing, on a shared bucket (one per cluster, or one per tenant — not one
per PVC).** Deliver the resulting `accessKeyId`/`secretAccessKey`/
`sessionToken` triple to the Pod's Container as env vars for direct
S3-SDK use; offer FUSE mounting as an opt-in pattern for apps that want
POSIX-style file access, once the two FUSE caveats above are verified
against a real deployment.

Why this over bucket-per-PVC: it doesn't spend down the account-wide
bucket cap or the shared 1,200 req/5 min control-plane budget as PVC count
grows, it avoids durably storing one long-lived secret per PVC (mint
ephemeral creds from a single parent secret instead), and it's the
pattern Cloudflare's own docs visibly steer integrators toward for
delegated multi-tenant access. Why local signing over the REST API for
minting: zero added latency on the PVC attach path (no extra Cloudflare
API round-trip), it doesn't compete with other control-plane traffic for
the shared rate limit, and it's the only way to get `actions`-level
scoping today.

**The one real gap to solve before this is production-ready**: temporary
credentials are deliberately short-lived by design, but a Pod can mount a
PVC for the life of a long-running workload (days/weeks). Nothing in
Cloudflare's docs or the FUSE example refreshes an in-place credential —
the FUSE example bakes `AWS_ACCESS_KEY_ID`/`SECRET`/(implicitly) a token
into env vars once at container start. Phase 8 needs an explicit answer
to this before it can rely on short TTLs: either a refresh sidecar that
re-mints and hot-swaps credentials (contingent on the FUSE adapter
re-reading `/etc/default/tigrisfs-<bucket>`-style credential files, which
is plausible but unconfirmed), or a deliberately longer TTL as a
pragmatic v1 trade-off against the "scope as narrowly as possible, use
short TTLs" guidance Cloudflare itself gives.

## Residual items (only answerable against a real account)

Everything below needs either an R2 API token (a write/provisioning
operation this spike was told not to perform) or a deployed Container —
neither was in scope here:

1. Actual `ttlSeconds` min/max bound (undocumented).
2. Whether a prefix-scoped credential can successfully be used to FUSE-
   mount a *whole bucket name* (root-level list behavior under a
   prefix-restricted credential is the open question).
3. Whether `tigrisfs` (or another adapter) actually honors
   `AWS_SESSION_TOKEN` end-to-end.
4. Whether R2 bucket names are globally unique or account-scoped (couldn't
   confirm from primary docs either way — only matters if bucket-per-PVC
   is reconsidered later).
5. Whether this OAuth session (or a purpose-built API token) can actually
   create a bucket / R2 API token / temporary credential — only listing
   was verified.
6. Whether Cloudflare's FUSE support genuinely needs no privileged-mode
   config in practice, or whether the docs are just silent about a knob
   that's required and currently missing from the example.
