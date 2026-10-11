package customresources

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkerNameFromRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://k8flare.com/worker/convert-echo", nil)
	if err != nil {
		t.Fatal(err)
	}
	name, ok := workerNameFromRequest(req)
	if !ok || name != "convert-echo" {
		t.Fatalf("got %q %v", name, ok)
	}
	plain, err := http.NewRequest(http.MethodPost, "https://example.com/convert", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := workerNameFromRequest(plain); ok {
		t.Fatal("external URL must not rewrite")
	}
}

func TestURLTransportRewritesWorkerToHooks(t *testing.T) {
	var got string
	hooks := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.URL.String()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	r := newConversionResolver(nil, nil, nil, hooks)
	req, err := http.NewRequest(http.MethodPost, "https://k8flare.com/worker/convert-echo", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.urlTransport().RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if got != "https://hooks.internal/hook/convert-echo" {
		t.Fatalf("rewrote to %s", got)
	}
}

func webhookConvertedCRD(annotations map[string]string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.com", Annotations: annotations},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Conversion: &apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.WebhookConverter,
				Webhook: &apiextensionsv1.WebhookConversion{
					ClientConfig:             &apiextensionsv1.WebhookClientConfig{Service: &apiextensionsv1.ServiceReference{Namespace: "default", Name: "convert"}},
					ConversionReviewVersions: []string{"v1"},
				},
			},
		},
	}
}

func TestWorkerAnnotationRoutesConversionToWorker(t *testing.T) {
	crd := webhookConvertedCRD(map[string]string{workerAnnot: " convert-echo "})
	routeConversionToWorker(crd)
	cfg := crd.Spec.Conversion.Webhook.ClientConfig
	if cfg.Service != nil || cfg.URL == nil {
		t.Fatalf("clientConfig %+v", cfg)
	}
	var got string
	hooks := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.URL.String()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	req, err := http.NewRequest(http.MethodPost, *cfg.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := newConversionResolver(nil, nil, nil, hooks).urlTransport().RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "https://hooks.internal/hook/convert-echo" {
		t.Fatalf("called %s", got)
	}
}

func TestConversionKeepsItsClientConfigWithoutAWorkerName(t *testing.T) {
	for _, annotations := range []map[string]string{nil, {workerAnnot: " "}, {workerAnnot: "a/b"}} {
		crd := webhookConvertedCRD(annotations)
		routeConversionToWorker(crd)
		cfg := crd.Spec.Conversion.Webhook.ClientConfig
		if cfg.URL != nil || cfg.Service == nil || cfg.Service.Name != "convert" {
			t.Fatalf("%v: clientConfig %+v", annotations, cfg)
		}
	}
	none := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{workerAnnot: "convert-echo"}}}
	routeConversionToWorker(none)
	if none.Spec.Conversion != nil {
		t.Fatalf("conversion %+v", none.Spec.Conversion)
	}
}

func TestSplitServiceHost(t *testing.T) {
	name, ns, port, err := splitServiceHost("e2e-test-crd-conversion-webhook.crd-selectable-fields-2102.svc:9443")
	if err != nil {
		t.Fatal(err)
	}
	if name != "e2e-test-crd-conversion-webhook" || ns != "crd-selectable-fields-2102" || port != 9443 {
		t.Fatalf("got %s %s %d", name, ns, port)
	}
	if _, _, _, err := splitServiceHost("no-port"); err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceConversionDialsTheEndpointThroughTheNodeTunnel(t *testing.T) {
	node := "node-a"
	ep, err := json.Marshal(corev1.Endpoints{Subsets: []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{IP: "10.42.0.7", NodeName: &node}},
		Ports:     []corev1.EndpointPort{{Port: 9444}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryKine{data: map[string]memoryEntry{
		"/registry/endpoints/ns-a/convert": {value: base64.StdEncoding.EncodeToString(ep), rev: 1},
	}}
	srv := httptest.NewServer(store)
	defer srv.Close()
	var got string
	tunnel := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.URL.String()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})
	transport := &serviceDialTransport{kine: &kine.Client{HTTP: &http.Client{Transport: rewriteTo{srv.URL}}}, next: tunnel}
	req, err := http.NewRequest(http.MethodPost, "https://convert.ns-a.svc:443/crdconvert", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "https://nodetunnel.internal/dial/node-a/10.42.0.7/9444/crdconvert" {
		t.Fatalf("dialed %s", got)
	}
}
