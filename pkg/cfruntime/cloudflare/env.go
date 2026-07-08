package cloudflare

import "syscall/js"

// context returns this invocation's {env, ctx, connect, binding} object
// -- Go.run's second argument (workers/k8flare/src/loader/bootstrap.ts's
// runtimeCtx), threaded into globalThis by the patched wasm_exec.js (see
// pkg/cfruntime/README.md and packages/wasm-build/src/patch-wasm-exec.ts).
func context() js.Value {
	v := js.Global().Get("context")
	if v.IsUndefined() {
		panic("cloudflare: no runtime context -- called before Go.run")
	}
	return v
}

// Getenv reads a Workers environment variable.
// https://developers.cloudflare.com/workers/platform/environment-variables/
func Getenv(name string) string {
	if v := context().Get("env").Get(name); !v.IsUndefined() {
		return v.String()
	}
	return ""
}

// GetBinding reads a Workers environment binding (KV namespace, R2
// bucket, service binding, ...) -- the same underlying env object Getenv
// reads, just returned as a raw js.Value instead of coerced to a string.
// https://developers.cloudflare.com/workers/platform/bindings/about-service-bindings/
func GetBinding(name string) js.Value {
	return context().Get("env").Get(name)
}

// WaitUntil extends the lifetime of the current request so task can keep
// running after the response is sent.
// https://developers.cloudflare.com/workers/runtime-apis/fetch-event/#waituntil
func WaitUntil(task func()) {
	var executor js.Func
	executor = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer executor.Release()
		resolve := args[0]
		go func() {
			task()
			resolve.Invoke(js.Undefined())
		}()
		return js.Undefined()
	})
	promise := js.Global().Get("Promise").New(executor)
	context().Get("ctx").Call("waitUntil", promise)
}
