// Real apimachinery label/field selector matching for the watch
// fan-out, replacing the hand-rolled TypeScript subset that used to
// live in label-selector.ts + watch.ts's parseFieldSelector (deleted
// with this file's introduction). The Go binary
// (pkg/selectormatch/cmd/selector-wasm, built by `make wasm-selector`
// into assets/wasm/selector.wasm) ships inside THIS Worker script as a
// dynamically imported wasm module -- not a Loader dynamic worker:
// production Workers forbid runtime WebAssembly compilation, and a
// Loader hop would put a cold start on the watch hot path. The import
// is dynamic so the 4.58MB module stays out of the eager entry graph
// (S35): only a watch carrying a selector pays for it, and under
// new_module_registry a module is compiled when first imported.
//
// Instantiated lazily once per isolate (~26ms; Go runtime init calls
// crypto.getRandomValues, which Workers disallow at module-eval time)
// and reused synchronously across independent fetch() events -- S8's
// IoContext constraint does not apply to synchronous js.FuncOf exports,
// verified live in docs/platform-verification.md S22.

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

interface SelectorExports {
  k8flareValidateSelectors(labelSelector: string, fieldSelector: string): string;
  k8flareMatchSelectors(
    objJSON: string,
    labelSelector: string,
    fieldSelector: string,
  ): { match: boolean; err: string };
}

let booted: SelectorExports | null = null;
let booting: Promise<SelectorExports> | null = null;

async function boot(): Promise<SelectorExports> {
  await import("@wasm/wasm_exec.js");
  const { default: selectorWasmModule } = await import("@wasm/selector.wasm");
  const go = new (globalThis as unknown as { Go: new () => GoRuntime }).Go();
  const instance = new WebAssembly.Instance(selectorWasmModule, go.importObject);
  // Not awaited: main() sets the js.FuncOf exports synchronously
  // before blocking forever, so they are installed on globalThis by
  // the time run() yields (S22). The promise only settles if the Go
  // runtime dies -- surface that so the next call re-instantiates.
  void go.run(instance).catch((e) => {
    console.error(`selector-wasm: Go runtime exited: ${e}`);
    booted = null;
    booting = null;
  });
  return globalThis as unknown as SelectorExports;
}

/** Load and instantiate the selector wasm, memoizing the in-flight boot
 * so concurrent watch opens share one Go runtime. Must be awaited before
 * validateSelectors / objectMatchesSelectors, which stay synchronous
 * because they run per event per watcher in the broadcast path. */
export async function ensureSelectorsReady(): Promise<void> {
  if (booted) return;
  if (!booting) {
    booting = boot().catch((e) => {
      booting = null;
      throw e;
    });
  }
  booted = await booting;
}

/** Validate selector strings, returning "" if OK or the parse error --
 * called once at watch-open so a bad selector 400s like upstream
 * instead of being silently mis-applied per event. */
export function validateSelectors(labelSelector: string, fieldSelector: string): string {
  return ready().k8flareValidateSelectors(labelSelector, fieldSelector);
}

/** Whether the object satisfies both selectors (real apimachinery
 * semantics, including set-based label operators the old TS subset
 * lacked). Selector strings must have passed validateSelectors. */
export function objectMatchesSelectors(
  obj: Record<string, unknown>,
  labelSelector: string,
  fieldSelector: string,
): boolean {
  const r = ready().k8flareMatchSelectors(JSON.stringify(obj), labelSelector, fieldSelector);
  if (r.err) {
    // Selectors were validated at watch open; an error here means the
    // OBJECT was unserializable, which cannot happen for decoded kine
    // values. Fail closed (no match) but say why.
    console.log(`selector-wasm: match error: ${r.err}`);
    return false;
  }
  return r.match;
}

function ready(): SelectorExports {
  if (!booted) throw new Error("selector-wasm: ensureSelectorsReady() was not awaited");
  return booted;
}
