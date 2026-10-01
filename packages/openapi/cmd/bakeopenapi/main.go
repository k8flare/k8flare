//go:build !js

// bakeopenapi renders the OpenAPI documents the openapi worker used to build at
// boot and writes them next to the package. Building the spec needs the whole
// REST installer -- cel-go, antlr, k8s.io/apiserver, every group's types -- and
// the result is identical on every boot, so the worker embeds these instead and
// stays near the 10 MB floor rather than 102% of the Loader cap.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/k8flare/k8flare/packages/openapi"
)

func main() {
	out := "packages/openapi/baked"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	handler, err := openapi.SpecHandler()
	if err != nil {
		fail(err)
	}
	if err := os.RemoveAll(out); err != nil {
		fail(err)
	}
	total := 0
	write := func(path, name, accept string) []byte {
		body, status, _ := get(handler, path, accept)
		if status != http.StatusOK {
			fail(fmt.Errorf("%s: status %d", path, status))
		}
		dst := filepath.Join(out, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			fail(err)
		}
		total += len(body)
		return body
	}
	if err := writeGroupDefinitions(write("/openapi/v2", "v2.json", "application/json")); err != nil {
		fail(err)
	}
	write("/openapi/v2", "v2.pb", protoV2Accept)
	root := write("/openapi/v3", "v3.json", "application/json")
	var discovery struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(root, &discovery); err != nil {
		fail(err)
	}
	for group := range discovery.Paths {
		write("/openapi/v3/"+group, filepath.Join("v3", group+".json"), "application/json")
		// kubectl asks for the gnostic protobuf encoding of each v3 group, so bake
		// that too rather than leaving the worker to convert and link gnostic.
		write("/openapi/v3/"+group, filepath.Join("v3", group+".pb"), protoV3Accept)
	}
	fmt.Printf("bakeopenapi: %d documents, %d bytes -> %s\n", len(discovery.Paths)+2, total, out)
}

// protoV3Accept is the encoding kubectl prefers for a v3 group document.
const protoV3Accept = "application/com.github.proto-openapi.spec.v3.v1.0+protobuf"
const protoV2Accept = "application/com.github.proto-openapi.spec.v2@v1.0+protobuf"

func get(h http.Handler, path, accept string) ([]byte, int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", accept)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.Bytes(), rec.Code, rec.Header().Get("Content-Type")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bakeopenapi:", strings.TrimSpace(err.Error()))
	os.Exit(1)
}
