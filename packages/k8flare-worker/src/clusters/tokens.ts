import type { Env } from "../env.ts";

// Per-cluster bearer tokens, stored as ONE kine value at
// /ca/cluster-tokens inside the cluster's own Cluster DO -- the /ca/
// prefix routes into the ca-vault facet via the existing keyspace
// classification (storage/keyspace.ts), so no new storage machinery
// exists for this. Plaintext by design: the NodeVM path injects the
// real token into microVMs and the kubeconfig endpoint re-serves it;
// the vault already holds the CA private keys, so this adds no new
// exposure class (multi-cluster plan, decision A).
//
// Multiple concurrently-valid tokens = rotation. Verifiers cache per
// isolate with a short TTL; revocation therefore propagates within
// TOKEN_CACHE_TTL_MS (documented tradeoff -- emergency revocation is
// cluster deletion).

// A token with no role is an administrator (system:masters, via
// pkg/apiserver/auth.go). Leaving it optional is what lets every vault
// written before roles existed keep authenticating unchanged; an "agent"
// token authenticates as system:nodes and nothing more, so a node's copy
// of it is no longer a cluster administrator (TODO.md P0-8).
export type ClusterTokenRole = "admin" | "agent";

export interface ClusterToken {
  tokenId: string;
  secret: string;
  createdAt: string;
  role?: ClusterTokenRole;
}

export const TOKENS_KEY = "/ca/cluster-tokens";
const TOKEN_CACHE_TTL_MS = 60_000;

const b64encode = (s: string) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));
const b64decode = (b: string) =>
  new TextDecoder().decode(Uint8Array.from(atob(b), (c) => c.charCodeAt(0)));

function clusterStub(env: Env, doName: string) {
  const ns = env.CLUSTER;
  return ns.get(ns.idFromName(doName));
}

/** Raw read of the token list (no cache). Returns null when the vault key doesn't exist. */
export async function readClusterTokens(
  env: Env,
  doName: string,
): Promise<{ tokens: ClusterToken[]; revision: number } | null> {
  const resp = await clusterStub(env, doName).fetch(`http://do.internal/key${TOKENS_KEY}`);
  if (!resp.ok) throw new Error(`read cluster tokens: HTTP ${resp.status}`);
  const body = (await resp.json()) as {
    revision: number;
    kv: { value: string; modRevision: number } | null;
  };
  if (!body.kv) return null;
  return {
    tokens: (JSON.parse(b64decode(body.kv.value)) as { tokens: ClusterToken[] }).tokens,
    revision: body.kv.modRevision,
  };
}

/** Write the token list. revision 0 = create (409 on exists); otherwise compare-and-swap. */
export async function writeClusterTokens(
  env: Env,
  doName: string,
  tokens: ClusterToken[],
  revision: number,
): Promise<void> {
  const resp = await clusterStub(env, doName).fetch(`http://do.internal/key${TOKENS_KEY}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value: b64encode(JSON.stringify({ tokens })), revision }),
  });
  if (!resp.ok && resp.status !== 201) {
    throw new Error(`write cluster tokens: HTTP ${resp.status} ${await resp.text()}`);
  }
}

/**
 * Mints a token. tokenId may be supplied by the caller to make a rotation
 * REPLAYABLE: the cluster operator derives it deterministically from the
 * rotate annotation's value, so a retried rotation recognizes the token it
 * already minted instead of minting a second one every attempt.
 *
 * An omitted role stringifies away, so an administrator token is stored in
 * exactly the shape every deployed vault already holds.
 */
export function mintToken(tokenId?: string, role?: ClusterTokenRole): ClusterToken {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return {
    tokenId: tokenId || crypto.randomUUID().slice(0, 8),
    secret: [...bytes].map((b) => b.toString(16).padStart(2, "0")).join(""),
    createdAt: new Date().toISOString(),
    role,
  };
}

// Isolate-level TTL cache of ACCEPTED secrets per cluster doName.
const tokenCache = new Map<string, { secrets: string[]; expires: number }>();

export async function clusterSecrets(env: Env, doName: string): Promise<string[]> {
  const hit = tokenCache.get(doName);
  if (hit && hit.expires > Date.now()) return hit.secrets;
  // Every cluster authenticates against its own vault. The DEFAULT
  // cluster additionally accepts the K3S_TOKEN Worker secret (2026-07-27
  // user decision: one intuitive always-valid root token; ADMIN_TOKENS is
  // gone -- administering clusters is a default-cluster privilege). With
  // neither a vault token nor the secret, default is the dev posture
  // (dev fallback token), same rule the Go side has always had.
  const vault = await readClusterTokens(env, doName);
  // Administrators first. Every caller that wants a CREDENTIAL rather
  // than a verifier takes [0] (controllers/index.ts, nodes/scheduler.ts,
  // clusters/api.ts), so an agent-role token sorting first would hand
  // them a token that cannot administer the cluster.
  const tokens = vault?.tokens ?? [];
  const secrets = [
    ...tokens.filter((t) => (t.role ?? "admin") === "admin"),
    ...tokens.filter((t) => (t.role ?? "admin") !== "admin"),
  ].map((t) => t.secret);
  if (doName === "default") {
    if (env.K3S_TOKEN) secrets.push(env.K3S_TOKEN);
    if (secrets.length === 0) secrets.push("k8flare-dev-token");
  }
  tokenCache.set(doName, { secrets, expires: Date.now() + TOKEN_CACHE_TTL_MS });
  return secrets;
}

function timingSafeEqualStr(a: string, b: string): boolean {
  const enc = new TextEncoder();
  const ab = enc.encode(a);
  const bb = enc.encode(b);
  if (ab.byteLength !== bb.byteLength) return false;
  // @ts-expect-error timingSafeEqual is a Workers runtime API on crypto.subtle
  return crypto.subtle.timingSafeEqual(ab, bb) as boolean;
}

/**
 * Cluster-scoped replacement for k8s/auth.ts's dwAuth: accepts Bearer
 * <token> (kubectl/clients) or Basic <any>:<token> (the k3s agent join
 * path) against ANY currently-valid token of the cluster. Returns the
 * presented secret on success (the caller threads it into the derived
 * env so downstream defense-in-depth dwAuth re-checks still pass), or
 * null on failure.
 */
export async function verifyClusterToken(
  req: Request,
  env: Env,
  doName: string,
): Promise<string | null> {
  const auth = req.headers.get("Authorization") || "";
  let presented: string | null = null;
  if (auth.startsWith("Bearer ")) {
    presented = auth.slice("Bearer ".length);
  } else if (auth.startsWith("Basic ")) {
    const decoded = atob(auth.slice("Basic ".length));
    const colon = decoded.indexOf(":");
    if (colon >= 0) presented = decoded.slice(colon + 1);
  }
  if (!presented) return null;
  const secrets = await clusterSecrets(env, doName);
  for (const s of secrets) {
    if (timingSafeEqualStr(s, presented)) return presented;
  }
  return null;
}

/** Test hook / teardown helper: drop a cluster's cached secrets in this isolate. */
export function invalidateTokenCache(doName: string): void {
  tokenCache.delete(doName);
}
