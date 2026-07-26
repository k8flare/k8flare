package cloudflare

import (
	// Aliased: this package already has an unexported context() helper
	// returning the JS runtime context object (below), which would collide
	// with the standard library's package name.
	gocontext "context"
	"syscall/js"
)

// context returns this invocation's {env, ctx, connect, binding} object
// -- Go.run's second argument (packages/k8flare-worker/src/loader/bootstrap.ts's
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

// GetenvDefault reads a Workers environment variable, falling back to
// def if unset -- e.g. every cmd/*-wasm entrypoint's K3S_TOKEN ->
// "k8flare-dev-token" fallback for local `wrangler dev` (see CLAUDE.md's
// local-dev-pitfalls list).
func GetenvDefault(name, def string) string {
	if v := Getenv(name); v != "" {
		return v
	}
	return def
}

// GetBinding reads a Workers environment binding (KV namespace, R2
// bucket, service binding, ...) -- the same underlying env object Getenv
// reads, just returned as a raw js.Value instead of coerced to a string.
// https://developers.cloudflare.com/workers/platform/bindings/about-service-bindings/
//
// NOTE for resident WASM binaries: this returns the env captured at
// Go.run time, i.e. whichever request first instantiated the isolate. A
// Fetcher binding is a request-scoped I/O object -- calling it from a
// later request's dispatch throws "Cannot perform I/O on behalf of a
// different request" once the capturing request's IoContext ends. Per-
// request handlers must use BindingFromContext instead. GetBinding stays
// correct for (a) per-request binaries and (b) the resident controllers,
// which capture their outbound binding once inside their own resident
// request's WaitUntil and use it only from within that still-live context
// (restconfig.go).
func GetBinding(name string) js.Value {
	return context().Get("env").Get(name)
}

// envContextKey keys the request-scoped env object attached by WithEnv.
type envContextKey struct{}

// WithEnv attaches this request's env object to ctx so resident handlers
// can resolve bindings scoped to the request currently being served
// rather than the module-global one from Go.run time. pkg/cfruntime's
// dispatch calls this with the env the JS bootstrap forwarded alongside
// the request.
func WithEnv(ctx gocontext.Context, env js.Value) gocontext.Context {
	return gocontext.WithValue(ctx, envContextKey{}, env)
}

// EnvFromContext returns the request-scoped env attached by WithEnv, or
// the Go.run-time global env (context()) when none is present -- so
// callers that predate request-scoped env, and the resident controllers,
// keep their existing behavior.
func EnvFromContext(ctx gocontext.Context) js.Value {
	if v, ok := ctx.Value(envContextKey{}).(js.Value); ok {
		return v
	}
	return context().Get("env")
}

// BindingFromContext returns the named binding from the request-scoped
// env -- the per-request analog of GetBinding, for resident binaries that
// must not reuse an earlier request's Fetcher.
func BindingFromContext(ctx gocontext.Context, name string) js.Value {
	return EnvFromContext(ctx).Get(name)
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
