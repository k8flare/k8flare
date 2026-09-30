package group

import (
	"net/http"
	"sync/atomic"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	installer "github.com/k8flare/k8flare/packages/apiserver-installer"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/token/union"
	"k8s.io/apiserver/pkg/endpoints/filters"
)

type Config struct {
	Kine          *http.Client
	AdminToken    string
	ReadonlyToken string
	Kubelet       registry.KubeletProxy
	Admission     *http.Client
}

func NewHandler(gv schema.GroupVersion, cfg Config) (http.Handler, error) {
	client := &kine.Client{HTTP: cfg.Kine}
	vault := supervisor.NewVault(client)
	deps := registry.Deps{
		Kine:      client,
		Tokens:    union.New(auth.AdminToken(cfg.AdminToken), auth.ReadonlyToken(cfg.ReadonlyToken), auth.VaultToken{Vault: vault}, auth.NodeToken{Vault: vault}, auth.ServiceAccountToken{HMAC: []byte(cfg.AdminToken), Objects: auth.KineObjects{Client: client}}),
		Kubelet:   cfg.Kubelet,
		Admission: cfg.Admission,
		TokenHMAC: []byte(cfg.AdminToken),
	}
	mux := http.NewServeMux()
	installed, err := installer.Install(mux, deps, servedFor(gv)...)
	if err != nil {
		return nil, err
	}
	handler := registry.NamespaceLifecycle(client)(mux)
	for _, wrap := range registry.Middleware {
		handler = wrap(installed.Stores)(handler)
	}
	handler = auth.WithRemoteUser(handler)
	var ready atomic.Bool
	return filters.WithRequestReceivedTimestamp(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			if err := auth.InstallServiceAccountKey(r.Context(), client, []byte(cfg.AdminToken)); err == nil {
				ready.Store(true)
			}
		}
		handler.ServeHTTP(w, r)
	})), nil
}

func servedFor(gv schema.GroupVersion) []schema.GroupVersion {
	var out []schema.GroupVersion
	for _, s := range registry.Served {
		if s.GV.Group == gv.Group {
			out = append(out, s.GV)
		}
	}
	if len(out) == 0 {
		return []schema.GroupVersion{gv}
	}
	return out
}
