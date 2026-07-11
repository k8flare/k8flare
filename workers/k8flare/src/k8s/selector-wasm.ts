// Real apimachinery label/field selector matching for the watch
// fan-out, replacing the hand-rolled TypeScript subset that used to
// live in label-selector.ts + watch.ts's parseFieldSelector (deleted
// with this file's introduction). The Go binary
// (pkg/selectormatch/cmd/selector-wasm, built by `make wasm-selector`
// into assets/wasm/selector.wasm) is bundled into THIS Worker script as
// a plain wasm module import -- not a Loader dynamic worker: production
// Workers forbid runtime WebAssembly compilation, and a Loader hop
// would put a cold start on the watch hot path.
//
// Instantiated lazily once per isolate (~26ms; Go runtime init calls
// crypto.getRandomValues, which Workers disallow at module-eval time)
// and reused synchronously across independent fetch() events -- S8's
// IoContext constraint does not apply to synchronous js.FuncOf exports,
// verified live in docs/platform-verification.md S22.
import "@wasm/wasm_exec.js";
import selectorWasmModule from "@wasm/selector.wasm";

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

let booted = false;

function ensureBooted(): SelectorExports {
  if (!booted) {
    const go = new (globalThis as unknown as { Go: new () => GoRuntime }).Go();
    const instance = new WebAssembly.Instance(selectorWasmModule, go.importObject);
    // Not awaited: main() sets the js.FuncOf exports synchronously
    // before blocking forever, so they are installed on globalThis by
    // the time run() yields (S22). The promise only settles if the Go
    // runtime dies -- surface that so the next call re-instantiates.
    void go.run(instance).catch((e) => {
      console.log(`selector-wasm: Go runtime exited: ${e}`);
      booted = false;
    });
    booted = true;
  }
  return globalThis as unknown as SelectorExports;
}

/** Validate selector strings, returning "" if OK or the parse error --
 * called once at watch-open so a bad selector 400s like upstream
 * instead of being silently mis-applied per event. */
export function validateSelectors(labelSelector: string, fieldSelector: string): string {
  return ensureBooted().k8flareValidateSelectors(labelSelector, fieldSelector);
}

/** Whether the object satisfies both selectors (real apimachinery
 * semantics, including set-based label operators the old TS subset
 * lacked). Selector strings must have passed validateSelectors. */
export function objectMatchesSelectors(
  obj: Record<string, unknown>,
  labelSelector: string,
  fieldSelector: string,
): boolean {
  const r = ensureBooted().k8flareMatchSelectors(JSON.stringify(obj), labelSelector, fieldSelector);
  if (r.err) {
    // Selectors were validated at watch open; an error here means the
    // OBJECT was unserializable, which cannot happen for decoded kine
    // values. Fail closed (no match) but say why.
    console.log(`selector-wasm: match error: ${r.err}`);
    return false;
  }
  return r.match;
}
