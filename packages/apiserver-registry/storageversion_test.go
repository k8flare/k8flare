package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/apiserver/pkg/storage"
)

type fakeKine struct {
	rev  int64
	data map[string][]byte
	revs map[string]int64
}

func (f *fakeKine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, body map[string]any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /kv":
		key := r.URL.Query().Get("key")
		v, ok := f.data[key]
		if !ok {
			reply(http.StatusNotFound, map[string]any{"revision": f.rev})
			return
		}
		reply(http.StatusOK, map[string]any{"revision": f.rev, "kv": kine.KV{Key: key, Value: base64.StdEncoding.EncodeToString(v), ModRevision: f.revs[key]}})
	case "PUT /kv":
		var body struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Revision int64  `json:"revision"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Revision != f.revs[body.Key] {
			reply(http.StatusConflict, map[string]any{"revision": f.rev})
			return
		}
		f.rev++
		f.data[body.Key], _ = base64.StdEncoding.DecodeString(body.Value)
		f.revs[body.Key] = f.rev
		reply(http.StatusOK, map[string]any{"revision": f.rev})
	case "GET /list":
		var kvs []kine.KV
		for key, v := range f.data {
			if strings.HasPrefix(key, r.URL.Query().Get("prefix")) && key >= r.URL.Query().Get("from") {
				kvs = append(kvs, kine.KV{Key: key, Value: base64.StdEncoding.EncodeToString(v), ModRevision: f.revs[key]})
			}
		}
		sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
		reply(http.StatusOK, map[string]any{"revision": f.rev, "kvs": kvs})
	default:
		reply(http.StatusNotFound, map[string]any{})
	}
}

type toServer struct{ base string }

func (t toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	next, err := http.NewRequestWithContext(req.Context(), req.Method, t.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.Header = req.Header
	return http.DefaultTransport.RoundTrip(next)
}

func newFakeKine(t *testing.T) (*fakeKine, *kine.Client) {
	f := &fakeKine{data: map[string][]byte{}, revs: map[string]int64{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, &kine.Client{HTTP: &http.Client{Transport: toServer{base: srv.URL}}}
}

func (f *fakeKine) seed(key, value string) {
	f.rev++
	f.data[key] = []byte(value)
	f.revs[key] = f.rev
}

func storedResource(t *testing.T, name string) kine.StoredResource {
	t.Helper()
	resources, err := StoredResources()
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range resources {
		if res.Name == name {
			return res
		}
	}
	t.Fatalf("%s is not a stored resource", name)
	return kine.StoredResource{}
}

func TestStoredResourcesRecordTheStorageVersionOfEveryKey(t *testing.T) {
	resources, err := StoredResources()
	if err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{}
	for _, res := range resources {
		if _, dup := hashes[res.Name]; dup {
			t.Fatalf("%s listed twice", res.Name)
		}
		hashes[res.Name] = res.Hash
	}
	for name, want := range map[string]schema.GroupVersionKind{
		"events":                   {Version: "v1", Kind: "Event"},
		"horizontalpodautoscalers": {Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"},
		"deployments":              {Group: "apps", Version: "v1", Kind: "Deployment"},
	} {
		if hashes[name] != discovery.StorageVersionHash(want.Group, want.Version, want.Kind) {
			t.Fatalf("%s: hash %q is not %s", name, hashes[name], want)
		}
	}
	for _, absent := range []string{"bindings", "componentstatuses", "tokenreviews", "subjectaccessreviews"} {
		if _, ok := hashes[absent]; ok {
			t.Fatalf("%s is not persisted but listed", absent)
		}
	}
}

func TestMigrateStorageRewritesAnHPAStoredAtAutoscalingV1(t *testing.T) {
	f, client := newFakeKine(t)
	f.seed("/registry/horizontalpodautoscalers/ns/web", `{"kind":"HorizontalPodAutoscaler","apiVersion":"autoscaling/v1","metadata":{"name":"web","namespace":"ns"},"spec":{"scaleTargetRef":{"kind":"Deployment","name":"web","apiVersion":"apps/v1"},"maxReplicas":3,"targetCPUUtilizationPercentage":50}}`)
	hpa := storedResource(t, "horizontalpodautoscalers")
	ctx := context.Background()

	status, err := client.MigrateStorage(ctx, []kine.StoredResource{hpa}, kine.MigrationPassBudget)
	if err != nil {
		t.Fatal(err)
	}
	if status.Rewritten != 1 || !status.Done {
		t.Fatalf("%+v", status)
	}
	var stored struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(f.data["/registry/horizontalpodautoscalers/ns/web"], &stored); err != nil {
		t.Fatal(err)
	}
	if stored.APIVersion != "autoscaling/v2" {
		t.Fatalf("stored at %q", stored.APIVersion)
	}
	var got autoscalingv2.HorizontalPodAutoscaler
	if err := kine.NewStorage(client, storageCodec(autoscalingv2.SchemeGroupVersion), hpa.New).Get(ctx, "/horizontalpodautoscalers/ns/web", storage.GetOptions{}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.MaxReplicas != 3 || len(got.Spec.Metrics) != 1 || *got.Spec.Metrics[0].Resource.Target.AverageUtilization != 50 {
		t.Fatalf("migrated object lost fields: %+v", got.Spec)
	}
	again, err := client.MigrateStorage(ctx, []kine.StoredResource{hpa}, kine.MigrationPassBudget)
	if err != nil || again.Rewritten != 0 {
		t.Fatalf("second pass: %+v %v", again, err)
	}
}
