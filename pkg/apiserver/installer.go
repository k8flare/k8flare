package apiserver

import (
	"fmt"
	"net/http"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// InstallRESTForGroupVersion builds the routes for one GroupVersion with
// k8s.io/apiserver's own API installer, over the same genericregistry.Store
// instances the hand-written handler already serves from.
//
// handler.go's HandleResource and apidef.Table's verb/subresource columns
// are a reimplementation of what this one call produces. Verified by running
// it under GOOS=js: InstallREST registers the full REST surface for a
// resource -- collection GET/POST/DELETE, object GET/PUT/PATCH/DELETE, and
// both watch paths (docs/real-apiserver-plan.md).
//
// A Worker has no listener, which does not matter: the installer fills a
// restful.Container, and a Container is an http.Handler that the fetch
// bootstrap can call ServeHTTP on directly.
func InstallRESTForGroupVersion(container *restful.Container, gv schema.GroupVersion, stores map[string]*ResourceStore, admit admission.Interface) error {
	storage := make(map[string]rest.Storage, len(stores))
	for resource, s := range stores {
		if s.upstream == nil {
			return fmt.Errorf("installer: %s %q has no upstream store", gv, resource)
		}
		storage[resource] = s.upstream
	}
	if len(storage) == 0 {
		return nil
	}

	// This project's own Scheme (scheme.go), not client-go's: apidef.Table
	// is the single registration path, and it carries k8flare's own
	// k8flare.com/v1alpha1 types which client-go's scheme has never heard
	// of. Using client-go's here fails to register the Cluster resource.
	s := Scheme
	served := make(map[string][]string, len(storage))
	for resource := range storage {
		if gv.Group == "" {
			served[resource] = []string{gv.Version}
		} else {
			served[resource] = []string{gv.Group + "/" + gv.Version}
		}
	}
	group := &endpoints.APIGroupVersion{
		Storage:                     storage,
		Root:                        apidef.APIRoot(gv),
		GroupVersion:                gv,
		MetaGroupVersion:            &metav1.SchemeGroupVersion,
		AllServedVersionsByResource: served,
		// The scheme is the single source for all three: k8flare serves
		// external versions only, so no internal-version conversion round
		// trip is configured.
		Creater:               s,
		Convertor:             s,
		Typer:                 s,
		Defaulter:             s,
		ConvertabilityChecker: s,
		UnsafeConvertor:       runtime.UnsafeObjectConvertor(s),
		// The accessor upstream uses. Without it every create panics in
		// ContextBasedNaming.ObjectName before it reaches storage.
		Namer: runtime.Namer(meta.NewAccessor()),

		Serializer:     serializer.NewCodecFactory(s),
		ParameterCodec: runtime.NewParameterCodec(s),

		EquivalentResourceRegistry: runtime.NewEquivalentResourceRegistry(),
		// Without this the field manager refuses to build and every
		// resource fails to register. Deduced rather than schema-backed:
		// k8flare serves the OpenAPI documents as static assets and has no
		// runtime schema to derive typed field sets from.
		TypeConverter: managedfields.NewDeducedTypeConverter(),

		Admit: admit,
	}

	if _, _, err := group.InstallREST(container); err != nil {
		return fmt.Errorf("installer: %s: %w", gv, err)
	}
	return nil
}

// NewRESTContainer installs every GroupVersion in apidef.Table and returns
// the container as a plain http.Handler.
func NewRESTContainer(storesByGV map[schema.GroupVersion]map[string]*ResourceStore, admit admission.Interface) (http.Handler, error) {
	container := restful.NewContainer()
	for _, gv := range apidef.GroupVersions() {
		if err := InstallRESTForGroupVersion(container, gv, storesByGV[gv], admit); err != nil {
			return nil, err
		}
	}
	return container, nil
}
