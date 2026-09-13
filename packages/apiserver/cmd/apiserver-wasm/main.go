//go:build js && wasm

package main

import (
	"context"
	"net/http"
	"strconv"

	"github.com/k8flare/k8flare/packages/apiserver"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func envDefault(name, def string) string {
	if v := bridge.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(bridge.Getenv(name)); err == nil {
		return v
	}
	return def
}

func main() {
	kine.WatchDialer = func(ctx context.Context, rawURL string) (<-chan []byte, func(), error) {
		ws, err := bridge.DialWebSocket(ctx, "STORAGE", rawURL)
		if err != nil {
			return nil, nil, err
		}
		return ws.Messages, ws.Close, nil
	}
	handler, err := apiserver.NewHandler(apiserver.Config{
		Kine:       &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
		AdminToken: bridge.Getenv("ADMIN_TOKEN"),
		JoinToken:  bridge.Getenv("JOIN_TOKEN"),
		Kubelet: registry.KubeletProxy{
			Scheme: envDefault("KUBELET_SCHEME", "https"),
			Port:   envInt("KUBELET_PORT", 10250),
			Token:  bridge.Getenv("ADMIN_TOKEN"),
		},
	})
	if err != nil {
		panic(err)
	}
	bridge.Serve(handler)
}
