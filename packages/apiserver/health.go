package apiserver

import (
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/server/healthz"
)

func installHealth(mux *http.ServeMux, client *kine.Client) {
	datastore := healthz.NamedCheck("datastore", func(r *http.Request) error {
		_, err := client.Revision(r.Context())
		return err
	})
	healthz.InstallLivezHandler(mux)
	healthz.InstallReadyzHandler(mux, healthz.PingHealthz, datastore)
	healthz.InstallHandler(mux, healthz.PingHealthz, datastore)
}
