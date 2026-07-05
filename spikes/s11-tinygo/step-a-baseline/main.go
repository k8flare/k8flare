//go:build js && wasm

// Command step-a-baseline is throwaway TinyGo-viability scaffolding for
// the S11 spike -- see ../README.md. Not a real Worker entrypoint.
//
// Minimal syumai/workers Worker: no k8s.io imports at all. This exists to
// establish a TinyGo + syumai/workers baseline before adding client-go.
package main

import (
	"net/http"

	"github.com/syumai/workers"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	workers.Serve(mux)
}
