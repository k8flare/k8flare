//go:build js && wasm

package main

import (
	"net/http"

	"github.com/k8flare/k8flare/packages/apiserver"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func main() {
	handler, err := apiserver.NewHandler(apiserver.Config{
		Kine:               &http.Client{Transport: bridge.BindingTransport{Name: "STORAGE"}},
		AdminToken:         bridge.Getenv("ADMIN_TOKEN"),
		ReadonlyToken:      bridge.Getenv("READONLY_TOKEN"),
		JoinToken:          bridge.Getenv("JOIN_TOKEN"),
		Groups:             &http.Client{Transport: bridge.BindingTransport{Name: "APIGROUPS"}},
		OpenAPI:            &http.Client{Transport: bridge.BindingTransport{Name: "OPENAPI"}},
		CustomResources:    &http.Client{Transport: bridge.BindingTransport{Name: "CUSTOMRESOURCES"}},
		Outbound:           &http.Client{Transport: bridge.BindingTransport{Name: "OUTBOUND"}},
		Tunnel:             &http.Client{Transport: bridge.BindingTransport{Name: "TUNNEL"}},
		Admission:          &http.Client{Transport: bridge.BindingTransport{Name: "ADMISSION"}},
		Hooks:              &http.Client{Transport: bridge.BindingTransport{Name: "HOOKS"}},
		AccessTeam:         bridge.Getenv("ACCESS_TEAM_DOMAIN"),
		AccessAUD:          bridge.Getenv("ACCESS_AUD"),
		AccessGroupsClaim:  bridge.Getenv("ACCESS_GROUPS_CLAIM"),
		AccessGroupsPrefix: bridge.Getenv("ACCESS_GROUPS_PREFIX"),
		ClusterUID:         bridge.Getenv("CLUSTER_UID"),

		SecretsEncryptionKeys: bridge.Getenv("SECRETS_ENCRYPTION_KEYS"),
	})
	if err != nil {
		panic(err)
	}
	bridge.Serve(handler)
}
