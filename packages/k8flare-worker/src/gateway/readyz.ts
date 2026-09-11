import type { Env } from "../env.ts";
import { apiserverFetch } from "../loader/apiserver.ts";
import { fetchWasmManifest } from "../loader/chunks.ts";

const RESIDENT_COMPONENTS = ["kcm", "gc", "sched"] as const;

// This endpoint is anonymous (an uptime monitor has no cluster token),
// and the checks below cost a Cluster DO read plus a Loader dispatch, so
// request volume must not reach them: one caller per TTL runs the real
// checks and everyone else is served that verdict.
//
// 30s for a ready verdict is the fast end of what an uptime checker
// polls at, so a monitor still pays for roughly every poll it makes
// while a flood pays for none of its extra volume. A not-ready verdict
// expires in 5s instead: that is the state where someone is watching for
// the recovery edge, and even then a flood is bounded at 12 real check
// runs a minute per isolate.
//
// The bound is per isolate, because that is the only state a Worker can
// hold for free. Global volume therefore scales with the number of live
// isolates, not with the number of requests.
const READY_TTL_MS = 30_000;
const NOT_READY_TTL_MS = 5_000;

interface ReadyCheck {
  name: string;
  ok: boolean;
  detail: string;
}

interface Verdict {
  checks: ReadyCheck[];
  at: number;
  ok: boolean;
}

interface Entry {
  verdict: Verdict | null;
  inFlight: Promise<Verdict> | null;
  runs: number;
}

const entries = new Map<string, Entry>();

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

// The apiserver's own /readyz (pkg/apiserver/discovery.go), one of the
// few routes registered outside AuthMiddleware. Nothing here may depend
// on the caller's identity: an anonymous caller's derived env carries a
// placeholder token, so the list this check used to issue answered 401
// and every anonymous verdict was a false 503 (measured 2026-09-11).
// What it asserts is that the Loader can assemble and boot the module
// and that Go is serving; the Cluster DO is the check above. Not
// covered: the apiserver's own STORAGE binding, which no endpoint an
// anonymous caller may reach exercises.
async function checkAPIServer(env: Env): Promise<string> {
  const resp = await apiserverFetch(env, new Request("http://internal/readyz"));
  if (!resp.ok) throw new Error(`GET /readyz: HTTP ${resp.status}`);
  const body = (await resp.text()).trim();
  if (body !== "ok") throw new Error(`GET /readyz: body ${JSON.stringify(body.slice(0, 120))}`);
  return "serving";
}

async function checkComponent(env: Env, name: string): Promise<string> {
  const manifest = await fetchWasmManifest(env.ASSETS, name);
  if (!manifest) {
    throw new Error(`wasm/${name}.manifest.json absent -- this deployment cannot load ${name}`);
  }
  return `${manifest.size} bytes, sha256 ${manifest.sha256.slice(0, 12)}`;
}

async function readyChecks(env: Env): Promise<ReadyCheck[]> {
  const components: string[] = [...RESIDENT_COMPONENTS];
  if ((env.CLUSTER_DO_NAME ?? "default") === "default") components.push("clusterop");
  return Promise.all([
    run("storage", () => checkStorage(env)),
    run("apiserver", () => checkAPIServer(env)),
    ...components.map((c) => run(`component/${c}`, () => checkComponent(env, c))),
  ]);
}

// Keyed per cluster: one isolate serves every tenant, and one tenant's
// verdict says nothing about another's storage.
function entryFor(env: Env): Entry {
  const key = env.CLUSTER_DO_NAME ?? "default";
  let e = entries.get(key);
  if (!e) {
    e = { verdict: null, inFlight: null, runs: 0 };
    entries.set(key, e);
  }
  return e;
}

function cachedVerdict(env: Env): Promise<Verdict> {
  const e = entryFor(env);
  const ttl = e.verdict?.ok ? READY_TTL_MS : NOT_READY_TTL_MS;
  if (e.verdict && Date.now() - e.verdict.at < ttl) return Promise.resolve(e.verdict);
  // Callers arriving while a run is in flight await that run rather than
  // starting one of their own, so a burst on a cold cache costs one run.
  if (!e.inFlight) {
    e.runs++;
    e.inFlight = readyChecks(env)
      .then((checks) => {
        e.verdict = { checks, at: Date.now(), ok: checks.every((c) => c.ok) };
        return e.verdict;
      })
      .finally(() => {
        e.inFlight = null;
      });
  }
  return e.inFlight;
}

// Upstream's shape. Without ?verbose a passing kube-apiserver answers a
// bare "ok" and a failing one names the checks with their reasons
// withheld, which is all an anonymous caller gets here too.
function terseBody(v: Verdict): string {
  if (v.ok) return "ok";
  const lines = v.checks.map((c) =>
    c.ok ? `[+]${c.name} ok` : `[-]${c.name} failed: reason withheld`,
  );
  lines.push("readyz check failed");
  return lines.join("\n") + "\n";
}

function verboseBody(v: Verdict, e: Entry, ageMs: number): string {
  const lines = v.checks.map(
    (c) => `[${c.ok ? "+" : "-"}]${c.name} ${c.ok ? "ok" : "failed"}: ${c.detail}`,
  );
  lines.push(v.ok ? "readyz check passed" : "readyz check failed");
  lines.push(
    `verdict ${Math.round(ageMs / 1000)}s old, ${e.runs} uncached run(s) in this isolate` +
      ` (at most one per ${(v.ok ? READY_TTL_MS : NOT_READY_TTL_MS) / 1000}s)`,
  );
  return lines.join("\n") + "\n";
}

export async function handleReadyz(env: Env, verbose: boolean): Promise<Response> {
  const v = await cachedVerdict(env);
  const ageMs = Date.now() - v.at;
  return new Response(verbose ? verboseBody(v, entryFor(env), ageMs) : terseBody(v), {
    status: v.ok ? 200 : 503,
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "no-store",
      Age: String(Math.round(ageMs / 1000)),
    },
  });
}
