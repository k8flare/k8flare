package customresources

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kube-openapi/pkg/handler3"
)

func TestStaticOpenAPISpecIncludesObjectMeta(t *testing.T) {
	spec := staticOpenAPISpec()
	if spec[metav1.ObjectMeta{}.OpenAPIModelName()] == nil {
		t.Fatal("missing ObjectMeta")
	}
}

func TestPublishOpenAPIListsEstablishedCRD(t *testing.T) {
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	r := registerRefillable(factory)
	_ = factory.Apiextensions().V1().CustomResourceDefinitions()
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "crontabs.stable.example.com"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "stable.example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "crontabs", Singular: "crontab", Kind: "CronTab", ListKind: "CronTabList"},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: "v1", Served: true, Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {Type: "object", Properties: map[string]apiextensionsv1.JSONSchemaProps{"replicas": {Type: "integer"}}},
						},
					}},
				},
				{
					Name: "v2", Served: true, Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {Type: "object", Properties: map[string]apiextensionsv1.JSONSchemaProps{"replicas": {Type: "integer"}}},
						},
					}},
				},
			},
		},
	}
	data, err := json.Marshal(crd)
	if err != nil {
		t.Fatal(err)
	}
	r.refill([]kine.KV{{Key: crdStoragePrefix + crd.Name, Value: base64.StdEncoding.EncodeToString(data), ModRevision: 1}})
	pub := &crdOpenAPI{svc: handler3.NewOpenAPIService(), informer: r}
	pub.publish()
	if _, ok := pub.published["apis/stable.example.com/v1"]; !ok {
		t.Fatalf("published=%v", pub.published)
	}
	if _, ok := pub.published["apis/stable.example.com/v2"]; !ok {
		t.Fatalf("missing v2 published=%v", pub.published)
	}
	if pub.v2 == nil {
		t.Fatal("v2 swagger is nil")
	}
	if _, ok := pub.v2.Definitions["com.example.stable.v1.CronTab"]; !ok {
		t.Fatalf("v2 definitions=%v", keysOf(pub.v2.Definitions))
	}
	if _, ok := pub.v2.Definitions["com.example.stable.v2.CronTab"]; !ok {
		t.Fatalf("v2 missing v2 kind definitions=%v", keysOf(pub.v2.Definitions))
	}
	r.refill(nil)
	pub.publish()
	if len(pub.published) != 0 {
		t.Fatalf("stale published=%v", pub.published)
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
