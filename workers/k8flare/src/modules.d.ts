// Bundled-module shims for the selector wasm (see k8s/selector-wasm.ts).
// Declared by exact aliased name (tsconfig "paths" maps @wasm/* to
// ../assets/wasm/*), deliberately WITHOUT resolving the real files:
// they are build outputs of `make wasm-selector`, and CI's type checks
// run before its wasm build step. wrangler's esbuild resolves the real
// files at dev/deploy time (and fails loudly there if unbuilt).
declare module "@wasm/selector.wasm" {
  const wasmModule: WebAssembly.Module;
  export default wasmModule;
}
declare module "@wasm/wasm_exec.js";
