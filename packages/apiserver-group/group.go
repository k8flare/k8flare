package group

import (
	"net/http"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	installer "github.com/k8flare/k8flare/packages/apiserver-installer"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/token/union"
)

type Config struct {
	Kine          *http.Client
	AdminToken    string
	ReadonlyToken string
	Kubelet       registry.KubeletProxy
}

func NewHandler(gv schema.GroupVersion, cfg Config) (http.Handler, error) {
	client := &kine.Client{HTTP: cfg.Kine}
	deps := registry.Deps{
		Kine:    client,
		Tokens:  union.New(auth.AdminToken(cfg.AdminToken), auth.ReadonlyToken(cfg.ReadonlyToken), auth.NodeToken{Vault: supervisor.NewVault(client)}),
		Kubelet: cfg.Kubelet,
	}
	mux := http.NewServeMux()
	installed, err := installer.Install(mux, deps, gv)
	if err != nil {
		return nil, err
	}
	var handler http.Handler = mux
	for _, wrap := range registry.Middleware {
		handler = wrap(installed.Stores)(handler)
	}
	return auth.WithRemoteUser(handler), nil
}
