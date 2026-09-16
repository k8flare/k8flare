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
    const keepalive = setInterval(() => {}, 5000);
    try {
      const out = await binding.handleRequest(
        { method: request.method, url: request.url, headers: [...request.headers], body: raw.byteLength === 0 ? null : new Uint8Array(raw), signal: request.signal },
        env,
      );
      return new Response(out.body, { status: out.status, headers: out.headers });
    } finally {
      clearInterval(keepalive);
    }
  },
};
`;

interface Manifest {
  size: number;
  sha256: string;
  parts: string[];
}

const manifests = new Map<string, Manifest>();
let loading: Promise<unknown> = Promise.resolve();

function serialized<T>(fn: () => Promise<T>): Promise<T> {
  const next = loading.then(fn, fn);
  loading = next.catch(() => {});
  return next;
}

async function asset(assets: Fetcher, path: string): Promise<Response> {
  const resp = await assets.fetch(`https://assets.internal/wasm/${path}`);
  if (!resp.ok) throw new Error(`asset wasm/${path}: HTTP ${resp.status} (run make wasm)`);
  return resp;
}

async function assemble(assets: Fetcher, m: Manifest): Promise<Uint8Array> {
  const wasm = new Uint8Array(m.size);
  let off = 0;
  for (const part of m.parts) {
    const reader = (await asset(assets, part)).body!.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      wasm.set(value, off);
      off += value.byteLength;
    }
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
  let m = manifests.get(name);
  if (!m) {
    m = (await (await asset(assets, `${name}.manifest.json`)).json()) as Manifest;
    manifests.set(name, m);
  }
  const manifest = m;
  const worker = loader.get(`${name}@${manifest.sha256}@${tail ? 1 : 0}`, () =>
    serialized(async () => ({
      compatibilityDate: "2026-09-01",
      mainModule: "index.js",
      modules: {
        "index.js": BOOTSTRAP,
        "wasm_exec.js": await (await asset(assets, "wasm_exec.js")).text(),
        "app.wasm": { wasm: (await assemble(assets, manifest)).buffer as ArrayBuffer },
      },
      env,
      ...(tail ? { tails: [tail] } : {}),
    })),
  );
  return worker.getEntrypoint();
}
