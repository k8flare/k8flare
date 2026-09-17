package crdreconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type memKine map[string]int64

func (m memKine) RoundTrip(req *http.Request) (*http.Response, error) {
	out := map[string]any{}
	switch {
	case req.URL.Path == "/list":
		prefix := req.URL.Query().Get("prefix")
		var keys []string
		for k := range m {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		kvs := []kine.KV{}
		for _, k := range keys {
			kvs = append(kvs, kine.KV{Key: k, ModRevision: m[k]})
		}
		out["kvs"] = kvs
	case req.URL.Path == "/kv" && req.Method == http.MethodDelete:
		var body struct{ Key string }
		json.NewDecoder(req.Body).Decode(&body)
		delete(m, body.Key)
	}
	data, _ := json.Marshal(out)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
}

func TestFinalizeDeletesInstancesAndRemovesFinalizer(t *testing.T) {
	now := metav1.Now()
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com", DeletionTimestamp: &now, Finalizers: []string{apiextensionsv1.CustomResourceCleanupFinalizer}},
		Spec:       apiextensionsv1.CustomResourceDefinitionSpec{Group: "example.com", Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "foos"}},
	}
	client := fake.NewSimpleClientset(crd)
	store := memKine{"/registry/example.com/foos/ns/a": 1, "/registry/example.com/foos/ns/b": 2, "/registry/example.com/bars/ns/c": 3}
	d := Deps{Client: client, Kine: &kine.Client{HTTP: &http.Client{Transport: store}}}
	if !NeedsFinalize(crd) {
		t.Fatal("expected finalize work")
	}
	if err := Finalize(context.Background(), d, crd); err != nil {
		t.Fatal(err)
	}
	if len(store) != 1 {
		t.Fatalf("remaining keys = %v", store)
	}
	got, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), crd.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if apihelpers.CRDHasFinalizer(got, apiextensionsv1.CustomResourceCleanupFinalizer) {
		t.Fatal("finalizer still present")
	}
	if !apihelpers.IsCRDConditionTrue(got, apiextensionsv1.Terminating) {
		t.Fatal("terminating condition not set")
	}
}
