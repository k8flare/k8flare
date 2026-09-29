package edgehost

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func TestLocateStreamForbidden(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://apiserver/api/v1/namespaces/default/pods/web/exec", nil)
	req.Header.Set("X-K8flare-Stream-Locate", "1")
	w := &bodyWriter{header: http.Header{}}
	LocateStream(w, req, nil, nil)
	if w.code != http.StatusForbidden {
		t.Fatal(w.code, w.body.String())
	}
	var status struct {
		Kind    string `json:"kind"`
		Status  string `json:"status"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := json.Unmarshal([]byte(w.body.String()), &status); err != nil {
		t.Fatal(err)
	}
	if status.Kind != "Status" || status.Status != "Failure" || status.Reason != "Forbidden" || status.Code != 403 || status.Message != "pod is not scheduled" {
		t.Fatalf("%+v", status)
	}
}

func TestStreamProtocol(t *testing.T) {
	if got := StreamProtocol("v4.channel.k8s.io, v5.channel.k8s.io"); got != "v5.channel.k8s.io" {
		t.Fatal(got)
	}
	if got := StreamProtocol("binary.k8s.io"); got != "binary.k8s.io" {
		t.Fatal(got)
	}
	if got := StreamProtocol(""); got != "" {
		t.Fatal(got)
	}
}

func TestLocateStreamLogIsHTTP(t *testing.T) {
	pod, _ := json.Marshal(map[string]any{"spec": map[string]any{"nodeName": "node-a", "containers": []any{map[string]string{"name": "app"}}}})
	var admitted []byte
	store := &kine.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{"kv": map[string]any{"value": base64.StdEncoding.EncodeToString(pod), "modRevision": 1}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})}}
	admission := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		admitted, _ = io.ReadAll(r.Body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"allowed":true}`)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://apiserver/api/v1/namespaces/default/pods/web/log?tailLines=5", nil)
	req.Header.Set("X-K8flare-Stream-Locate", "1")
	req.Header.Set("Sec-WebSocket-Protocol", "binary.k8s.io, base64.binary.k8s.io")
	w := &bodyWriter{header: http.Header{}}
	LocateStream(w, req, store, admission)
	if w.code != http.StatusOK {
		t.Fatal(w.code, w.body.String())
	}
	var loc struct {
		URL       string `json:"url"`
		Transport string `json:"transport"`
		Protocol  string `json:"protocol"`
	}
	if err := json.Unmarshal([]byte(w.body.String()), &loc); err != nil {
		t.Fatal(err)
	}
	if loc.Transport != "http" || loc.Protocol != "binary.k8s.io" || !strings.Contains(loc.URL, "/node/node-a/containerLogs/default/web/app") {
		t.Fatalf("%+v", loc)
	}
	var review struct {
		Kind struct {
			Kind string `json:"kind"`
		} `json:"kind"`
		Object struct {
			Kind      string `json:"kind"`
			TailLines int    `json:"tailLines"`
		} `json:"object"`
		Subresource string `json:"subresource"`
	}
	if err := json.Unmarshal(admitted, &review); err != nil {
		t.Fatal(err)
	}
	if review.Kind.Kind != "PodLogOptions" || review.Object.Kind != "PodLogOptions" || review.Object.TailLines != 5 || review.Subresource != "log" {
		t.Fatalf("%s", admitted)
	}
}

func TestLocateStream(t *testing.T) {
	pod, _ := json.Marshal(map[string]any{"spec": map[string]any{"nodeName": "node-a", "containers": []any{map[string]string{"name": "app"}}}})
	store := &kine.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{"kv": map[string]any{"value": base64.StdEncoding.EncodeToString(pod), "modRevision": 1}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})}}
	req, _ := http.NewRequest(http.MethodGet, "https://apiserver/api/v1/namespaces/default/pods/web/exec?stdout=true", nil)
	req.Header.Set("X-K8flare-Stream-Locate", "1")
	w := &bodyWriter{header: http.Header{}}
	LocateStream(w, req, store, nil)
	if w.code != http.StatusOK || !strings.Contains(w.body.String(), "/node/node-a/exec/default/web/app") {
		t.Fatal(w.code, w.body.String())
	}
}

type bodyWriter struct {
	header http.Header
	code   int
	body   strings.Builder
}

func (w *bodyWriter) Header() http.Header { return w.header }
func (w *bodyWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.body.Write(b)
}
func (w *bodyWriter) WriteHeader(status int) { w.code = status }
