import { addWasmBytes } from "./wasm-fixture.ts";

export interface Env {
  LOADER: any; // WorkerLoader — @cloudflare/workers-types doesn't export the name publicly yet
  COUNTER_DO: DurableObjectNamespace;
  REMOTE_SVC: Fetcher;
  SIMPLE_VALUE: string;
}

// Durable Object owned by the parent worker itself, used only to test
// whether a DO namespace binding can be forwarded into a Loader-loaded
// worker's env (S2 item 3a).
export class Counter {
  state: DurableObjectState;
  constructor(state: DurableObjectState) {
    this.state = state;
  }
  async fetch(_req: Request): Promise<Response> {
    let count = ((await this.state.storage.get("count")) as number) || 0;
    count++;
    await this.state.storage.put("count", count);
    return Response.json({ count });
  }
}

// Module-scope counter in the PARENT isolate. Only incremented inside the
// getCode() factory passed to LOADER.get() — lets us tell empirically
// whether a given call to .get(id, factory) actually invoked the factory
// (cache miss) or reused an already-loaded worker for that id (cache hit).
let factoryCallCount = 0;

const COUNTER_CHILD_SRC = `
let counter = 0;
export default {
  async fetch(req) {
    counter++;
    return Response.json({ counter });
  }
};
`;

function sizedModuleSrc(totalBytes: number): string {
  const prefix = 'export default { fetch(req) { return new Response(String(PAD.length)); } };\nconst PAD = "';
  const suffix = '";\n';
  const padLen = Math.max(0, totalBytes - prefix.length - suffix.length);
  return prefix + "A".repeat(padLen) + suffix;
}

const ENV_CHILD_SRC = `
export default {
  async fetch(req, env, ctx) {
    const out = {
      simpleValue: env.SIMPLE_VALUE,
      hasDoBinding: !!env.COUNTER_DO,
      hasSvcBinding: !!env.REMOTE_SVC,
      hasDoStub: !!env.COUNTER_STUB,
    };
    try {
      const id = env.COUNTER_DO.idFromName("child-test");
      const stub = env.COUNTER_DO.get(id);
      const r = await stub.fetch("http://internal/increment");
      out.doResult = await r.json();
    } catch (e) {
      out.doError = String((e && e.message) || e);
    }
    try {
      const r2 = await env.REMOTE_SVC.fetch("http://internal/ping");
      out.svcResult = await r2.json();
    } catch (e) {
      out.svcError = String((e && e.message) || e);
    }
    try {
      const r3 = await env.COUNTER_STUB.fetch("http://internal/increment");
      out.doStubResult = await r3.json();
    } catch (e) {
      out.doStubError = String((e && e.message) || e);
    }
    return Response.json(out);
  }
};
`;

function outboundChildSrc(targetUrl: string): string {
  return `
export default {
  async fetch(req, env, ctx) {
    try {
      const r = await fetch(${JSON.stringify(targetUrl)});
      const text = await r.text();
      return Response.json({ ok: true, status: r.status, bodyPrefix: text.slice(0, 60) });
    } catch (e) {
      return Response.json({ ok: false, error: String((e && e.message) || e) });
    }
  }
};
`;
}

const WASM_MAIN_SRC = `
import wasmModule from "./add.wasm";
export default {
  async fetch(req, env, ctx) {
    const instance = new WebAssembly.Instance(wasmModule, {});
    const result = instance.exports.add(3, 4);
    return Response.json({ ok: true, result, moduleCtor: wasmModule.constructor.name });
  }
};
`;

async function passthrough(resp: Response): Promise<Response> {
  return new Response(resp.body, { status: resp.status, headers: resp.headers });
}

export default {
  async fetch(req: Request, env: Env, _ctx: ExecutionContext): Promise<Response> {
    const url = new URL(req.url);
    const { pathname, searchParams } = url;

    try {
      // --- Item 1: WASM module in a Loader-loaded worker ---
      if (pathname === "/wasm") {
        const worker = env.LOADER.get("wasm-test", () => ({
          compatibilityDate: "2026-03-24",
          mainModule: "index.js",
          modules: {
            "index.js": WASM_MAIN_SRC,
            "add.wasm": { wasm: addWasmBytes() },
          },
        }));
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        return passthrough(resp);
      }

      // --- Item 2: size ceiling, step via ?mb=1|5|10|20 ---
      if (pathname === "/size") {
        const mb = Number(searchParams.get("mb") || "1");
        const totalBytes = Math.floor(mb * 1024 * 1024);
        const src = sizedModuleSrc(totalBytes);
        const start = Date.now();
        try {
          const worker = env.LOADER.get(`size-test-${mb}-${Date.now()}`, () => ({
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: { "index.js": src },
          }));
          const resp = await worker.getEntrypoint().fetch("http://internal/");
          const body = await resp.text();
          return Response.json({
            ok: true,
            requestedMb: mb,
            actualSrcBytes: src.length,
            elapsedMs: Date.now() - start,
            childReportedPadLen: body,
          });
        } catch (e: any) {
          return Response.json({
            ok: false,
            requestedMb: mb,
            actualSrcBytes: src.length,
            elapsedMs: Date.now() - start,
            error: String(e?.message || e),
            stack: String(e?.stack || ""),
          });
        }
      }

      // --- Item 3: env bindings forwarded into a Loader-loaded worker ---
      if (pathname === "/env") {
        const fields = (searchParams.get("fields") || "simple,do,svc").split(",");
        const childEnv: any = {};
        if (fields.includes("simple")) childEnv.SIMPLE_VALUE = env.SIMPLE_VALUE;
        if (fields.includes("do")) childEnv.COUNTER_DO = env.COUNTER_DO;
        if (fields.includes("svc")) childEnv.REMOTE_SVC = env.REMOTE_SVC;
        if (fields.includes("stub")) childEnv.COUNTER_STUB = env.COUNTER_DO.get(env.COUNTER_DO.idFromName("child-test"));
        const worker = env.LOADER.get(`env-test-${fields.join("-")}`, () => ({
          compatibilityDate: "2026-03-24",
          mainModule: "index.js",
          modules: { "index.js": ENV_CHILD_SRC },
          env: childEnv,
        }));
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        return passthrough(resp);
      }

      // --- Item 3 (globalOutbound): none | inherit | custom-svc | custom-plain ---
      if (pathname === "/outbound") {
        const mode = searchParams.get("mode") || "none";
        const targetUrl = searchParams.get("url") || "https://cloudflare.com/robots.txt";
        const src = outboundChildSrc(targetUrl);
        const codeBase: any = {
          compatibilityDate: "2026-03-24",
          mainModule: "index.js",
          modules: { "index.js": src },
        };
        if (mode === "none") {
          codeBase.globalOutbound = null;
        } else if (mode === "inherit") {
          // leave globalOutbound unset -> default behavior
        } else if (mode === "custom-svc") {
          codeBase.globalOutbound = env.REMOTE_SVC;
        } else if (mode === "custom-plain") {
          codeBase.globalOutbound = {
            fetch: async (r: Request) => Response.json({ intercepted: true, url: String(r.url ?? r) }),
          };
        }
        const worker = env.LOADER.get(`outbound-test-${mode}`, () => codeBase);
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        return passthrough(resp);
      }

      // --- Item 4: get() reuse/caching per id ---
      if (pathname === "/counter") {
        const id = searchParams.get("id") || "A";
        const before = factoryCallCount;
        const worker = env.LOADER.get(id, () => {
          factoryCallCount++;
          return {
            compatibilityDate: "2026-03-24",
            mainModule: "index.js",
            modules: { "index.js": COUNTER_CHILD_SRC },
          };
        });
        const resp = await worker.getEntrypoint().fetch("http://internal/");
        const body: any = await resp.json();
        return Response.json({
          id,
          factoryInvokedThisCall: factoryCallCount > before,
          factoryCallCountTotal: factoryCallCount,
          ...body,
        });
      }

      return Response.json({
        ok: true,
        routes: [
          "/wasm",
          "/size?mb=1|5|10|20",
          "/env",
          "/outbound?mode=none|inherit|custom-svc|custom-plain&url=...",
          "/counter?id=A",
        ],
      });
    } catch (e: any) {
      return Response.json({ ok: false, error: String(e?.message || e), stack: String(e?.stack || "") }, { status: 500 });
    }
  },
};
