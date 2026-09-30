export type Component = "scheduler" | "gc" | "hpa" | "attachdetach" | "admission" | "workloads" | "addons" | "hookecho" | "podkubelet";

const encoder = new TextEncoder();

export async function componentToken(env: { ADMIN_TOKEN?: string }, component: Component): Promise<string> {
  if (!env.ADMIN_TOKEN) return "";
  const key = await crypto.subtle.importKey("raw", encoder.encode(env.ADMIN_TOKEN), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", key, encoder.encode(`k8flare-component:${component}`)));
  const hex = Array.from(mac, (b) => b.toString(16).padStart(2, "0")).join("");
  return `component:${component}:${hex}`;
}
