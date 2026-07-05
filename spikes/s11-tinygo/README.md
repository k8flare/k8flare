# S11: TinyGo viability for kube-scheduler / kube-controller-manager

Throwaway decision-experiment scaffolding. Answers: can TinyGo (a
different Go compiler toolchain, not `GOOS=js GOARCH=wasm go build`)
close the raw-size gap stock Go leaves for `pkg/controllers.RunScheduler`
/ `RunControllerManager` under the Worker Loader 64MiB raw ceiling (see
`spikes/s10-scheduler-size/README.md` for the stock-Go numbers this
compares against)?

**Result: no. TinyGo cannot build this repository's Worker at all**, for
reasons that have nothing to do with size and are unrelated to the
`reflect`-completeness concern this experiment set out to check —
`net/http` itself does not compile under TinyGo 0.41.1's `wasm` target,
before client-go or k8s.io/kubernetes are even reached. This file holds
the repro scaffolding and the full writeup (not yet promoted to
`docs/platform-verification.md` -- this spike's instructions scoped
writes to this directory only).

## Layout

- `step-a-baseline/`: minimal `github.com/syumai/workers` Worker, zero
  k8s.io imports. Establishes whether TinyGo + syumai/workers works at
  all.
- `step-b-clientset/`: `k8s.io/client-go/kubernetes` typed Clientset
  only. Deliberately does **not** import `github.com/syumai/workers` (see
  the file's doc comment) so the client-go question can be answered
  independently of step A's failure.
- `step-c-real/`: the real thing -- imports `github.com/k8flare/k8flare/pkg/controllers`
  and calls both `RunScheduler` and `RunControllerManager`, same shape as
  `spikes/s10-scheduler-size/combined/main.go` but built with `tinygo`
  instead of stock `go`.

All three are part of the root module (no nested `go.mod`), same
convention as `spikes/s10-scheduler-size`, so they inherit the root
`go.mod`'s `replace` directives (`syumai/workers` fork,
`k8s.io/kubernetes` -> `.build/k8s-js-mirror`). `.build/k8s-js-mirror`
must exist first (`scripts/gen-k8s-js-mirror.sh`, or `npm run build:wasm`)
for step-c, since `pkg/controllers/scheduler.go` imports
`k8s.io/kubernetes/pkg/scheduler`.

## Toolchain

```
mise use -g aqua:tinygo-org/tinygo   # needs GITHUB_TOKEN=$(gh auth token) in env
                                      # to dodge unauthenticated GitHub API rate limits
mise exec -- tinygo version          # 0.41.1, using go version go1.26.2 -- matches
                                      # this repo's go.mod `go 1.26.2` exactly
```

## Reproducing

```
mise exec -- tinygo build -target wasm -o /tmp/out.wasm ./spikes/s11-tinygo/step-a-baseline
mise exec -- tinygo build -target wasm -o /tmp/out.wasm ./spikes/s11-tinygo/step-b-clientset
mise exec -- tinygo build -target wasm -o /tmp/out.wasm ./spikes/s11-tinygo/step-c-real
```

`-target wasm` (not `wasip1`/`wasip2`) is required: `tinygo info
-target=wasm` reports `GOOS: js / GOARCH: wasm`, the same pair stock Go
uses, and is the only TinyGo target implementing the `syscall/js`
JS-value-interop ABI `syumai/workers` and its `wasm_exec_tinygo.js` glue
(already vendored in this repo at
`third_party/syumai-workers-fork/cmd/workers-assets-gen/assets/wasm_exec_tinygo.js`)
depend on. WASI targets don't implement `syscall/js` at all and are not
an alternative here.

## What actually happened (short version)

All three steps hit the identical error on an unmodified TinyGo install:

```
# net/http
.../tinygo/src/net/http/roundtrip_js.go:73:12: t.roundTrip undefined
  (type *Transport has no field or method roundTrip, but does have method RoundTrip)
```

Confirmed with a zero-dependency isolation case (`import _ "net/http"`)
that this is a bug/gap in TinyGo's own vendored `net/http` for the `wasm`
target, not anything caused by this repo's code: TinyGo's
`transport.go` declares `type Transport struct{}` (no fields, no
methods) for this target, while its copy of upstream's
`roundtrip_js.go` still calls a `t.roundTrip(req)` fallback that was
never (re)implemented alongside it.

To find out whether this is the *only* blocker or one of several, a
temporary diagnostic stub was applied directly to the TinyGo
installation's own `src/net/http/transport.go` (**not** to anything in
this repository -- backed up and reverted immediately after this
experiment; the installation is a `mise`-managed copy under
`~/.local/share/mise/installs/aqua-tinygo-org-tinygo/`, shared across
projects on this machine, so leaving it patched was not an option):

```go
// added right after `var DefaultTransport RoundTripper = &Transport{}`
func (t *Transport) roundTrip(req *Request) (*Response, error) {
	return nil, errUnsupported
}
var errUnsupported = &unsupportedError{}
type unsupportedError struct{}
func (*unsupportedError) Error() string { return "s11 spike stub: roundTrip not implemented" }
```

That alone unblocked **step-a**: `tinygo build -target wasm` succeeded,
producing a 2,184,417 byte (2.18MiB) raw / 631,849 byte (631KiB) gzip
`.wasm` -- much smaller than stock Go's equivalent empty-`main`-plus-
syumai/workers baseline (1.58MiB gzip per
`docs/platform-verification.md`'s S8/Phase-5 numbers), consistent with
TinyGo's usual size advantage. So the syumai/workers integration itself
is not fundamentally incompatible with TinyGo -- it was blocked by
exactly one narrow, identifiable stdlib gap.

**step-b** (client-go typed Clientset, no syumai/workers) hit a second,
unrelated gap with that same stub applied:

```
# k8s.io/utils/net
.../k8s.io/utils@v0.0.0-20260319190234-28399d86e0b5/net/port.go:120:20: undefined: net.ListenUDP
```

`net.ListenUDP` does not exist anywhere in TinyGo's `net` package for
any target (`DialUDP` exists, no listen-only counterpart) -- confirmed
by grepping the installed toolchain source, not assumed. A second
diagnostic stub (`func ListenUDP(network string, laddr *UDPAddr)
(*UDPConn, error) { return nil, errors.New(...) }`, added to the
installation's `src/net/udpsock.go`, same revert-after treatment) got
past that too, and surfaced a third, much larger wall in
`golang.org/x/net/http2` (pulled in unconditionally by
`k8s.io/client-go/rest/request.go` -- confirmed by grepping client-go's
own source, not assumed, so this isn't avoidable by not using HTTP/2
features): over 40 distinct "undefined" / "has no field or method"
errors, e.g.:

```
.../golang.org/x/net@v0.55.0/http2/transport.go:120:8: t1.TLSClientConfig undefined (type *http.Transport has no field or method TLSClientConfig)
.../golang.org/x/net@v0.55.0/http2/server.go:184:13: undefined: tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
.../golang.org/x/net@v0.55.0/http2/server.go:213:55: undefined: tls.Conn
.../golang.org/x/net@v0.55.0/http2/transport.go:994:7: res.TLS undefined (type *http.Response has no field or method TLS)
.../golang.org/x/net@v0.55.0/http2/transport.go:2970:15: t.t1.IdleConnTimeout undefined (type *http.Transport has no field or method IdleConnTimeout)
```

TinyGo's wasm-target `http.Transport` is a bare `type Transport
struct{}` (zero fields, confirmed by reading the installed source) and
its `crypto/tls` for this target apparently has no `Conn` type at all.
`golang.org/x/net/http2`'s stub was not attempted -- it is not "add one
method" like the first two, and even closing it would not prove
anything about what comes after (see below).

**step-c** (the real `pkg/controllers.RunScheduler` +
`RunControllerManager`) was only run against the *unpatched* toolchain,
where it failed with the exact same single `net/http/roundtrip_js.go`
error as step-a/b's own unpatched-toolchain attempts -- expected, since
`pkg/controllers/scheduler.go` and `controllermanager.go` both build
their client through the same `k8s.io/client-go/rest` stack step-b
isolates. It was **not** rebuilt against the two-stub-patched toolchain
(step-b already answers what's immediately downstream of client-go's
transport layer -- the `golang.org/x/net/http2` wall -- and step-c would
only add its own scheduler/KCM-specific code *after* clearing that same
wall first, which did not happen). Stated plainly so this isn't
overclaimed: step-c's own code was never reached by either the
unpatched or the patched toolchain.

The investigation stopped at the `golang.org/x/net/http2` wall: closing
it isn't "add one stub method" like the first two, it's reimplementing
non-trivial parts of `net/http` and `crypto/tls` with no TinyGo
reference implementation for this target to copy from, and there is no
guarantee what the *next* wall would be after that (the
`reflect`-completeness concern this experiment was chartered to check
was never reached -- the code never gets past the HTTP/TLS transport
layer, well before any client-go-generated type's JSON
marshal/unmarshal or scheme registration runs). Both toolchain patches
were reverted (`git`-free `mv`-from-backup) and verified clean before
this experiment ended; nothing under `~/.local/share/mise/` or this
repository outside `spikes/s11-tinygo/` was left modified.
