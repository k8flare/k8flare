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
	"k8s.io/client-go/tools/cache"
)

func crdKV(t *testing.T, name string, rev int64) kine.KV {
	data, err := json.Marshal(&apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}})
	if err != nil {
		t.Fatal(err)
	}
	return kine.KV{Key: crdStoragePrefix + name, Value: base64.StdEncoding.EncodeToString(data), ModRevision: rev}
}

func TestRefillDeliversChangesToHandlers(t *testing.T) {
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	r := registerRefillable(factory)
	crds := factory.Apiextensions().V1().CustomResourceDefinitions()
	var added, updated, deleted int
	crds.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { added++ },
		UpdateFunc: func(any, any) { updated++ },
		DeleteFunc: func(any) { deleted++ },
	})
	if n := r.refill([]kine.KV{crdKV(t, "a.example.com", 1), crdKV(t, "b.example.com", 2)}); n != 2 || added != 2 {
		t.Fatalf("first refill changed=%d added=%d", n, added)
	}
	if n := r.refill([]kine.KV{crdKV(t, "a.example.com", 1), crdKV(t, "b.example.com", 2)}); n != 0 {
		t.Fatalf("unchanged refill changed=%d", n)
	}
	if n := r.refill([]kine.KV{crdKV(t, "a.example.com", 3)}); n != 2 || updated != 1 || deleted != 1 {
		t.Fatalf("third refill changed=%d updated=%d deleted=%d", n, updated, deleted)
	}
	got, err := crds.Lister().Get("a.example.com")
	if err != nil || got.ResourceVersion != "3" {
		t.Fatalf("lister a = %v %v", got, err)
	}
}
