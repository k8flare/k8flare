//go:build js && wasm

package main

import (
	"net/http"

	"github.com/k8flare/k8flare/packages/admission"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func main() {
	secrets, err := kine.ParseSecretKeys(bridge.Getenv("SECRETS_ENCRYPTION_KEYS"))
	if err != nil {
		panic(err)
	}
	handler := admission.NewHandler(admission.Config{
		Kine:     &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
		Tunnel:   &http.Client{Transport: bridge.BindingTransport{Name: "TUNNEL"}},
		Hooks:    &http.Client{Transport: bridge.BindingTransport{Name: "HOOKS"}},
		Outbound: &http.Client{Transport: bridge.BindingTransport{Name: "OUTBOUND"}},
		API:      &http.Client{Transport: bridge.BindingTransport{Name: "APISERVER"}},
		Token:    bridge.Getenv("API_TOKEN"),
		Secrets:  secrets,
	})
	bridge.Serve(handler)
}
