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

// EnvFromContext returns the request-scoped env attached by WithEnv,
// falling back to the newest open pump window's env and, only when this
// isolate has no open window at all, to the Go.run-time global env
// (context()). The window fallback is what keeps a context-less caller
// (pkg/apiserver's vault read, whose http.NewRequest carries none) on a
// request that is still allowed to perform I/O rather than on the
// long-dead request that instantiated the isolate.
func EnvFromContext(ctx gocontext.Context) js.Value {
	if v, ok := ctx.Value(envContextKey{}).(js.Value); ok {
		return v
	}
	if env, ok := currentWindowEnv(); ok {
		return env
	}
	return context().Get("env")
}

// BindingFromContext returns the named binding from the request-scoped
// env -- the per-request analog of GetBinding, for resident binaries that
// must not reuse an earlier request's Fetcher.
func BindingFromContext(ctx gocontext.Context, name string) js.Value {
	return EnvFromContext(ctx).Get(name)
}
