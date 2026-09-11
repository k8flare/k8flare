import type { Env } from "../env.ts";
import { apiserverFetch } from "../loader/apiserver.ts";
import { fetchWasmManifest } from "../loader/chunks.ts";

const RESIDENT_COMPONENTS = ["kcm", "gc", "sched"] as const;

export interface ReadyCheck {
  name: string;
  ok: boolean;
  detail: string;
}

async function run(name: string, fn: () => Promise<string>): Promise<ReadyCheck> {
  try {
    return { name, ok: true, detail: await fn() };
  } catch (err) {
    return { name, ok: false, detail: err instanceof Error ? err.message : String(err) };
  }
}

// GET /revision, not a list or a write: one sqlite read of the kine
// head is the cheapest thing that proves the Cluster DO is reachable,
// and it touches no path that can arm the safety-net alarm.
async function checkStorage(env: Env): Promise<string> {
  const ns = env.CLUSTER;
  const resp = await ns.get(ns.idFromName("default")).fetch("http://cluster.internal/revision");
  if (!resp.ok) throw new Error(`GET /revision: HTTP ${resp.status}`);
  const body = await resp.json<{ revision?: number }>();
  if (typeof body.revision !== "number") {
    throw new Error(`GET /revision: no revision in ${JSON.stringify(body).slice(0, 120)}`);
  }
  return `kine revision ${body.revision}`;
}

async function checkAPIServer(env: Env): Promise<string> {
  const token = env.K3S_TOKEN || "k8flare-dev-token";
  const resp = await apiserverFetch(
    env,
    new Request("http://internal/api/v1/namespaces?limit=1", {
      headers: { Authorization: `Bearer ${token}` },
    }),
  );
  if (!resp.ok) throw new Error(`GET /api/v1/namespaces: HTTP ${resp.status}`);
  const body = await resp.json<{ kind?: string }>();
  if (body.kind !== "NamespaceList") {
    throw new Error(`GET /api/v1/namespaces: kind ${body.kind ?? "(absent)"}`);
  }
  return "serving";
}

async function checkComponent(env: Env, name: string): Promise<string> {
  const manifest = await fetchWasmManifest(env.ASSETS, name);
  if (!manifest) {
    throw new Error(`wasm/${name}.manifest.json absent -- this deployment cannot load ${name}`);
  }
  return `${manifest.size} bytes, sha256 ${manifest.sha256.slice(0, 12)}`;
}

export async function readyChecks(env: Env): Promise<ReadyCheck[]> {
  const components: string[] = [...RESIDENT_COMPONENTS];
  if ((env.CLUSTER_DO_NAME ?? "default") === "default") components.push("clusterop");
  return Promise.all([
    run("storage", () => checkStorage(env)),
    run("apiserver", () => checkAPIServer(env)),
    ...components.map((c) => run(`component/${c}`, () => checkComponent(env, c))),
  ]);
}

export function formatReadyChecks(checks: ReadyCheck[]): string {
  const lines = checks.map(
    (c) => `[${c.ok ? "+" : "-"}]${c.name} ${c.ok ? "ok" : "failed"}: ${c.detail}`,
  );
  lines.push(checks.every((c) => c.ok) ? "readyz check passed" : "readyz check failed");
  return lines.join("\n") + "\n";
}

export async function handleReadyz(env: Env): Promise<Response> {
  const checks = await readyChecks(env);
  return new Response(formatReadyChecks(checks), {
    status: checks.every((c) => c.ok) ? 200 : 503,
    headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" },
  });
}
