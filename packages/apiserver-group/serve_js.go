//go:build js && wasm

package group

import (
	"context"
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Serve(groupVersion string) {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		panic(err)
	}
	kine.WatchDialer = func(ctx context.Context, rawURL string) (<-chan []byte, func(), error) {
		ws, err := bridge.DialWebSocket(ctx, "STORAGE", rawURL)
		if err != nil {
			return nil, nil, err
		}
		return ws.Messages, ws.Close, nil
	}
	registry.TableSource = func(ctx context.Context, group string, object []byte) ([]byte, error) {
		if !bridge.HasBinding(ctx, "PRINTERS") {
			return nil, registry.ErrNoTableSource
		}
		return bridge.CallBytes(ctx, "PRINTERS", "convertToTable", group, object)
	}
	registry.Poke = func(ctx context.Context) {
		if bridge.HasBinding(ctx, "SCHEDULER") {
			if _, err := bridge.Call(ctx, "SCHEDULER", "poke"); err != nil {
				println("scheduler poke:", err.Error())
			}
		}
	}
	kubelet := registry.KubeletProxy{Transport: bridge.BindingTransport{Name: "TUNNEL"}, Base: "https://nodetunnel.internal"}
	handler, err := NewHandler(gv, Config{
		Kine:          &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
		AdminToken:    bridge.Getenv("ADMIN_TOKEN"),
		ReadonlyToken: bridge.Getenv("READONLY_TOKEN"),
		Kubelet:       kubelet,
	})
	if err != nil {
		panic(err)
	}
	bridge.Serve(handler)
}
