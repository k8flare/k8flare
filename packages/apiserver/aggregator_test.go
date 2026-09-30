package apiserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

func workerAPIServiceStore(t *testing.T, worker string) *kine.Client {
	t.Helper()
	svc := apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: "v1.wardle.example.com", Annotations: map[string]string{workerAnnot: worker}},
		Spec:       apiregistrationv1.APIServiceSpec{Group: "wardle.example.com", Version: "v1"},
	}
	raw, err := json.Marshal(svc)
	if err != nil {
		t.Fatal(err)
	}
	value := base64.StdEncoding.EncodeToString(raw)
	return &kine.Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body any
		switch r.URL.Path {
		case "/list":
			body = map[string]any{"revision": 1, "kvs": []map[string]any{{"key": "/registry/apiservices/v1.wardle.example.com", "value": value, "modRevision": 1}}}
		case "/kv":
			body = map[string]any{"revision": 1, "kv": map[string]any{"key": r.URL.Query().Get("key"), "value": value, "modRevision": 1}}
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
		}
		data, _ := json.Marshal(body)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}, Request: r}, nil
	})}}
}

func TestWorkerAPIServiceIsRemote(t *testing.T) {
	store := workerAPIServiceStore(t, "wardle")
	if _, ok := remoteAPIService(t.Context(), store, "wardle.example.com", "v1"); !ok {
		t.Fatal("APIService pointing at a worker is not remote")
	}
	if !hasRemoteAPIServiceGroup(t.Context(), store, "wardle.example.com") {
		t.Fatal("group with a worker APIService is not remote")
	}
	groups := remoteAPIServiceGroups(t.Context(), store)
	if len(groups) != 1 || groups[0].Name != "wardle.example.com" {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestAPIGroupAndVersion(t *testing.T) {
	cases := []struct {
		path, group, version string
	}{
		{"/apis/wardle.example.com/v1/namespaces/default/flunders", "wardle.example.com", "v1"},
		{"/apis/wardle.example.com/v1", "wardle.example.com", "v1"},
		{"/apis/wardle.example.com", "wardle.example.com", ""},
		{"/apis/apps/v1/deployments", "apps", "v1"},
		{"/api/v1/pods", "", ""},
	}
	for _, tc := range cases {
		g, v := apiGroupAndVersion(tc.path)
		if g != tc.group || v != tc.version {
			t.Fatalf("%s: got %q %q want %q %q", tc.path, g, v, tc.group, tc.version)
		}
	}
}

func TestAPIServiceName(t *testing.T) {
	if got := apiServiceName("wardle.example.com", "v1"); got != "v1.wardle.example.com" {
		t.Fatalf("got %q", got)
	}
	if got := apiServiceName("", "v1"); got != "v1." {
		t.Fatalf("got %q", got)
	}
}
