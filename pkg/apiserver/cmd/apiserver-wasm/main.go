//go:build js && wasm

package main

import (
	"net/http"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/wasmhttp"
)

func main() {
	handler, err := apiserver.NewHandler(apiserver.Config{
		Kine:       &http.Client{Transport: wasmhttp.BindingTransport{Name: "STORAGE"}},
		AdminToken: wasmhttp.Getenv("ADMIN_TOKEN"),
	})
	if err != nil {
		panic(err)
	}
	wasmhttp.Serve(handler)
}
