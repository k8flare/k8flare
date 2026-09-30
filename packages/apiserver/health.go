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
	queues := healthz.NamedCheck("queues", func(r *http.Request) error {
		h, err := client.Health(r.Context())
		if err != nil {
			return err
		}
		return h.Queues()
	})
	controllers := healthz.NamedCheck("controllers", func(r *http.Request) error {
		h, err := client.Health(r.Context())
		if err != nil {
			return err
		}
		return h.Controllers(nil)
	})
	healthz.InstallLivezHandler(mux)
	healthz.InstallReadyzHandler(mux, healthz.PingHealthz, datastore, queues, controllers)
	healthz.InstallHandler(mux, healthz.PingHealthz, datastore, queues, controllers)
}
