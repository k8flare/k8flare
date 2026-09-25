package customresources

import (
	"io"
	"net/http"
	"strings"
	"testing"
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
