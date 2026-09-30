//go:build js && wasm

package main

import (
	"net/http"

	"github.com/k8flare/k8flare/packages/hookecho"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func main() {
	bridge.Serve(hookecho.Handler(&http.Client{Transport: bridge.BindingTransport{Name: "APISERVER"}}, bridge.Getenv("API_TOKEN")))
}
