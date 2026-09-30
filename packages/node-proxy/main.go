package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	kubeconfigPath := flag.String("kubeconfig", "/var/lib/rancher/k3s/agent/kubeproxy.kubeconfig", "kubeconfig the k3s agent wrote, read only for the server URL and CA path")
	tokenFile := flag.String("token-file", "/var/run/secrets/kubernetes.io/serviceaccount/token", "service account token used to request the serving certificate")
	listen := flag.String("listen", ":6443", "address to serve the Kubernetes API on")
	flag.Parse()

	server, caPath, err := loadKubeconfig(*kubeconfigPath)
	if err != nil {
		log.Fatal(err)
	}
	var pool *x509.CertPool
	if caPath != "" {
		pem, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatal(err)
		}
		pool = x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			log.Fatalf("%s holds no certificates", caPath)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	var store certStore
	go certFetcher{target: server, pool: pool, tokenFile: *tokenFile}.keep(ctx, &store, log.Printf)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           newProxy(server, pool),
		ReadHeaderTimeout: 30 * time.Second,
		TLSConfig:         &tls.Config{GetCertificate: store.get, MinVersion: tls.VersionTLS12},
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	log.Printf("proxying %s on %s", server.Host, *listen)
	if err := srv.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
