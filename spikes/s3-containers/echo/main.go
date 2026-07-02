// s3-spike-echo: minimal HTTP server used only to probe Cloudflare Containers
// local dev lifecycle (wrangler dev + @cloudflare/containers). Not part of
// k8flare; throwaway spike code per docs/platform-verification.md S3.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	startTime = time.Now()
	hostname  string
	hits      int
)

func main() {
	hostname, _ = os.Hostname()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("s3-spike-echo: starting pid=%d hostname=%s port=%s", os.Getpid(), hostname, port)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hits++
		log.Printf("request: %s %s (hit #%d)", r.Method, r.URL.Path, hits)
		fmt.Fprintf(w, "s3-spike-echo hostname=%s pid=%d uptime=%s hits=%d\n", hostname, os.Getpid(), time.Since(startTime).Round(time.Millisecond), hits)
	})

	// /dial-host: outbound-dial probe for item 6 (container -> host long-lived
	// stream). Container makes an HTTP GET to targetURL (expected to be a
	// host-side long-lived chunked stream reachable via host.docker.internal)
	// and relays each received line back to the original caller, flushing as
	// it goes, so the *caller's* connection to the container is itself held
	// open for the duration -- proving both legs of the streaming path work
	// through Docker's networking layer that wrangler dev's Containers support
	// uses locally.
	mux.HandleFunc("/dial-host", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("url")
		if target == "" {
			target = "http://host.docker.internal:8846/stream"
		}
		log.Printf("dial-host: dialing %s", target)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(target)
		if err != nil {
			fmt.Fprintf(w, "container: dial error: %v\n", err)
			log.Printf("dial-host: error dialing %s: %v", target, err)
			return
		}
		defer resp.Body.Close()
		fmt.Fprintf(w, "container: connected to %s (status=%s)\n", target, resp.Status)
		if flusher != nil {
			flusher.Flush()
		}

		buf := make([]byte, 256)
		total := 0
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				total += n
				fmt.Fprintf(w, "container: received %q\n", string(buf[:n]))
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err == io.EOF {
				fmt.Fprintf(w, "container: host stream closed (EOF), total_bytes=%d\n", total)
				log.Printf("dial-host: host stream closed, total_bytes=%d", total)
				break
			}
			if err != nil {
				fmt.Fprintf(w, "container: read error: %v\n", err)
				log.Printf("dial-host: read error: %v", err)
				break
			}
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok\n")
	})

	log.Fatal(http.ListenAndServe(":"+port, mux))
}
