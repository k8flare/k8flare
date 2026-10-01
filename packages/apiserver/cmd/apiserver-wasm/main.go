//go:build js && wasm

package main

import (
	"net/http"
	"strconv"

	"github.com/k8flare/k8flare/packages/apiserver"
	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	"github.com/k8flare/k8flare/packages/edgehost"
	bridge "github.com/k8flare/k8flare/packages/worker-bridge"
)

func inflightLimit(name string, fallback int) int {
	raw := bridge.Getenv(name)
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		panic(name + " must be a non-negative integer")
	}
	return limit
}

func main() {
	edgehost.SetClusterDomain(bridge.Getenv("CLUSTER_DOMAIN"))
	edgehost.AddAPIHosts(bridge.Getenv("API_HOSTS"))
	edgehost.SetDisabled(bridge.Getenv("DISABLE"))
	requiredClaims, err := auth.ParseRequiredClaims(bridge.Getenv("OIDC_REQUIRED_CLAIMS"))
	if err != nil {
		panic(err)
	}
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
		OIDC: auth.OIDC{
			IssuerURL:      bridge.Getenv("OIDC_ISSUER_URL"),
			ClientID:       bridge.Getenv("OIDC_CLIENT_ID"),
			UsernameClaim:  bridge.Getenv("OIDC_USERNAME_CLAIM"),
			UsernamePrefix: bridge.Getenv("OIDC_USERNAME_PREFIX"),
			GroupsClaim:    bridge.Getenv("OIDC_GROUPS_CLAIM"),
			GroupsPrefix:   bridge.Getenv("OIDC_GROUPS_PREFIX"),
			RequiredClaims: requiredClaims,
		},
		MaxRequestsInflight:         inflightLimit("MAX_REQUESTS_INFLIGHT", apiserver.DefaultMaxRequestsInflight),
		MaxMutatingRequestsInflight: inflightLimit("MAX_MUTATING_REQUESTS_INFLIGHT", apiserver.DefaultMaxMutatingRequestsInflight),
		ClusterUID:                  bridge.Getenv("CLUSTER_UID"),
		AuditPolicy:                 bridge.Getenv("AUDIT_POLICY"),

		SecretsEncryptionKeys: bridge.Getenv("SECRETS_ENCRYPTION_KEYS"),
	})
	if err != nil {
		panic(err)
	}
	bridge.Serve(handler)
}
