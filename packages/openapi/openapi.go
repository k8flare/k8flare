package openapi

import (
	"embed"
	"net/http"
	"strings"
)

// baked holds the documents bakeopenapi rendered from SpecHandler. Serving them
// instead of building the spec at boot keeps this worker near the 10 MB floor:
// building it links the REST installer, cel-go, antlr and every group's types.
//
//go:embed baked
var baked embed.FS

// A v3 group document is also baked in the gnostic protobuf encoding kubectl
// prefers; v2 was never served that way, so it stays a 406.
const protoSuffix = "+protobuf"
const protoContentType = "application/com.github.proto-openapi.spec.v3.v1.0+protobuf"
const protoV2ContentType = "application/com.github.proto-openapi.spec.v2.v1.0+protobuf"

func Handler() (http.Handler, error) {
	m := http.NewServeMux()
	m.HandleFunc("/openapi/v2", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), protoSuffix) {
			write(w, "baked/v2.pb", protoV2ContentType)
			return
		}
		write(w, "baked/v2.json", "application/json")
	})
	m.HandleFunc("/openapi/v3", serveJSON("baked/v3.json"))
	m.HandleFunc("/openapi/v3/", func(w http.ResponseWriter, r *http.Request) {
		group := strings.TrimPrefix(r.URL.Path, "/openapi/v3/")
		if group == "" || strings.Contains(group, "..") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), protoSuffix) {
			write(w, "baked/v3/"+group+".pb", protoContentType)
			return
		}
		write(w, "baked/v3/"+group+".json", "application/json")
	})
	return m, nil
}

func serveJSON(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), protoSuffix) {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		write(w, name, "application/json")
	}
}

func write(w http.ResponseWriter, name, contentType string) {
	body, err := baked.ReadFile(name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Vary", "Accept")
	_, _ = w.Write(body)
}
