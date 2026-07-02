// s3-spike-hoststream: minimal long-lived chunked HTTP stream server run
// directly on the host (not in Docker), used to check whether a Container
// instance can dial out to host.docker.internal and hold a long-lived
// stream open. Throwaway spike code per docs/platform-verification.md S3.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", ":8846", "listen address")
	ticks := flag.Int("ticks", 20, "number of ticks to stream before closing")
	flag.Parse()

	http.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("host: stream client connected from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		for i := 0; i < *ticks; i++ {
			fmt.Fprintf(w, "tick %d at %s\n", i, time.Now().Format(time.RFC3339))
			if ok {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
				log.Printf("host: client disconnected after %d ticks", i)
				return
			case <-time.After(1 * time.Second):
			}
		}
		log.Printf("host: stream complete (%d ticks), closing", *ticks)
	})

	log.Printf("host: listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
