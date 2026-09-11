//go:build js && wasm

package cloudflare

import (
	gocontext "context"
	"syscall/js"
	"testing"
)

// withBootContext installs the Go.run-time {env: ...} object the JS
// bootstrap normally threads into globalThis, so the last-resort fallback
// in EnvFromContext has something to return.
func withBootContext(t *testing.T, marker string) {
	t.Helper()
	env := js.Global().Get("Object").New()
	env.Set("marker", marker)
	ctxObj := js.Global().Get("Object").New()
	ctxObj.Set("env", env)
	js.Global().Set("context", ctxObj)
	t.Cleanup(func() { js.Global().Delete("context") })
}

func TestEnvFromContextPrefersTheRequestScopedEnv(t *testing.T) {
	retireAllWindows(t)
	withBootContext(t, "boot")
	openWindow(t, "window", 60_000)

	reqEnv := js.Global().Get("Object").New()
	reqEnv.Set("marker", "request")
	got := EnvFromContext(WithEnv(gocontext.Background(), reqEnv))
	if got.Get("marker").String() != "request" {
		t.Fatalf("EnvFromContext returned %q, want the request-scoped env", got.Get("marker"))
	}
}

// A caller with no context (pkg/apiserver's vault read builds an
// http.Request that carries none) must land on a request that is still
// allowed to perform I/O, not on the long-dead request that instantiated
// the isolate.
func TestEnvFromContextFallsBackToTheOpenWindow(t *testing.T) {
	retireAllWindows(t)
	withBootContext(t, "boot")
	openWindow(t, "window", 60_000)

	got := EnvFromContext(gocontext.Background())
	if got.Get("marker").String() != "window" {
		t.Fatalf("EnvFromContext returned %q, want the open window's env", got.Get("marker"))
	}
}

func TestEnvFromContextFallsBackToBootEnvWithNoWindowOpen(t *testing.T) {
	retireAllWindows(t)
	withBootContext(t, "boot")

	got := EnvFromContext(gocontext.Background())
	if got.Get("marker").String() != "boot" {
		t.Fatalf("EnvFromContext returned %q, want the boot env", got.Get("marker"))
	}
}

func TestBindingFromContextResolvesOnTheOpenWindow(t *testing.T) {
	retireAllWindows(t)
	withBootContext(t, "boot")
	_, env := openWindow(t, "window", 60_000)
	binding := js.Global().Get("Object").New()
	binding.Set("marker", "window-storage")
	env.Set("STORAGE", binding)

	got := BindingFromContext(gocontext.Background(), "STORAGE")
	if got.Get("marker").String() != "window-storage" {
		t.Fatalf("BindingFromContext returned %q, want the open window's binding", got.Get("marker"))
	}
}

func TestGetenvDefaultFallsBackWhenUnset(t *testing.T) {
	withBootContext(t, "boot")
	if got := GetenvDefault("K3S_TOKEN", "k8flare-dev-token"); got != "k8flare-dev-token" {
		t.Fatalf("GetenvDefault = %q, want the fallback", got)
	}
	js.Global().Get("context").Get("env").Set("K3S_TOKEN", "real")
	if got := GetenvDefault("K3S_TOKEN", "k8flare-dev-token"); got != "real" {
		t.Fatalf("GetenvDefault = %q, want the set value", got)
	}
}
