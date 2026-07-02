// Minimal hand-assembled WASM module (no toolchain dependency):
//   (module
//     (func (export "add") (param i32 i32) (result i32)
//       local.get 0
//       local.get 1
//       i32.add))
//
// Verified independently with `node -e` + WebAssembly.instantiate() before
// use here: add(3,4) === 7, byteLength === 41. See FINDINGS.md item 1.
export const ADD_WASM_BASE64 = "AGFzbQEAAAABBwFgAn9/AX8DAgEABwcBA2FkZAAACgkBBwAgACABags=";

export function addWasmBytes(): ArrayBuffer {
  const bin = atob(ADD_WASM_BASE64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes.buffer;
}
