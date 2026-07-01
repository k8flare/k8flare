/**
 * Validate Bearer token or Basic auth against env.K3S_TOKEN.
 *
 * Returns true if the request carries a valid Authorization header.
 */
export function dwAuth(req: Request, env: { K3S_TOKEN?: string }): boolean {
  const token = env.K3S_TOKEN || "k8flare-dev-token";
  const auth = req.headers.get("Authorization") || "";
  if (auth.startsWith("Bearer ") && auth.slice(7) === token) return true;
  if (auth.startsWith("Basic ")) {
    try {
      const pw = atob(auth.slice(6)).split(":").slice(1).join(":");
      if (pw === token) return true;
    } catch {
      // ignore decode errors
    }
  }
  return false;
}
