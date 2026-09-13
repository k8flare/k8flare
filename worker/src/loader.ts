
// The Go apiserver runs as a Loader dynamic worker: one resident Go
// instance per isolate, instantiated from WASM parts served by ASSETS.
// The bootstrap below is the dynamic worker's own module source.
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
      () => reject(new Error("go program exited before ready")),
      (err) => reject(err),
    );
  });
}
export default {
  async fetch(request, env, ctx) {
    if (!bindingPromise) bindingPromise = instantiate(env, ctx);
    const binding = await bindingPromise;
    const raw = await request.arrayBuffer();
    const out = await binding.handleRequest(
      { method: request.method, url: request.url, headers: [...request.headers], body: raw.byteLength === 0 ? null : new Uint8Array(raw) },
      env,
    );
    return new Response(out.body, { status: out.status, headers: out.headers });
  },
};
`;

interface Manifest {
  size: number;
  sha256: string;
  parts: string[];
}

let manifest: Manifest | null = null;

async function asset(env: Env, path: string): Promise<Response> {
  const resp = await env.ASSETS.fetch(`https://assets.internal/wasm/${path}`);
  if (!resp.ok) throw new Error(`asset wasm/${path}: HTTP ${resp.status} (run make wasm)`);
  return resp;
}

async function assemble(env: Env, m: Manifest): Promise<Uint8Array> {
  const wasm = new Uint8Array(m.size);
  let off = 0;
  for (const part of m.parts) {
    const reader = (await asset(env, part)).body!.getReader();
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

export async function apiserverFetch(env: Env, request: Request): Promise<Response> {
  manifest ??= (await (await asset(env, "apiserver.manifest.json")).json()) as Manifest;
  const m = manifest;
  const worker = env.LOADER.get(`apiserver@${m.sha256}`, async () => ({
    compatibilityDate: "2026-09-01",
    mainModule: "index.js",
    modules: {
      "index.js": BOOTSTRAP,
      "wasm_exec.js": await (await asset(env, "wasm_exec.js")).text(),
      "app.wasm": { wasm: (await assemble(env, m)).buffer as ArrayBuffer },
    },
    env: {
      STORAGE: env.STORAGE,
      ADMIN_TOKEN: env.ADMIN_TOKEN,
      JOIN_TOKEN: env.JOIN_TOKEN,
      KUBELET_SCHEME: env.KUBELET_SCHEME,
      KUBELET_PORT: env.KUBELET_PORT,
    },
  }));
  return worker.getEntrypoint().fetch(request);
}
