//go:build js && wasm

package nodetunnel

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDialRequestWritesOneRemoteGroupHeaderPerGroup(t *testing.T) {
	in := httptest.NewRequest(http.MethodGet, "https://nodetunnel.internal/dial/n1/10.42.0.5/443/apis/wardle.example.com/v1alpha1/flunders", nil)
	in.Header.Set("X-Remote-User", "admin")
	in.Header.Set("X-Remote-Group", "system:masters, system:authenticated")
	in.Header.Set("X-Dial-TLS", "1")
	out := dialRequest(in, "10.42.0.5", "443", "/apis/wardle.example.com/v1alpha1/flunders")
	if got, want := out.Header.Values("X-Remote-Group"), []string{"system:masters", "system:authenticated"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("X-Remote-Group = %q, want %q", got, want)
	}
	if out.Header.Get("X-Dial-TLS") != "" || out.URL.String() != "http://10.42.0.5:443/apis/wardle.example.com/v1alpha1/flunders" {
		t.Fatalf("unexpected request %s %v", out.URL, out.Header)
	}
}
