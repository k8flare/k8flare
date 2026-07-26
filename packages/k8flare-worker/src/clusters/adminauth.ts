import type { Env } from "../env.ts";
import { verifyClusterToken } from "./tokens.ts";

// Management-API authentication (POST /clusters etc.), pluggable per the
// user's 2026-07-06 decision: rotatable shared secrets AND room for
// Cloudflare Access.
//
//  1. ADMIN_TOKENS: comma-separated secret; ANY listed value is accepted
//     (constant-time compare), so rotation = add the new token, roll
//     clients, remove the old one.
//  2. ACCESS_TEAM_DOMAIN + ACCESS_AUD: verify the Cf-Access-Jwt-Assertion
//     JWT's signature against the team's JWKS (cached per isolate),
//     issuer, audience, and expiry. Merely trusting the header's
//     presence was rejected -- a workers.dev direct hit would bypass
//     Access entirely.
//  3. Neither configured: fall back to the dev admin token -- the same
//     posture as K3S_TOKEN's k8flare-dev-token fallback (a deployment
//     that sets no secrets is a dev deployment; production MUST set
//     ADMIN_TOKENS or Access vars, documented in README).

const JWKS_TTL_MS = 10 * 60_000;

let jwksCache: { keys: Map<string, CryptoKey>; expires: number } | null = null;

async function accessJWKS(teamDomain: string): Promise<Map<string, CryptoKey>> {
  if (jwksCache && jwksCache.expires > Date.now()) return jwksCache.keys;
  const resp = await fetch(`https://${teamDomain}/cdn-cgi/access/certs`);
  if (!resp.ok) throw new Error(`Access JWKS fetch: HTTP ${resp.status}`);
  const { keys } = (await resp.json()) as { keys: Array<JsonWebKey & { kid: string }> };
  const imported = new Map<string, CryptoKey>();
  for (const jwk of keys) {
    imported.set(
      jwk.kid,
      await crypto.subtle.importKey(
        "jwk",
        jwk,
        { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
        false,
        ["verify"],
      ),
    );
  }
  jwksCache = { keys: imported, expires: Date.now() + JWKS_TTL_MS };
  return imported;
}

const b64urlToBytes = (s: string) =>
  Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0));

async function verifyAccessJWT(env: Env, jwt: string): Promise<boolean> {
  try {
    const [h, p, sig] = jwt.split(".");
    if (!h || !p || !sig) return false;
    const header = JSON.parse(new TextDecoder().decode(b64urlToBytes(h)));
    const payload = JSON.parse(new TextDecoder().decode(b64urlToBytes(p)));
    if (payload.iss !== `https://${env.ACCESS_TEAM_DOMAIN}`) return false;
    const aud = Array.isArray(payload.aud) ? payload.aud : [payload.aud];
    if (!aud.includes(env.ACCESS_AUD)) return false;
    if (typeof payload.exp !== "number" || payload.exp * 1000 < Date.now()) return false;
    const key = (await accessJWKS(env.ACCESS_TEAM_DOMAIN!)).get(header.kid);
    if (!key) return false;
    return await crypto.subtle.verify(
      "RSASSA-PKCS1-v1_5",
      key,
      b64urlToBytes(sig),
      new TextEncoder().encode(`${h}.${p}`),
    );
  } catch {
    return false;
  }
}

export async function authorizeAdmin(req: Request, env: Env): Promise<boolean> {
  // ADMIN_TOKENS is abolished (2026-07-27): administering clusters IS a
  // default-cluster privilege, so the management API accepts any
  // currently-valid DEFAULT-cluster token (the K3S_TOKEN secret or a
  // vault token; the dev fallback token while neither exists -- the
  // same dev posture as the cluster APIs). Cloudflare Access remains an
  // optional additional gate. Finer-grained admin identities are
  // deliberately deferred to the RBAC + Access integration planned in
  // docs/cluster-api-design.md.
  if (env.ACCESS_TEAM_DOMAIN && env.ACCESS_AUD) {
    const jwt = req.headers.get("Cf-Access-Jwt-Assertion");
    if (jwt && (await verifyAccessJWT(env, jwt))) return true;
  }
  return (await verifyClusterToken(req, env, "default")) !== null;
}

