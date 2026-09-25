//go:build js && wasm

package main

import (
	"context"
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"github.com/k8flare/k8flare/packages/customresources"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func main() {
	kine.WatchDialer = func(ctx context.Context, rawURL string) (<-chan []byte, func(), error) {
		ws, err := bridge.DialWebSocket(ctx, "STORAGE", rawURL)
		if err != nil {
			return nil, nil, err
		}
		return ws.Messages, ws.Close, nil
	}
	handler, err := customresources.NewHandler(customresources.Config{
		Kine:      &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
		Admission: &http.Client{Transport: bridge.BindingTransport{Name: "ADMISSION"}},
		Tunnel:    &http.Client{Transport: bridge.BindingTransport{Name: "TUNNEL"}},
		Outbound:  &http.Client{Transport: bridge.BindingTransport{Name: "OUTBOUND"}},
		Hooks:     &http.Client{Transport: bridge.BindingTransport{Name: "HOOKS"}},
	})
	if err != nil {
		panic(err)
	}
	bridge.Serve(handler)
}
