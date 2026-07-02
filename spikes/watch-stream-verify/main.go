//go:build js && wasm

// Command watch-stream-verify is a copy of the S8 spike's "prod" variant,
// redeployed under its own Worker name to isolate one question: can
// production Cloudflare Workers deliver a chunked streaming response to a
// plain Go net/http client at all? This follows up on a local-only finding
// (see the k8flare watch redesign commit and docs/platform-verification.md):
// against local wrangler dev, Go's net/http client blocks forever on the
// first Read() of a long-lived streaming response because Go requests
// "Accept-Encoding: gzip" by default and something in the local dev stack
// gzip-compresses the stream without flushing per chunk, while curl (which
// doesn't request gzip by default) reads the same bytes immediately. The
// /stream handler below is unmodified from the S8 spike; only the Worker
// name changed, so this is a clean substrate for comparing curl vs. Go
// (default / DisableCompression / explicit Accept-Encoding) against a real
// production deployment instead of local wrangler dev.
package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
	cffetch "github.com/syumai/workers/cloudflare/fetch"
)

var (
	instanceID = fmt.Sprintf("stock-%08x", rand.Uint32())
	startedAt  = time.Now()

	tickerCount  int64
	informerTick int64
)

// outboundTestHandler issues a single, simple outbound GET (to this same
// worker's own /status) to check whether Go's net/http client works at all
// under syumai/workers in this environment, independent of any streaming or
// instance-reuse concerns.
func outboundTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	url := "http://" + r.Host + "/status"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(w, `{"ok":false,"stage":"get","err":%q}`, err.Error())
		return
	}
	defer resp.Body.Close()
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	fmt.Fprintf(w, `{"ok":true,"status":%d,"body":%q}`, resp.StatusCode, string(body[:n]))
}

// cfFetchTestHandler tries the same outbound GET, but via
// github.com/syumai/workers/cloudflare/fetch (the library's own alternative
// to net/http's default js/wasm RoundTripper), to see whether that avoids
// whatever breaks in outboundTestHandler.
func cfFetchTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client := cffetch.NewClient().HTTPClient(cffetch.RedirectModeFollow)
	url := "http://" + r.Host + "/status"
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(w, `{"ok":false,"stage":"get","err":%q}`, err.Error())
		return
	}
	defer resp.Body.Close()
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	fmt.Fprintf(w, `{"ok":true,"status":%d,"body":%q}`, resp.StatusCode, string(body[:n]))
}

// bindingFetchTestHandler routes the outbound call through a real service
// binding (env.SELF, bound to this same worker in wrangler.jsonc) instead of
// js.Global(), to check whether that avoids the "Illegal invocation" panic
// hit by outboundTestHandler/cfFetchTestHandler.
func bindingFetchTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Fprintf(w, `{"ok":false,"stage":"recover","err":%q}`, fmt.Sprint(rec))
		}
	}()
	binding := cloudflare.GetBinding("SELF")
	client := cffetch.NewClient(cffetch.WithBinding(binding)).HTTPClient(cffetch.RedirectModeFollow)
	resp, err := client.Get("http://self/status")
	if err != nil {
		fmt.Fprintf(w, `{"ok":false,"stage":"get","err":%q}`, err.Error())
		return
	}
	defer resp.Body.Close()
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	fmt.Fprintf(w, `{"ok":true,"status":%d,"body":%q}`, resp.StatusCode, string(body[:n]))
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"variant":"stock","instance_id":%q,"goroutines":%d,"uptime_s":%.3f,"ticker_count":%d,"informer_tick":%d}`,
		instanceID, runtime.NumGoroutine(), time.Since(startedAt).Seconds(), atomic.LoadInt64(&tickerCount), atomic.LoadInt64(&informerTick))
}

// streamHandler keeps a chunked response open, writing a heartbeat line every 2s,
// while two independent background goroutines (a fast ticker and a slower
// "informer-like" I/O-wait loop) keep running for as long as the stream is open.
func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")

	stop := make(chan struct{})
	defer close(stop)

	// "informer-like" background goroutine: simulates a watch loop that is
	// mostly I/O-wait (blocked on a timer here, standing in for a blocked
	// network read) and only occasionally does work.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				atomic.AddInt64(&informerTick, 1)
			}
		}
	}()

	start := time.Now()
	seq := 0
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		seq++
		atomic.AddInt64(&tickerCount, 1)
		line := fmt.Sprintf("seq=%d instance=%s goroutines=%d elapsed=%s ticker_count=%d informer_tick=%d\n",
			seq, instanceID, runtime.NumGoroutine(), time.Since(start).Truncate(time.Second),
			atomic.LoadInt64(&tickerCount), atomic.LoadInt64(&informerTick))
		if _, err := fmt.Fprint(w, line); err != nil {
			// Client disconnected / stream cancelled: io.Pipe write fails
			// (e.g. io.ErrClosedPipe). ctx.Done() is NOT wired to this in
			// syumai/workers v0.32.0, so checking the Write error is the
			// only reliable way to detect cancellation.
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", statusHandler)
	mux.HandleFunc("/stream", streamHandler)
	mux.HandleFunc("/outbound-test", outboundTestHandler)
	mux.HandleFunc("/cf-fetch-test", cfFetchTestHandler)
	mux.HandleFunc("/binding-fetch-test", bindingFetchTestHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "s8 spike stock variant instance=%s\n", instanceID)
	})
	workers.Serve(mux)
}
