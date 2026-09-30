package apiserver

import (
	"io"
	"net/http"
	"net/url"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func forwardToStore(client *kine.Client, method, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader
		if r.ContentLength != 0 {
			body = r.Body
		}
		status, out, err := client.Forward(r.Context(), method, path, nil, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(out)
	}
}

func installSnapshots(mux *http.ServeMux, client *kine.Client) {
	mux.HandleFunc("POST /internal/snapshots", adminOnly(forwardToStore(client, http.MethodPost, "/snapshot")))
	mux.HandleFunc("GET /internal/snapshots", adminOnly(forwardToStore(client, http.MethodGet, "/snapshots")))
	mux.HandleFunc("POST /internal/snapshots/restore", adminOnly(forwardToStore(client, http.MethodPost, "/snapshot/restore")))
	mux.HandleFunc("POST /internal/restore", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		status, out, err := client.Forward(r.Context(), http.MethodPost, "/restore", url.Values{"to": {r.URL.Query().Get("to")}}, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if status == http.StatusOK {
			_, _, _ = client.Forward(r.Context(), http.MethodPost, "/restore/apply", nil, nil)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(out)
	}))
}
