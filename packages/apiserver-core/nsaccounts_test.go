package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
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
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/cert"
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
	for _, state := range []string{"missing", "stale", "bare", "shared"} {
		t.Run(state, func(t *testing.T) { testEnsureExtensionAuth(t, state) })
	}
}

func testEnsureExtensionAuth(t *testing.T, state string) {
	data := map[string][]byte{}
	writes := 0
	revisions := map[string]int64{}
	var mu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/kv" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			k := r.URL.Query().Get("key")
			if v, ok := data[k]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": revisions[k]}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
			return
		}
		var body struct {
			Key      string `json:"key"`
			Value    string `json:"value"`
			Revision int64  `json:"revision"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		if body.Revision != revisions[body.Key] {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		raw, _ := base64.StdEncoding.DecodeString(body.Value)
		data[body.Key] = raw
		writes++
		revisions[body.Key] = int64(writes)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": writes})
	})
	client := &kine.Client{HTTP: &http.Client{Transport: extensionAuthTransport{handler}}}
	cms, err := registry.NewStore(client, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap"})
	if err != nil {
		t.Fatal(err)
	}
	nsAccounts.cms, nsAccounts.kine = cms, client
	t.Cleanup(func() { nsAccounts.cms, nsAccounts.kine = nil, nil })
	ctx := genericapirequest.WithNamespace(context.Background(), metav1.NamespaceSystem)
	vault := supervisor.NewVault(client)
	serverCA, err := vault.CAPEM(ctx, "server-ca")
	if err != nil {
		t.Fatal(err)
	}
	clientCA, err := vault.CAPEM(ctx, "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	stale := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "extension-apiserver-authentication", Namespace: metav1.NamespaceSystem, Labels: map[string]string{"owner": "other"}},
		Data: map[string]string{
			"client-ca-file":               string(serverCA),
			"requestheader-client-ca-file": string(serverCA),
			"requestheader-allowed-names":  "",
			"unrelated":                    "preserved",
		},
	}
	otherCA := ""
	if state == "bare" {
		stale.Data["requestheader-allowed-names"] = supervisor.RequestHeaderCN
		stale.Data["requestheader-username-headers"] = "X-Remote-User"
		stale.Data["requestheader-group-headers"] = "X-Remote-Group"
		stale.Data["requestheader-extra-headers-prefix"] = "X-Remote-Extra-"
	}
	if state == "shared" {
		bundle, err := vault.CAPEM(ctx, "other-ca")
		if err != nil {
			t.Fatal(err)
		}
		otherCA = string(bundle)
		stale.Data["client-ca-file"] += otherCA + otherCA
		stale.Data["requestheader-client-ca-file"] += otherCA
		stale.Data["requestheader-allowed-names"] = `["other-proxy","other-proxy"]`
		stale.Data["requestheader-uid-headers"] = `["X-Remote-Uid"]`
	}
	if state != "missing" {
		if _, err := cms.Create(ctx, stale, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	ensureExtensionAuth(ctx)
	got, err := cms.Get(ctx, "extension-apiserver-authentication", &metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dataMap := got.(*corev1.ConfigMap).Data
	if dataMap["client-ca-file"] != otherCA+string(clientCA) {
		t.Error("client-ca-file must contain the client CA bundle")
	}
	if state != "missing" && (dataMap["unrelated"] != "preserved" || got.(*corev1.ConfigMap).Labels["owner"] != "other") {
		t.Error("reconciliation must preserve unrelated data and metadata")
	}
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
	if dataMap["requestheader-client-ca-file"] != otherCA+string(requestHeaderCA) || string(requestHeaderCA) == string(serverCA) {
		t.Fatalf("requestheader-client-ca-file must be the request-header CA, not the server CA")
	}
	if state == "shared" {
		want["requestheader-allowed-names"] = []string{"other-proxy", supervisor.RequestHeaderCN}
		want["requestheader-uid-headers"] = []string{"X-Remote-Uid"}
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
	before := writes
	ensureExtensionAuth(ctx)
	if writes != before {
		t.Errorf("unchanged reconciliation wrote %d times", writes-before)
	}
	if _, err := vault.RotateCAs(ctx); err != nil {
		t.Fatal(err)
	}
	ensureExtensionAuth(ctx)
	got, err = cms.Get(ctx, "extension-apiserver-authentication", &metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rotatedCA, err := vault.CAPEM(ctx, "client-ca")
	if err != nil {
		t.Fatal(err)
	}
	for _, bundle := range []string{string(clientCA), string(rotatedCA)} {
		certs, err := cert.ParseCertsPEM([]byte(bundle))
		if err != nil {
			t.Fatal(err)
		}
		for _, ca := range certs {
			if !strings.Contains(got.(*corev1.ConfigMap).Data["client-ca-file"], string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))) {
				t.Error("reconciled bundle is missing a client CA certificate")
			}
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

type extensionAuthTransport struct {
	handler http.Handler
}

func (t extensionAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response := httptest.NewRecorder()
	t.handler.ServeHTTP(response, req)
	return response.Result(), nil
}
