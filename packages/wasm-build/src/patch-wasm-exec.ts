#!/usr/bin/env node
// Patches $(go env GOROOT)/lib/wasm/wasm_exec.js for the Cloudflare
// Workers js/wasm runtime, in place of hand-maintaining a full copy that
// silently drifts from whatever Go version actually compiled the WASM
// binary. The only change needed (verified: normalizing both files
// through prettier and diffing reduces the gap to exactly this) is
// threading a `context` argument into Go.run and exposing it through
// globalThis via a Proxy -- see pkg/cfruntime/README.md and
// docs/platform-verification.md's S2/S8 sections for why.
//
// Usage: node patch-wasm-exec.ts <input.js> <output.js>
import * as fs from "node:fs";

const [input, output] = process.argv.slice(2);
if (!input || !output) {
  console.error("usage: patch-wasm-exec.ts <input.js> <output.js>");
  process.exit(1);
}

let src = fs.readFileSync(input, "utf8");

const runAnchor = "async run(instance) {";
if (src.indexOf(runAnchor) !== src.lastIndexOf(runAnchor) || !src.includes(runAnchor)) {
  console.error(
    `patch-wasm-exec: expected exactly one "${runAnchor}" in ${input} -- upstream wasm_exec.js changed, review this patch`,
  );
  process.exit(1);
}
src = src.replace(runAnchor, "async run(instance, context) {");

const valuesAnchor =
  "this._values = [ // JS values that Go currently has references to, indexed by reference id";
if (src.indexOf(valuesAnchor) !== src.lastIndexOf(valuesAnchor) || !src.includes(valuesAnchor)) {
  console.error(
    `patch-wasm-exec: expected exactly one values-array anchor in ${input} -- upstream wasm_exec.js changed, review this patch`,
  );
  process.exit(1);
}
const proxyDecl =
  `// Cloudflare Workers patch: expose the per-invocation {env, ctx,
			// connect, binding} context object (this.run's new second argument)
			// through globalThis, since syscall/js code (cloudflare.Getenv etc.)
			// can only reach values already on globalThis. A Proxy is required
			// (not a plain assignment) because ` +
  "`context`" +
  ` is per-invocation, not a
			// singleton the wasm module can be initialized with once.
			//
			// fetch and setTimeout are bound to the real target, deliberately
			// narrowly: both do a receiver/brand check and throw "Illegal
			// invocation" when called with this Proxy as \`this\` (which happens
			// whenever Go code calls js.Global().Call("fetch"/"setTimeout", ...),
			// since js.Global() now resolves to this proxy). setTimeout is
			// needed by pkg/cfruntime's yieldToEventLoop (see its doc comment:
			// resolve/reject.Invoke only schedules delivery of the response as a
			// microtask, and setTimeout(...,0) is how Go waits for that to
			// actually run before letting main() return and this WASM instance
			// be torn down -- confirmed live: omitting this bind reproduces the
			// exact "Illegal invocation" panic, one goroutine stack frame inside
			// yieldToEventLoop). An earlier attempt bound every function
			// returned through the trap unconditionally, which broke static
			// methods retrieved through the same proxy (e.g. Array.bind(target).from
			// is undefined, since Function.prototype.bind does not forward a
			// function's own properties) and crashed header parsing that relied on
			// Array.from -- see docs/platform-verification.md's S8 section. Keep
			// this list narrow and explicit, one function at a time, not a
			// blanket bind.
			const boundGlobals = new Set(["fetch", "setTimeout"]);
			const globalProxy = new Proxy(globalThis, {
				get(target, prop) {
					if (prop === "context") {
						return context;
					}
					const val = Reflect.get(target, prop, target);
					if (boundGlobals.has(prop) && typeof val === "function") {
						return val.bind(target);
					}
					return val;
				},
			});
			`;
src = src.replace(valuesAnchor, proxyDecl + valuesAnchor);

// Both occurrences of bare `globalThis` inside the (now patched) run()
// method -- the _values array entry and its matching _refIds key --
// must resolve through the proxy instead, so JS code holding "the global
// object" via a Go-originated js.Value (js.Global()) sees `context` and
// gets fetch()/setTimeout() bound correctly. Every other globalThis
// reference in this file (globalThis.fs, globalThis.process,
// globalThis.Go, ...) is untouched -- those run before/outside Go.run
// and refer to the real object on purpose.
const beforeCount =
  (src.match(/^\t\t\t\tglobalThis,$/m) ?? []).length +
  (src.match(/^\t\t\t\t\[globalThis, 5\],$/m) ?? []).length;
if (beforeCount !== 2) {
  console.error(
    `patch-wasm-exec: expected exactly 2 globalThis references inside run() in ${input}, found ${beforeCount} -- upstream wasm_exec.js changed, review this patch`,
  );
  process.exit(1);
}
src = src.replace(/^(\t\t\t\t)globalThis,$/m, "$1globalProxy,");
src = src.replace(/^(\t\t\t\t)\[globalThis, 5\],$/m, "$1[globalProxy, 5],");

fs.writeFileSync(output, src);
console.log(`patch-wasm-exec: wrote ${output}`);
