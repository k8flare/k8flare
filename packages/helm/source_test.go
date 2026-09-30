package helm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPickVersion(t *testing.T) {
	candidates := []string{"0.2.0-rc.1", "0.1.0", "0.1.5", "1.0.0"}
	cases := map[string]string{
		"":           "1.0.0",
		"0.1.x":      "0.1.5",
		"~0.1.0":     "0.1.5",
		">=0.1.0 <1": "0.1.5",
		"0.1.0":      "0.1.0",
		"0.2.0-rc.1": "0.2.0-rc.1",
	}
	for constraint, want := range cases {
		got, err := pickVersion(candidates, constraint)
		if err != nil || got != want {
			t.Errorf("pickVersion(%q) = %q, %v; want %q", constraint, got, err, want)
		}
	}
	if _, err := pickVersion(candidates, "2.x"); err == nil {
		t.Error("expected an error when nothing matches")
	}
}

func TestFetchFromRepositorySelectsTheVersionAndResolvesRelativeURLs(t *testing.T) {
	server := chartRepository(t)
	source := &Source{HTTP: http.DefaultClient}
	chart := &helmv1.HelmChart{Spec: helmv1.HelmChartSpec{Chart: "demo", Repo: server.URL + "/", Version: "0.1.x"}}
	got, err := source.Fetch(context.Background(), chart)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(readTestdata(t, "demo-0.1.0.tgz")) {
		t.Fatal("downloaded archive differs")
	}
}

func TestFetchAuthenticatesWithTheAuthSecret(t *testing.T) {
	archive := readTestdata(t, "demo-0.1.0.tgz")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "me" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/index.yaml" {
			w.Write([]byte("entries:\n  demo:\n    - version: 0.1.0\n      urls: [demo-0.1.0.tgz]\n"))
			return
		}
		w.Write(archive)
	}))
	defer server.Close()
	source := &Source{HTTP: http.DefaultClient, Secrets: func(_ context.Context, namespace, name string) (map[string][]byte, error) {
		if namespace != "kube-system" || name != "repo-auth" {
			t.Fatalf("secret %s/%s", namespace, name)
		}
		return map[string][]byte{"username": []byte("me"), "password": []byte("secret")}, nil
	}}
	chart := &helmv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system"},
		Spec:       helmv1.HelmChartSpec{Chart: "demo", Repo: server.URL, AuthSecret: &corev1.LocalObjectReference{Name: "repo-auth"}},
	}
	if _, err := source.Fetch(context.Background(), chart); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Source{HTTP: http.DefaultClient}).Fetch(context.Background(), &helmv1.HelmChart{Spec: helmv1.HelmChartSpec{Chart: "demo", Repo: server.URL}}); err == nil {
		t.Fatal("expected a 401 without credentials")
	}
}

func TestFetchDirectChartURL(t *testing.T) {
	archive := readTestdata(t, "demo-0.1.0.tgz")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive) }))
	defer server.Close()
	got, err := (&Source{HTTP: http.DefaultClient}).Fetch(context.Background(), &helmv1.HelmChart{Spec: helmv1.HelmChartSpec{Chart: server.URL + "/demo-0.1.0.tgz"}})
	if err != nil || len(got) != len(archive) {
		t.Fatalf("got %d bytes, %v", len(got), err)
	}
}

func ociRegistry(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(archive)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	manifest, _ := json.Marshal(map[string]any{"layers": []map[string]any{
		{"mediaType": "application/vnd.cncf.helm.config.v1+json", "digest": "sha256:0"},
		{"mediaType": ociChartLayer, "digest": digest},
	}})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if user, _, _ := r.BasicAuth(); user != "puller" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("scope") != "repository:charts/demo:pull" {
				t.Errorf("scope = %q", r.URL.Query().Get("scope"))
			}
			w.Write([]byte(`{"token":"t0k"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer t0k" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry",scope="repository:charts/demo:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v2/charts/demo/tags/list":
			w.Write([]byte(`{"tags":["0.1.0","0.1.1"]}`))
		case "/v2/charts/demo/manifests/0.1.1", "/v2/charts/demo/manifests/0.1.0":
			w.Write(manifest)
		case "/v2/charts/demo/blobs/" + digest:
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchFromOCIRegistryWithBearerToken(t *testing.T) {
	archive := readTestdata(t, "demo-0.1.0.tgz")
	registry := ociRegistry(t, archive)
	secrets := func(context.Context, string, string) (map[string][]byte, error) {
		return map[string][]byte{"username": []byte("puller"), "password": []byte("pw")}, nil
	}
	host := strings.TrimPrefix(registry.URL, "http://")
	for _, version := range []string{"", "0.1.0", "0.1.x"} {
		chart := &helmv1.HelmChart{Spec: helmv1.HelmChartSpec{
			Chart:      "oci://" + host + "/charts/demo",
			Version:    version,
			PlainHTTP:  true,
			AuthSecret: &corev1.LocalObjectReference{Name: "reg"},
		}}
		got, err := (&Source{HTTP: http.DefaultClient, Secrets: secrets}).Fetch(context.Background(), chart)
		if err != nil {
			t.Fatalf("version %q: %v", version, err)
		}
		if string(got) != string(archive) {
			t.Fatalf("version %q: archive differs", version)
		}
	}
}
