//go:build js && wasm

package group

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		table, err := bridge.CallBytes(ctx, "PRINTERS", "convertToTable", group, object)
		if err == nil && len(table) == 0 {
			return nil, registry.ErrNoTableSource
		}
		return table, err
	}
	registry.Poke = func(ctx context.Context) {
		if bridge.HasBinding(ctx, "SCHEDULER") {
			if _, err := bridge.Call(ctx, "SCHEDULER", "poke"); err != nil {
				println("scheduler poke:", err.Error())
			}
		}
	}
	registry.PokeControllers = func(ctx context.Context) {
		if bridge.HasBinding(ctx, "CONTROLLERS") {
			if _, err := bridge.Call(ctx, "CONTROLLERS", "poke"); err != nil {
				println("controllers poke:", err.Error())
			}
		}
	}
	registry.WakeControllers = func(ctx context.Context, delay time.Duration) {
		body := strings.NewReader(fmt.Sprintf(`{"target":"controllers","delayMs":%d}`, delay.Milliseconds()))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://cluster.internal/wake", body)
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}}).Do(req)
		if err != nil {
			println("controllers wake:", err.Error())
			return
		}
		resp.Body.Close()
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
