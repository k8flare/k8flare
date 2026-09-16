//go:build js && wasm

package main

import (
	"context"
	"net/http"
	"time"

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
	kine.Resident = bridge.Holding
	handler, err := customresources.NewHandler(customresources.Config{
		Kine: &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
	})
	if err != nil {
		panic(err)
	}
	bridge.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridge.Poked()
		if r.URL.Path != "/hold" {
			handler.ServeHTTP(w, r)
			if r.Method != http.MethodGet && bridge.HasBinding(r.Context(), "CONTROLLERS") {
				if _, err := bridge.Call(r.Context(), "CONTROLLERS", "poke"); err != nil {
					println("customresources: controllers poke:", err.Error())
				}
			}
			return
		}
		bridge.OpenWindow(r.Context())
		defer bridge.CloseWindow(r.Context())
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		bridge.Hold(r.Context(), bridge.ParseHold(r, holdWindow), bridge.Quiet)
		bridge.WriteNext(w, 0)
	}))
}

const holdWindow = 15 * time.Second
