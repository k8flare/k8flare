// Command demo is k8flare's v1 allowlisted Pod image for workers/nodes
// (Pod-on-Containers). It is deliberately minimal: k8flare's Containers
// backend cannot run an arbitrary user-supplied image (see
// spikes/s3-containers/FINDINGS.md item 4 -- container image is
// deploy-time-fixed, not chosen at runtime), so a Pod's
// spec.containers[].image must name one of a small, wrangler.jsonc-declared
// set of pre-built images. This is that set's only member for now -- a
// static HTTP server that reports its own identity, useful for smoke-testing
// that a Pod actually reached Running and is reachable, without needing to
// trust or execute anything the caller supplies. See workers/nodes/README.md
// for how to add more images to the allowlist.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var startTime = time.Now()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	hostname, _ := os.Hostname()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "k8flare-demo hostname=%s pid=%d uptime=%s\n", hostname, os.Getpid(), time.Since(startTime).Round(time.Second))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok\n")
	})

	log.Printf("k8flare-demo: listening on :%s (hostname=%s pid=%d)", port, hostname, os.Getpid())
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
