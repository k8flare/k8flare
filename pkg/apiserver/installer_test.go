package apiserver

import (
	"strings"
	"testing"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// The hand-written router (handler.go's HandleResource, plus apidef.Table's
// verb and subresource columns) reimplements what k8s.io/apiserver's own
// installer produces from the same genericregistry.Store instances. This
// pins that the real installer accepts every resource this project serves --
// the precondition for deleting the reimplementation
// (docs/real-apiserver-plan.md).
func TestTheRealInstallerAcceptsEveryResourceWeServe(t *testing.T) {
	storage := newTestStorage(newFakeKV())
	storesByGV := map[schema.GroupVersion]map[string]*ResourceStore{}
	for _, gv := range apidef.GroupVersions() {
		storesByGV[gv] = NewResourceStoresForGroupVersion(storage, gv)
	}

	handler, err := NewRESTContainer(storesByGV)
	if err != nil {
		t.Fatalf("NewRESTContainer: %v", err)
	}
	container, ok := handler.(*restful.Container)
	if !ok {
		t.Fatalf("NewRESTContainer returned %T, want *restful.Container", handler)
	}

	routes := map[string]bool{}
	for _, ws := range container.RegisteredWebServices() {
		for _, r := range ws.Routes() {
			routes[r.Method+" "+r.Path] = true
		}
	}
	if len(routes) == 0 {
		t.Fatal("the installer registered no routes at all")
	}

	// Spot-check one resource per shape rather than every route: a
	// namespaced one, a cluster-scoped one, and the watch path that
	// handler.go serves from the shell Worker today.
	for _, want := range []string{
		"GET /api/v1/namespaces/{namespace}/pods",
		"POST /api/v1/namespaces/{namespace}/pods",
		"GET /api/v1/namespaces/{namespace}/pods/{name}",
		"PUT /api/v1/namespaces/{namespace}/pods/{name}",
		"DELETE /api/v1/namespaces/{namespace}/pods/{name}",
		"GET /api/v1/pods",
		"GET /api/v1/namespaces/{name}",
		"GET /apis/apps/v1/namespaces/{namespace}/deployments",
	} {
		if !routes[want] {
			t.Errorf("the installer did not register %q", want)
		}
	}

	t.Logf("installed %d routes across %d group-versions", len(routes), len(apidef.GroupVersions()))
	var sample []string
	for r := range routes {
		if strings.Contains(r, "/pods") {
			sample = append(sample, r)
		}
	}
	t.Logf("pods routes: %v", sample)
}
