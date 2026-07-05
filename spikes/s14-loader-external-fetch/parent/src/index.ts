export interface Env {
  LOADER: any;
  CODE_BUCKET: R2Bucket;
  SCHED_BUCKET: R2Bucket;
  ASSETS: Fetcher;
}

// The 65 goscript-transpiled satellite modules for the real kube-scheduler
// dependency graph (esbuild-bundled per-package, relative-import style,
// scratchpad scc-out46), seeded into local R2 under "sched/<basename>" by
// the s14 spike's shell driver -- NOT checked into this repo.
const SCHEDULER_SATELLITES = [
  "@goscript-go.yaml.in-yaml-v2-decode.gs.js",
  "@goscript-go.yaml.in-yaml-v2-encode.gs.js",
  "@goscript-go.yaml.in-yaml-v2-index.js",
  "@goscript-go.yaml.in-yaml-v2-resolve.gs.js",
  "@goscript-go.yaml.in-yaml-v2-yaml.gs.js",
  "@goscript-go.yaml.in-yaml-v3-decode.gs.js",
  "@goscript-go.yaml.in-yaml-v3-encode.gs.js",
  "@goscript-go.yaml.in-yaml-v3-index.js",
  "@goscript-go.yaml.in-yaml-v3-resolve.gs.js",
  "@goscript-go.yaml.in-yaml-v3-yaml.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-build.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-desc_init.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-desc_lazy.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-desc_list_gen.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-desc.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-editions.gs.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-index.js",
  "@goscript-google.golang.org-protobuf-internal-filedesc-placeholder.gs.js",
  "@goscript-google.golang.org-protobuf-internal-pragma-index.js",
  "@goscript-google.golang.org-protobuf-internal-pragma-pragma.gs.js",
  "@goscript-gopkg.in-yaml.v3-decode.gs.js",
  "@goscript-gopkg.in-yaml.v3-encode.gs.js",
  "@goscript-gopkg.in-yaml.v3-index.js",
  "@goscript-gopkg.in-yaml.v3-resolve.gs.js",
  "@goscript-gopkg.in-yaml.v3-yaml.gs.js",
  "@goscript-k8s.io-apimachinery-pkg-apis-meta-v1-conversion.gs.js",
  "@goscript-k8s.io-apimachinery-pkg-apis-meta-v1-index.js",
  "@goscript-k8s.io-apimachinery-pkg-apis-meta-v1-register.gs.js",
  "@goscript-k8s.io-apimachinery-pkg-apis-meta-v1-zz_generated.conversion.gs.js",
  "@goscript-k8s.io-client-go-tools-clientcmd-api-helpers.gs.js",
  "@goscript-k8s.io-client-go-tools-clientcmd-api-index.js",
  "@goscript-k8s.io-client-go-tools-clientcmd-api-register.gs.js",
  "@goscript-k8s.io-client-go-tools-clientcmd-api-types.gs.js",
  "@goscript-k8s.io-client-go-tools-clientcmd-api-zz_generated.deepcopy.gs.js",
  "@goscript-k8s.io-klog-v2-contextual_slog.gs.js",
  "@goscript-k8s.io-klog-v2-contextual.gs.js",
  "@goscript-k8s.io-klog-v2-exit.gs.js",
  "@goscript-k8s.io-klog-v2-index.js",
  "@goscript-k8s.io-klog-v2-klog_file_others.gs.js",
  "@goscript-k8s.io-klog-v2-klog_file.gs.js",
  "@goscript-k8s.io-klog-v2-klog.gs.js",
  "@goscript-k8s.io-klog-v2-klogr_slog.gs.js",
  "@goscript-k8s.io-klog-v2-klogr.gs.js",
  "@goscript-k8s.io-kube-scheduler-config-v1-index.js",
  "@goscript-k8s.io-kube-scheduler-config-v1-register.gs.js",
  "@goscript-k8s.io-kube-scheduler-config-v1-types_pluginargs.gs.js",
  "@goscript-k8s.io-kube-scheduler-config-v1-types.gs.js",
  "@goscript-k8s.io-kube-scheduler-config-v1-zz_generated.deepcopy.gs.js",
  "@goscript-k8s.io-kube-scheduler-config-v1-zz_generated.model_name.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-core-v1-conversion.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-core-v1-index.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-core-v1-register.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-core-v1-zz_generated.conversion.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-core-v1-zz_generated.validations.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-scheduling-index.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-scheduling-register.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-scheduling-types.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-apis-scheduling-zz_generated.deepcopy.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-scheduler-apis-config-v1-conversion.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-scheduler-apis-config-v1-defaults.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-scheduler-apis-config-v1-index.js",
  "@goscript-k8s.io-kubernetes-pkg-scheduler-apis-config-v1-register.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-scheduler-apis-config-v1-zz_generated.conversion.gs.js",
  "@goscript-k8s.io-kubernetes-pkg-scheduler-apis-config-v1-zz_generated.defaults.gs.js",
];

const ADD_WASM_BASE64 = "AGFzbQEAAAABBwFgAn9/AX8DAgEABwcBA2FkZAAACgkBBwAgACABags=";
function addWasmBytes(): ArrayBuffer {
  const bin = atob(ADD_WASM_BASE64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes.buffer;
}

const WASM_MAIN_SRC = `
import wasmModule from "./add.wasm";
export default {
  async fetch(req) {
    const instance = new WebAssembly.Instance(wasmModule, {});
    return Response.json({ ok: true, result: instance.exports.add(3, 4), source: "r2-fetched" });
  }
};
`;

async function passthrough(resp: Response): Promise<Response> {
  return new Response(resp.body, { status: resp.status, headers: resp.headers });
}

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const url = new URL(req.url);
    const { pathname } = url;

    try {
      // Seed R2 with a JS module and WASM bytes so the loader's factory can
      // fetch them at *request time* instead of importing build-time bundled
      // strings/ArrayBuffers.
      if (pathname === "/seed") {
        await env.CODE_BUCKET.put("module.js", `export default { async fetch(req) { return Response.json({ ok: true, source: "r2-js-fetched" }); } };`);
        await env.CODE_BUCKET.put("add.wasm", addWasmBytes());
        return Response.json({ ok: true, seeded: ["module.js", "add.wasm"] });
      }

      // Factory fetches JS module TEXT from an R2 binding at request time,
      // then hands it to LOADER.get() as a plain string module.
      if (pathname === "/from-r2-js") {
        const worker = env.LOADER.get(`r2-js-${Date.now()}`, async () => {
          const obj = await env.CODE_BUCKET.get("module.js");
          if (!obj) throw new Error("module.js not found in R2 -- call /seed first");
          const src = await obj.text();
          return {
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: { "index.js": src },
          };
        });
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        return passthrough(resp);
      }

      // Factory fetches WASM bytes from R2 at request time as an ArrayBuffer,
      // combined with a build-time-bundled JS shell that imports it.
      if (pathname === "/from-r2-wasm") {
        const worker = env.LOADER.get(`r2-wasm-${Date.now()}`, async () => {
          const obj = await env.CODE_BUCKET.get("add.wasm");
          if (!obj) throw new Error("add.wasm not found in R2 -- call /seed first");
          const wasmBytes = await obj.arrayBuffer();
          return {
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: {
              "index.js": WASM_MAIN_SRC,
              "add.wasm": { wasm: wasmBytes },
            },
          };
        });
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        return passthrough(resp);
      }

      // Same idea but sourced from the ASSETS binding (Static Assets) instead
      // of R2 -- ASSETS.fetch() returns a Response, .text()/.arrayBuffer() it.
      if (pathname === "/from-assets") {
        const worker = env.LOADER.get(`assets-js-${Date.now()}`, async () => {
          const assetResp = await env.ASSETS.fetch("http://internal/module.js");
          if (!assetResp.ok) throw new Error(`ASSETS fetch failed: ${assetResp.status}`);
          const src = await assetResp.text();
          return {
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: { "index.js": src },
          };
        });
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        return passthrough(resp);
      }

      // Control: try WebAssembly.compile() from runtime-fetched bytes
      // OUTSIDE the Loader, in the parent worker's own isolate -- checks
      // whether Workers' dynamic-code-generation restriction blocks this
      // even in wrangler dev (production is stricter; see report).
      if (pathname === "/direct-wasm-compile") {
        try {
          const obj = await env.CODE_BUCKET.get("add.wasm");
          if (!obj) throw new Error("add.wasm not found in R2 -- call /seed first");
          const bytes = await obj.arrayBuffer();
          const mod = await WebAssembly.compile(bytes);
          const instance = new WebAssembly.Instance(mod, {});
          return Response.json({ ok: true, result: (instance.exports.add as any)(3, 4), path: "direct-no-loader" });
        } catch (e: any) {
          return Response.json({ ok: false, error: String(e?.message || e), stack: String(e?.stack || "") });
        }
      }

      // Real ~226MB goscript-transpiled kube-scheduler dependency graph
      // (main.js + 65 satellite bundles), seeded into R2 under "sched/*" by
      // the shell driver (not checked into the repo). Fetches every file
      // from R2 at request time, assembles the Loader modules map, and
      // times cold-start end to end.
      if (pathname === "/scheduler-load") {
        const t0 = Date.now();
        const mainObj = await env.SCHED_BUCKET.get("sched/main.js");
        if (!mainObj) throw new Error("sched/main.js not found in R2 -- run the seed script first");
        const mainSrc = await mainObj.text();
        const tMainFetched = Date.now();

        const modules: Record<string, string> = { "main.js": mainSrc };
        let satBytes = mainSrc.length;
        await Promise.all(
          SCHEDULER_SATELLITES.map(async (name) => {
            const obj = await env.SCHED_BUCKET.get(`sched/${name}`);
            if (!obj) throw new Error(`sched/${name} not found in R2`);
            const src = await obj.text();
            modules[name] = src;
            satBytes += src.length;
          }),
        );
        const tAllFetched = Date.now();

        try {
          const worker = env.LOADER.get(`scheduler-${Date.now()}`, () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "main.js",
            modules,
          }));
          const resp = await worker.getEntrypoint().fetch("http://internal/");
          const tLoaded = Date.now();
          const body = await resp.text();
          return Response.json({
            ok: true,
            moduleCount: Object.keys(modules).length,
            totalBytes: satBytes,
            timingsMs: {
              mainFetch: tMainFetched - t0,
              allSatellitesFetch: tAllFetched - tMainFetched,
              loaderGetAndFetch: tLoaded - tAllFetched,
              total: tLoaded - t0,
            },
            childStatus: resp.status,
            childBody: body.slice(0, 2000),
          });
        } catch (e: any) {
          return Response.json({
            ok: false,
            moduleCount: Object.keys(modules).length,
            totalBytes: satBytes,
            timingsMs: {
              mainFetch: tMainFetched - t0,
              allSatellitesFetch: tAllFetched - tMainFetched,
              total: Date.now() - t0,
            },
            error: String(e?.message || e),
            stack: String(e?.stack || ""),
          });
        }
      }

      // Real ~69MB GOOS=js/GOARCH=wasm kube-controller-manager-lean binary
      // (spikes/s13-kcm-lean-only), seeded into R2 as "sched/kcm.wasm" by
      // the shell driver. Just checks whether the Loader accepts a WASM
      // module this large via its `wasm: ArrayBuffer` field -- does NOT
      // attempt actual instantiation (that needs a full wasm_exec.js Go
      // runtime shim as the JS mainModule, out of scope for this size probe).
      if (pathname === "/kcm-wasm-load") {
        const t0 = Date.now();
        const obj = await env.SCHED_BUCKET.get("kcm.wasm");
        if (!obj) throw new Error("kcm.wasm not found in R2 -- run the seed step first");
        const wasmBytes = await obj.arrayBuffer();
        const tFetched = Date.now();
        try {
          const worker = env.LOADER.get(`kcm-wasm-${Date.now()}`, () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: {
              "index.js": `import kcmWasm from "./kcm.wasm";\nexport default { async fetch(req) { return Response.json({ ok: true, ctor: kcmWasm.constructor.name }); } };`,
              "kcm.wasm": { wasm: wasmBytes },
            },
          }));
          const resp = await worker.getEntrypoint().fetch("http://internal/");
          const tLoaded = Date.now();
          const body = await resp.text();
          return Response.json({
            ok: true,
            wasmBytes: wasmBytes.byteLength,
            timingsMs: { r2Fetch: tFetched - t0, loaderGetAndFetch: tLoaded - tFetched, total: tLoaded - t0 },
            childStatus: resp.status,
            childBody: body.slice(0, 500),
          });
        } catch (e: any) {
          return Response.json({
            ok: false,
            wasmBytes: wasmBytes.byteLength,
            timingsMs: { r2Fetch: tFetched - t0, total: Date.now() - t0 },
            error: String(e?.message || e),
          });
        }
      }

      // Real GOOS=js/GOARCH=wasm kube-controller-manager-lean binary, built
      // with `-ldflags="-s -w"` then `wasm-opt -Oz` (55,605,496 bytes,
      // comfortably under the 64MiB Loader cap found by /kcm-wasm-load),
      // seeded into R2 as "sched/kcm-opt.wasm" + Go's own "sched/wasm_exec.js"
      // runtime shim. Actually instantiates via the real wasm_exec.js Go
      // class and calls go.run(instance) -- this is the full "does a real
      // kube-controller-manager binary start executing inside a Cloudflare
      // Worker" test, not just a size probe.
      if (pathname === "/kcm-wasm-run") {
        const t0 = Date.now();
        const [wasmObj, execObj] = await Promise.all([
          env.SCHED_BUCKET.get("kcm-opt.wasm"),
          env.SCHED_BUCKET.get("wasm_exec.js"),
        ]);
        if (!wasmObj) throw new Error("kcm-opt.wasm not found in R2 -- run the seed step first");
        if (!execObj) throw new Error("wasm_exec.js not found in R2 -- run the seed step first");
        const [wasmBytes, execSrc] = await Promise.all([wasmObj.arrayBuffer(), execObj.text()]);
        const tFetched = Date.now();

        const RUNNER_SRC = `
${execSrc}
import kcmWasm from "./kcm.wasm";
export default {
  async fetch(req) {
    const log = [];
    const origLog = console.log;
    const origErr = console.error;
    console.log = (...args) => { log.push(String(args.join(" "))); };
    console.error = (...args) => { log.push("[err] " + String(args.join(" "))); };
    try {
      const go = new globalThis.Go();
      const t0 = Date.now();
      // kcmWasm is already a compiled WebAssembly.Module (imported via the
      // Loader's "wasm" module field), so instantiate() resolves directly
      // to an Instance here -- NOT a {module, instance} pair (that shape is
      // only returned when the first argument is raw bytes).
      const instance = await WebAssembly.instantiate(kcmWasm, go.importObject);
      const tInstantiated = Date.now();
      let runErr = null;
      const runPromise = go.run(instance).catch((e) => { runErr = e; });
      const timeout = new Promise((resolve) => setTimeout(resolve, 14000, "timeout"));
      const raceResult = await Promise.race([runPromise.then(() => "exited"), timeout]);
      return Response.json({
        ok: true,
        instantiateMs: tInstantiated - t0,
        raceResult,
        runErr: runErr ? String(runErr.message || runErr) : null,
        capturedLog: log.slice(0, 300),
      });
    } catch (e) {
      return Response.json({ ok: false, error: String(e.message || e), stack: String(e.stack || ""), capturedLog: log.slice(0, 100) });
    } finally {
      console.log = origLog;
      console.error = origErr;
    }
  }
};
`;
        try {
          const worker = env.LOADER.get(`kcm-run-${Date.now()}`, () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: {
              "index.js": RUNNER_SRC,
              "kcm.wasm": { wasm: wasmBytes },
            },
          }));
          const resp = await worker.getEntrypoint().fetch("http://internal/");
          const tLoaded = Date.now();
          const body = await resp.json();
          return Response.json({
            ok: true,
            wasmBytes: wasmBytes.byteLength,
            timingsMs: { r2Fetch: tFetched - t0, loaderGetAndRun: tLoaded - tFetched, total: tLoaded - t0 },
            childStatus: resp.status,
            child: body,
          });
        } catch (e: any) {
          return Response.json({
            ok: false,
            wasmBytes: wasmBytes.byteLength,
            timingsMs: { r2Fetch: tFetched - t0, total: Date.now() - t0 },
            error: String(e?.message || e),
            stack: String(e?.stack || ""),
          });
        }
      }

      // Part 7 re-test: same real kcm-opt.wasm binary as /kcm-wasm-run, but
      // this time (a) all 10 upstream controllers enabled (spikes/s13's
      // main.go re-enabled replicaset/deployment/daemon/job/cronjob/
      // endpoint/endpointslice now that pkg/leanclient/gen has a real
      // Events() instead of a panic stub), (b) pointed at a real running
      // k8flare stack on localhost:8860 instead of the dummy URL, (c) uses
      // a FIXED loader-worker id + ctx.waitUntil so go.run() keeps
      // executing in the background past this request's response, and a
      // separate /kcm-run-status request against the SAME id can poll
      // accumulated klog/MEMSTATS output over a longer wall-clock window
      // than a single request's timeout allows.
      if (pathname === "/kcm-run-start" || pathname === "/kcm-run-status") {
        const RUNNER_SRC_PERSIST = `
${await (await env.SCHED_BUCKET.get("wasm_exec.js"))!.text()}
import kcmWasm from "./kcm.wasm";
globalThis.__kcmLog = globalThis.__kcmLog || [];
globalThis.__kcmStarted = globalThis.__kcmStarted || false;
function pushLog(s) {
  globalThis.__kcmLog.push(s);
  if (globalThis.__kcmLog.length > 2000) globalThis.__kcmLog.shift();
}
export default {
  async fetch(req, env, ctx) {
    const url = new URL(req.url);
    if (url.pathname === "/status") {
      return Response.json({ started: globalThis.__kcmStarted, lines: globalThis.__kcmLog.length, log: globalThis.__kcmLog.slice(-400) });
    }
    if (!globalThis.__kcmStarted) {
      globalThis.__kcmStarted = true;
      console.log = (...args) => pushLog(String(args.join(" ")));
      console.error = (...args) => pushLog("[err] " + String(args.join(" ")));
      const go = new globalThis.Go();
      const instance = await WebAssembly.instantiate(kcmWasm, go.importObject);
      const runPromise = go.run(instance).catch((e) => pushLog("[run-exit] " + String(e && e.message || e)));
      ctx.waitUntil(runPromise);
      return Response.json({ ok: true, startedNow: true });
    }
    return Response.json({ ok: true, startedNow: false, alreadyRunning: true });
  }
};
`;
        try {
          const wasmObj = await env.SCHED_BUCKET.get("kcm-opt.wasm");
          if (!wasmObj) throw new Error("kcm-opt.wasm not found in R2");
          const wasmBytes = await wasmObj.arrayBuffer();
          const worker = env.LOADER.get("kcm-run-persist-fixed", () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: {
              "index.js": RUNNER_SRC_PERSIST,
              "kcm.wasm": { wasm: wasmBytes },
            },
          }));
          const innerPath = pathname === "/kcm-run-status" ? "/status" : "/";
          const resp = await worker.getEntrypoint().fetch("http://internal" + innerPath);
          const body = await resp.json();
          return Response.json({ ok: true, child: body });
        } catch (e: any) {
          return Response.json({ ok: false, error: String(e?.message || e), stack: String(e?.stack || "") });
        }
      }

      // Does a Loader-loaded worker's own env accept a WorkerLoader binding
      // (env.LOADER passed through, mirroring s2-loader item 3a's test for
      // DurableObjectNamespace/Fetcher), and if so, can that inner worker
      // call .get() on it to nest-load a SECOND Loader-loaded worker and
      // reach it via its Fetcher? Also: is WebAssembly.compile() blocked
      // inside a Loader-loaded worker the same way it is in the plain
      // parent isolate (item /direct-wasm-compile), or does being
      // Loader-loaded change that?
      if (pathname === "/loader-nesting") {
        const INNER_SRC = `
export default {
  async fetch(req, env, ctx) {
    const out = { hasLoaderBinding: !!env.LOADER, loaderType: env.LOADER ? env.LOADER.constructor.name : null };
    if (env.LOADER) {
      try {
        const nested = env.LOADER.get("nested-leaf-" + Date.now(), () => ({
          compatibilityDate: "2026-03-24",
          mainModule: "leaf.js",
          modules: { "leaf.js": "export default { async fetch(req) { return Response.json({ ok: true, leaf: true }); } };" },
        }));
        const r = await nested.getEntrypoint().fetch("http://internal/");
        out.nestedLoadResult = await r.json();
      } catch (e) {
        out.nestedLoadError = String((e && e.message) || e);
      }
    }
    try {
      const bytes = Uint8Array.from(atob("AGFzbQEAAAABBwFgAn9/AX8DAgEABwcBA2FkZAAACgkBBwAgACABags="), c => c.charCodeAt(0));
      const mod = await WebAssembly.compile(bytes.buffer);
      out.innerWasmCompile = { ok: true, ctor: mod.constructor.name };
    } catch (e) {
      out.innerWasmCompile = { ok: false, error: String((e && e.message) || e) };
    }
    return Response.json(out);
  }
};
`;
        // Attempt 1: try passing the LOADER binding itself through env (like
        // s2-loader item 3a tried for DurableObjectNamespace/Stub).
        let passthroughResult: any;
        try {
          const outerWorker = env.LOADER.get(`nesting-outer-${Date.now()}`, () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: { "index.js": INNER_SRC },
            env: { LOADER: env.LOADER },
          }));
          const resp = await outerWorker.getEntrypoint().fetch("http://internal/");
          passthroughResult = { ok: true, outerStatus: resp.status, outer: await resp.json() };
        } catch (e: any) {
          passthroughResult = { ok: false, error: String(e?.message || e) };
        }

        // Attempt 2: same INNER_SRC but with no env.LOADER passed (isolates
        // the WebAssembly.compile()-inside-a-loaded-worker question from the
        // env-passthrough question above, since attempt 1 never reaches the
        // worker's fetch() handler at all if the env clone itself fails).
        let wasmOnlyResult: any;
        try {
          const outerWorker2 = env.LOADER.get(`nesting-outer-noenv-${Date.now()}`, () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: { "index.js": INNER_SRC },
          }));
          const resp2 = await outerWorker2.getEntrypoint().fetch("http://internal/");
          wasmOnlyResult = { ok: true, status: resp2.status, body: await resp2.json() };
        } catch (e: any) {
          wasmOnlyResult = { ok: false, error: String(e?.message || e) };
        }

        return Response.json({ passthroughAttempt: passthroughResult, noEnvAttempt: wasmOnlyResult });
      }

      return Response.json({
        ok: true,
        routes: ["/seed", "/from-r2-js", "/from-r2-wasm", "/from-assets", "/direct-wasm-compile", "/scheduler-load", "/kcm-wasm-load", "/kcm-wasm-run", "/loader-nesting"],
      });
    } catch (e: any) {
      return Response.json({ ok: false, error: String(e?.message || e), stack: String(e?.stack || "") }, { status: 500 });
    }
  },
};
