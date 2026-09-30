package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func TestProvisionNamespaceAccountsCreatesDefaultObjects(t *testing.T) {
	data := map[string][]byte{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/kv" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := data[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		data[body.Key] = raw
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2})
	}))
	t.Cleanup(srv.Close)
	client := &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}
	sas, err := registry.NewStore(client, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: "serviceaccounts", SingularName: "serviceaccount", Namespaced: true, Kind: "ServiceAccount"})
	if err != nil {
		t.Fatal(err)
	}
	cms, err := registry.NewStore(client, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap"})
	if err != nil {
		t.Fatal(err)
	}
	nsAccounts.sas, nsAccounts.cms, nsAccounts.kine = sas, cms, client
	t.Cleanup(func() { nsAccounts.sas, nsAccounts.cms, nsAccounts.kine = nil, nil, nil })
	provisionNamespaceAccounts(context.Background(), "webhook-1")
	ctx := genericapirequest.WithNamespace(context.Background(), "webhook-1")
	if _, err := sas.Get(ctx, "default", &metav1.GetOptions{}); err != nil {
		t.Fatalf("default sa: %v", err)
	}
	got, err := cms.Get(ctx, "kube-root-ca.crt", &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("root ca: %v", err)
	}
	if _, ok := got.(*corev1.ConfigMap).Data["ca.crt"]; !ok {
		t.Fatal("missing ca.crt")
	}
}

func TestEnsureExtensionAuth(t *testing.T) {
	data := map[string][]byte{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/kv" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := data[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 1}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		data[body.Key] = raw
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2})
	}))
	t.Cleanup(srv.Close)
	client := &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}
	cms, err := registry.NewStore(client, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap"})
	if err != nil {
		t.Fatal(err)
	}
	nsAccounts.cms, nsAccounts.kine = cms, client
	t.Cleanup(func() { nsAccounts.cms, nsAccounts.kine = nil, nil })
	ensureExtensionAuth(context.Background())
	ctx := genericapirequest.WithNamespace(context.Background(), metav1.NamespaceSystem)
	got, err := cms.Get(ctx, "extension-apiserver-authentication", &metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dataMap := got.(*corev1.ConfigMap).Data
	want := map[string][]string{
		"requestheader-username-headers":     {"X-Remote-User"},
		"requestheader-group-headers":        {"X-Remote-Group"},
		"requestheader-extra-headers-prefix": {"X-Remote-Extra-"},
		"requestheader-allowed-names":        {"system:auth-proxy"},
	}
	requestHeaderCA, err := supervisor.NewVault(client).CAPEM(context.Background(), supervisor.RequestHeaderCAName)
	if err != nil {
		t.Fatal(err)
	}
	serverCA, err := supervisor.NewVault(client).CAPEM(context.Background(), "server-ca")
	if err != nil {
		t.Fatal(err)
	}
	if dataMap["requestheader-client-ca-file"] != string(requestHeaderCA) || string(requestHeaderCA) == string(serverCA) {
		t.Fatalf("requestheader-client-ca-file must be the request-header CA, not the server CA")
	}
	for key, values := range want {
		var decoded []string
		if err := json.Unmarshal([]byte(dataMap[key]), &decoded); err != nil {
			t.Fatalf("%s must be a JSON list as kube-apiserver writes it: %q: %v", key, dataMap[key], err)
		}
		if strings.Join(decoded, ",") != strings.Join(values, ",") {
			t.Fatalf("%s = %v want %v", key, decoded, values)
		}
	}
}

func TestNamespaceBeginCreateSkipsDryRun(t *testing.T) {
	fn, err := namespaceBeginCreate(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x"}}, &metav1.CreateOptions{DryRun: []string{"All"}})
	if err != nil {
		t.Fatal(err)
	}
	fn(context.Background(), true)
}

type rewrite struct {
	base string
	next http.RoundTripper
}

func (h rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := http.NewRequest(req.Method, h.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	u.Header = req.Header
	return h.next.RoundTrip(u)
}
