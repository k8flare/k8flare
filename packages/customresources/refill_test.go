package customresources

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func establishedCRDKV(t *testing.T, name string, rev int64) kine.KV {
	crd := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}
	crd.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue}}
	data, err := json.Marshal(crd)
	if err != nil {
		t.Fatal(err)
	}
	return kine.KV{Key: crdStoragePrefix + name, Value: base64.StdEncoding.EncodeToString(data), ModRevision: rev}
}

func TestGateDoesNotDecodeAnUnchangedCRDSetAgain(t *testing.T) {
	kvs := []kine.KV{establishedCRDKV(t, "widgets.example.com", 1)}
	client := &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body, err := json.Marshal(map[string]any{"revision": 1, "kvs": kvs})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}}
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	fresh := freshCRDs{client: client, informer: registerRefillable(factory), reconciler: &reconciler{}, settled: &settledRevisions{}}
	refilled := func() string {
		rec := httptest.NewRecorder()
		fresh.gate(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/apis/example.com/v1/widgets", nil))
		return rec.Header().Get("X-CRD-Refill") + "/" + rec.Header().Get("X-CRD-Pending")
	}
	if got := refilled(); got != "1/0" {
		t.Fatalf("first request refill/pending = %s", got)
	}
	kvs[0].Value = "not a stored CRD"
	if got := refilled(); got != "0/0" {
		t.Fatalf("unchanged revisions were decoded again: refill/pending = %s", got)
	}
	if _, err := factory.Apiextensions().V1().CustomResourceDefinitions().Lister().Get("widgets.example.com"); err != nil {
		t.Fatal(err)
	}
	kvs = []kine.KV{establishedCRDKV(t, "widgets.example.com", 2)}
	if got := refilled(); got != "1/0" {
		t.Fatalf("changed revision refill/pending = %s", got)
	}
}

func TestRefillRoutesAnnotatedConversionToWorkerWithoutChangingTheStoredCRD(t *testing.T) {
	data, err := json.Marshal(webhookConvertedCRD(map[string]string{workerAnnot: "convert-echo"}))
	if err != nil {
		t.Fatal(err)
	}
	kvs := []kine.KV{{Key: crdStoragePrefix + "widgets.example.com", Value: base64.StdEncoding.EncodeToString(data), ModRevision: 1}}
	factory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	r := registerRefillable(factory)
	r.refill(kvs)
	served, err := factory.Apiextensions().V1().CustomResourceDefinitions().Lister().Get("widgets.example.com")
	if err != nil {
		t.Fatal(err)
	}
	cfg := served.Spec.Conversion.Webhook.ClientConfig
	if cfg.Service != nil || cfg.URL == nil || *cfg.URL != "https://k8flare.com/worker/convert-echo" {
		t.Fatalf("served clientConfig %+v", cfg)
	}
	for _, stored := range decodeCRDs(kvs) {
		cfg := stored.Spec.Conversion.Webhook.ClientConfig
		if cfg.URL != nil || cfg.Service == nil {
			t.Fatalf("stored clientConfig %+v", cfg)
		}
	}
}
