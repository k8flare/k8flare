package customresources

import (
	"encoding/json"
	"net/http"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapi/builder"
	generatedopenapi "k8s.io/apiextensions-apiserver/pkg/generated/openapi"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/handler3"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func staticOpenAPISpec() map[string]*spec.Schema {
	defs := generatedopenapi.GetOpenAPIDefinitions(func(name string) spec.Ref {
		return spec.MustCreateRef("#/components/schemas/" + common.EscapeJsonPointer(name))
	})
	out := make(map[string]*spec.Schema, len(defs))
	for name, def := range defs {
		s := def.Schema
		out[name] = &s
	}
	return out
}

type crdOpenAPI struct {
	svc       *handler3.OpenAPIService
	informer  *refillableInformer
	mu        sync.Mutex
	published map[string]struct{}
	v2        *spec.Swagger
}

func (o *crdOpenAPI) handler(fresh freshCRDs, next http.Handler) http.Handler {
	return fresh.gate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.publish()
		next.ServeHTTP(w, r)
	}))
}

func (o *crdOpenAPI) serveV2(fresh freshCRDs) http.Handler {
	return fresh.gate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.publish()
		o.mu.Lock()
		doc := o.v2
		o.mu.Unlock()
		if doc == nil {
			doc = &spec.Swagger{SwaggerProps: spec.SwaggerProps{Swagger: "2.0"}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
}

func (o *crdOpenAPI) publish() {
	want := map[string][]*spec3.OpenAPI{}
	var v2docs []*spec.Swagger
	for _, obj := range o.informer.GetIndexer().List() {
		crd := obj.(*apiextensionsv1.CustomResourceDefinition)
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}
			if doc, err := builder.BuildOpenAPIV3(crd, v.Name, builder.Options{V2: false}); err == nil {
				path := "apis/" + crd.Spec.Group + "/" + v.Name
				want[path] = append(want[path], doc)
			}
			if doc, err := builder.BuildOpenAPIV2(crd, v.Name, builder.Options{V2: true}); err == nil {
				v2docs = append(v2docs, doc)
			}
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.published == nil {
		o.published = map[string]struct{}{}
	}
	for path := range o.published {
		if _, ok := want[path]; !ok {
			o.svc.DeleteGroupVersion(path)
			delete(o.published, path)
		}
	}
	for path, specs := range want {
		merged, err := builder.MergeSpecsV3(specs...)
		if err != nil {
			continue
		}
		o.svc.UpdateGroupVersion(path, merged)
		o.published[path] = struct{}{}
	}
	base := &spec.Swagger{SwaggerProps: spec.SwaggerProps{Swagger: "2.0", Info: &spec.Info{InfoProps: spec.InfoProps{Title: "Kubernetes CRDs"}}}}
	if merged, err := builder.MergeSpecs(base, v2docs...); err == nil {
		o.v2 = merged
	}
}
