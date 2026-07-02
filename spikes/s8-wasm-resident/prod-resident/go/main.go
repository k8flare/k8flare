//go:build js && wasm

// Command prod-resident is the production re-verification build for S8's
// two highest-value open questions:
//
//	(1) how long does a ctx.WaitUntil-wrapped goroutine actually run in
//	    real production workerd once the visible response has closed
//	    (local wrangler dev showed 4+ minutes with no cap; production may
//	    enforce a much shorter limit, e.g. the historically-documented ~30s)
//	(2) does the wasm_exec.js fetch-bind patch (vendor/syumai-workers-fork)
//	    work in production for both self-loopback and external URLs
//
// Hosted inside a Durable Object (see ../src/index.ts's WasmDO) rather than
// a plain Worker fetch() handler, so state persistence doesn't depend on
// production Worker-isolate module-scope reuse (a separate, unverified
// behavior) -- DO instance lifetime is the more predictable mechanism, and
// was already confirmed locally to host this same instantiate-once pattern
// correctly (spikes/s8-wasm-resident/do-hosted/).
package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
)

var (
	instanceID = fmt.Sprintf("prod-resident-%08x", rand.Uint32())
	startedAt  = time.Now()

	requestsServed int64
	extendTicker   int64
	extendOnce     sync.Once
	extendStarted  int64 // unix millis, 0 if not started

	outboundSelfErr string
	outboundSelfOK  int64
	outboundExtErr  string
	outboundExtOK   int64
	outboundMu      sync.Mutex
)

func statusHandler(w http.ResponseWriter, r *http.Request) {
	n := atomic.AddInt64(&requestsServed, 1)
	outboundMu.Lock()
	selfErr, extErr := outboundSelfErr, outboundExtErr
	outboundMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"variant":"prod-resident","instance_id":%q,"goroutines":%d,"uptime_s":%.3f,"requests_served":%d,"extend_ticker":%d,"extend_started_ms":%d,"now_ms":%d,"outbound_self_ok":%d,"outbound_self_err":%q,"outbound_ext_ok":%d,"outbound_ext_err":%q}`,
		instanceID, runtime.NumGoroutine(), time.Since(startedAt).Seconds(), n,
		atomic.LoadInt64(&extendTicker), atomic.LoadInt64(&extendStarted), time.Now().UnixMilli(),
		atomic.LoadInt64(&outboundSelfOK), selfErr, atomic.LoadInt64(&outboundExtOK), extErr)
}

// closeThenExtendHandler answers immediately, then keeps a goroutine
// running via cloudflare.WaitUntil for up to 20 minutes (1200 ticks),
// ticking once per second. /status (dispatched to the same DO) reports how
// far it got.
func closeThenExtendHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&requestsServed, 1)
	extendOnce.Do(func() {
		atomic.StoreInt64(&extendStarted, time.Now().UnixMilli())
		cloudflare.WaitUntil(func() {
			for i := 0; i < 1200; i++ {
				time.Sleep(1 * time.Second)
				atomic.AddInt64(&extendTicker, 1)
			}
		})
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"started_at_ms":%d}`, atomic.LoadInt64(&extendStarted))
}

// outboundSelfTestHandler exercises the wasm_exec.js fetch-bind patch via a
// same-worker loopback call (scheme matches the incoming request, so this
// is https:// in production).
func outboundSelfTestHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&requestsServed, 1)
	scheme := r.URL.Scheme
	if scheme == "" {
		scheme = "https"
	}
	resp, err := http.Get(scheme + "://" + r.Host + "/status")
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		outboundMu.Lock()
		outboundSelfErr = err.Error()
		outboundMu.Unlock()
		fmt.Fprintf(w, `{"ok":false,"err":%q}`, err.Error())
		return
	}
	defer resp.Body.Close()
	atomic.AddInt64(&outboundSelfOK, 1)
	body := make([]byte, 300)
	n, _ := resp.Body.Read(body)
	fmt.Fprintf(w, `{"ok":true,"status":%d,"body":%q}`, resp.StatusCode, string(body[:n]))
}

// externalFetchTestHandler exercises the same patch against a genuine
// external URL.
func externalFetchTestHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&requestsServed, 1)
	resp, err := http.Get("https://example.com/")
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		outboundMu.Lock()
		outboundExtErr = err.Error()
		outboundMu.Unlock()
		fmt.Fprintf(w, `{"ok":false,"err":%q}`, err.Error())
		return
	}
	defer resp.Body.Close()
	atomic.AddInt64(&outboundExtOK, 1)
	body := make([]byte, 200)
	n, _ := resp.Body.Read(body)
	fmt.Fprintf(w, `{"ok":true,"status":%d,"body_prefix":%q}`, resp.StatusCode, string(body[:n]))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", statusHandler)
	mux.HandleFunc("/close-then-extend", closeThenExtendHandler)
	mux.HandleFunc("/outbound-self-test", outboundSelfTestHandler)
	mux.HandleFunc("/external-fetch-test", externalFetchTestHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "s8 prod-resident instance=%s\n", instanceID)
	})

	workers.ServeNonBlock(mux)
	workers.Ready()
	select {}
}
