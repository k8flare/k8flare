//go:build js && wasm

package main

import (
	"context"
	"net/http"
	"strconv"

	"github.com/k8flare/k8flare/pkg/apiserver"
	"github.com/k8flare/k8flare/pkg/wasmhttp"
)

func envDefault(name, def string) string {
	if v := wasmhttp.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(wasmhttp.Getenv(name)); err == nil {
		return v
	}
	return def
}

func main() {
	apiserver.WatchDialer = func(ctx context.Context, rawURL string) (<-chan []byte, func(), error) {
		ws, err := wasmhttp.DialWebSocket(ctx, "STORAGE", rawURL)
		if err != nil {
			return nil, nil, err
		}
		return ws.Messages, ws.Close, nil
	}
	handler, err := apiserver.NewHandler(apiserver.Config{
		Kine:       &http.Client{Transport: wasmhttp.BindingTransport{Name: "STORAGE"}},
		AdminToken: wasmhttp.Getenv("ADMIN_TOKEN"),
		JoinToken:  wasmhttp.Getenv("JOIN_TOKEN"),
		Kubelet: apiserver.KubeletProxy{
			Scheme: envDefault("KUBELET_SCHEME", "https"),
			Port:   envInt("KUBELET_PORT", 10250),
			Token:  wasmhttp.Getenv("ADMIN_TOKEN"),
		},
	})
	if err != nil {
		panic(err)
	}
	wasmhttp.Serve(handler)
}
