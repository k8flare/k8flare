// A Go WASM binary as a Worker Loader dynamic worker: one resident Go
// instance per isolate, instantiated from the parts a Static Assets binding
// serves under /wasm/<name>.*, dispatched per request through
// context.binding.handleRequest (see packages/worker-bridge).
const BOOTSTRAP = `
import "./wasm_exec.js";
import wasmModule from "./app.wasm";
let bindingPromise = null;
function instantiate(env, ctx) {
  return new Promise((resolve, reject) => {
    const go = new Go();
    const binding = {};
    const context = { env, ctx, binding, ready: () => resolve(binding) };
    go.run(new WebAssembly.Instance(wasmModule, go.importObject), context).then(
      () => { bindingPromise = null; reject(new Error("go program exited")); console.error("go program exited"); },
      (err) => { bindingPromise = null; reject(err); console.error("go program failed:", err); },
    );
  });
}
export default {
  async fetch(request, env, ctx) {
    if (!bindingPromise) bindingPromise = instantiate(env, ctx);
    const binding = await bindingPromise;
    const raw = await request.arrayBuffer();
    const path = new URL(request.url).pathname;
    const drive = (path.endsWith("/poke") || path.endsWith("/hold")) && typeof binding.tick === "function";
    const keepalive = drive ? setInterval(() => binding.tick(), 1000) : setInterval(() => {}, 5000);
    let out;
    try {
      out = await binding.handleRequest(
        { method: request.method, url: request.url, headers: [...request.headers], body: raw.byteLength === 0 ? null : new Uint8Array(raw), signal: request.signal },
        env,
      );
    } catch (err) {
      clearInterval(keepalive);
      throw err;
    }
    if (!out.body || typeof out.body.getReader !== "function") {
      clearInterval(keepalive);
      return new Response(out.body, { status: out.status, headers: out.headers });
    }
    const reader = out.body.getReader();
    const body = new ReadableStream({
      async pull(controller) {
        try {
          const { done, value } = await reader.read();
          if (done) {
            clearInterval(keepalive);
            controller.close();
          } else {
            controller.enqueue(value);
          }
        } catch (err) {
          clearInterval(keepalive);
          controller.error(err);
        }
      },
      cancel(reason) {
        clearInterval(keepalive);
        return reader.cancel(reason);
      },
    });
    return new Response(body, { status: out.status, headers: out.headers });
  },
};
`;

interface Manifest {
  size: number;
  sha256: string;
  parts: string[];
}

const manifests = new Map<string, Manifest>();
let isolate = "";
let bornAt = 0;
export function isolateId(): string {
  if (!isolate) {
    isolate = crypto.randomUUID().slice(0, 8);
    bornAt = Date.now();
  }
  return isolate;
}
const loadedWorkers = new Set<string>();

const assetAttempts = 4;

async function asset(assets: Fetcher, path: string): Promise<Response> {
  for (let attempt = 1; ; attempt++) {
    const resp = await assets.fetch(`https://assets.internal/wasm/${path}`);
    if (resp.ok) return resp;
    if (resp.status < 500 || attempt >= assetAttempts) throw new Error(`asset wasm/${path}: HTTP ${resp.status} (run make wasm)`);
    console.log(`loader iso=${isolateId()} asset wasm/${path} HTTP ${resp.status}, retry ${attempt}`);
    await new Promise((r) => setTimeout(r, 300 * attempt));
  }
}

async function assemble(assets: Fetcher, m: Manifest): Promise<Uint8Array> {
  const parts = await Promise.all(m.parts.map(async (part) => new Uint8Array(await (await asset(assets, part)).arrayBuffer())));
  const wasm = new Uint8Array(m.size);
  let off = 0;
  for (const part of parts) {
    wasm.set(part, off);
    off += part.byteLength;
  }
  if (off !== m.size) throw new Error(`wasm reassembly: got ${off} bytes, manifest says ${m.size}`);
  return wasm;
}

/** Returns the entrypoint of the dynamic worker hosting `name`, loading it on first use. */
export async function loadWasmWorker(
  loader: WorkerLoader,
  assets: Fetcher,
  name: string,
  env: Record<string, unknown>,
  tail?: Fetcher,
): Promise<Fetcher> {
  try {
    return await loadOnce(loader, assets, name, env, tail);
  } catch (err) {
    if (!String(err).includes("asset wasm/")) throw err;
    manifests.delete(name);
    console.log(`loader iso=${isolateId()} stale manifest for ${name}, refetching`);
    return loadOnce(loader, assets, name, env, tail);
  }
}

async function loadOnce(
  loader: WorkerLoader,
  assets: Fetcher,
  name: string,
  env: Record<string, unknown>,
  tail?: Fetcher,
): Promise<Fetcher> {
  let m = manifests.get(name);
  if (!m) {
    m = (await (await asset(assets, `${name}.manifest.json`)).json()) as Manifest;
    manifests.set(name, m);
  }
  const manifest = m;
  let loadedNow = false;
  const worker = loader.get(`${name}@${manifest.sha256}@${tail ? 1 : 0}`, async () => {
      loadedNow = true;
      const started = Date.now();
      const code = {
        compatibilityDate: "2026-09-01",
        mainModule: "index.js",
        modules: {
          "index.js": BOOTSTRAP,
          "wasm_exec.js": await (await asset(assets, "wasm_exec.js")).text(),
          "app.wasm": { wasm: (await assemble(assets, manifest)).buffer as ArrayBuffer },
        },
        env,
        ...(tail ? { tails: [tail] } : {}),
      };
      loadedWorkers.add(name);
      console.log(`loader iso=${isolateId()} age=${Math.round((Date.now() - bornAt) / 1000)}s load=${name} loaded=${loadedWorkers.size} ms=${Date.now() - started}`);
      return code;
  });
  const entrypoint = worker.getEntrypoint();
  return entrypoint;
}
