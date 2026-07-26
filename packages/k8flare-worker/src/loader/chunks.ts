// Chunked-WASM supply channel shared by the apiserver loader
// (./apiserver.ts) and the Controllers DO (../controllers/index.ts):
// ≤24MiB Static Assets parts + sha256 manifest, assembled by streaming
// into ONE preallocated buffer (never all-parts-plus-copy at once --
// production's 128MiB isolate limit, S14).

export interface WasmManifest {
  sha256: string;
  size: number;
  parts: string[];
}

// Manifests live under assets/wasm/ (the run_worker_first-protected
// subtree; see wrangler.jsonc).
export async function fetchWasmManifest(
  assets: Fetcher,
  name: string,
): Promise<WasmManifest | null> {
  const resp = await assets.fetch(`https://assets.internal/wasm/${name}.manifest.json`);
  if (resp.status === 404) return null; // component not shipped
  if (!resp.ok) {
    throw new Error(
      `wasm/${name}.manifest.json: HTTP ${resp.status} -- packages/k8flare-worker/assets/wasm/ is missing; run npm run build:wasm first`,
    );
  }
  return resp.json();
}

export async function fetchWasmAsset(assets: Fetcher, path: string): Promise<Response> {
  const resp = await assets.fetch(`https://assets.internal/wasm/${path}`);
  if (!resp.ok) throw new Error(`asset wasm/${path}: HTTP ${resp.status}`);
  return resp;
}

export async function assembleWasm(assets: Fetcher, manifest: WasmManifest): Promise<Uint8Array> {
  const wasm = new Uint8Array(manifest.size);
  let off = 0;
  for (const part of manifest.parts) {
    const resp = await fetchWasmAsset(assets, part);
    if (!resp.body) throw new Error(`asset wasm/${part}: empty body`);
    const reader = resp.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      wasm.set(value, off);
      off += value.byteLength;
    }
  }
  if (off !== manifest.size) {
    throw new Error(`wasm reassembly: got ${off} bytes, manifest says ${manifest.size}`);
  }
  return wasm;
}
