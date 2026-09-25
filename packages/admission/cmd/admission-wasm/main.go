//go:build js && wasm

package main

import (
	"net/http"

	"github.com/k8flare/k8flare/packages/admission"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func main() {
	handler := admission.NewHandler(admission.Config{
		Kine:     &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
		Tunnel:   &http.Client{Transport: bridge.BindingTransport{Name: "TUNNEL"}},
		Hooks:    &http.Client{Transport: bridge.BindingTransport{Name: "HOOKS"}},
		Outbound: &http.Client{Transport: bridge.BindingTransport{Name: "OUTBOUND"}},
		API:      &http.Client{Transport: bridge.BindingTransport{Name: "APISERVER"}},
		Token:    bridge.Getenv("ADMIN_TOKEN"),
	})
	bridge.Serve(handler)
}
