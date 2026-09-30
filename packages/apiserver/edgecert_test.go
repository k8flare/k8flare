package apiserver

import (
	"encoding/json"
	"net/http"
	"testing"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"k8s.io/apiserver/pkg/authentication/user"
)

func edgeCertMux() *http.ServeMux {
	mux := http.NewServeMux()
	installEdgeCertificate(mux, supervisor.NewVault(fakeVaultStore()))
	return mux
}

func TestEdgeCertificateRouteIsAdminOnly(t *testing.T) {
	if code, _ := callAdmin(edgeCertMux(), http.MethodPost, "/internal/edge-certificate", `{"hosts":["api.example.com"]}`, user.AllAuthenticated); code != http.StatusForbidden {
		t.Fatalf("non-admin: %d", code)
	}
}

func TestEdgeCertificateIssuesForTheRequestedHosts(t *testing.T) {
	code, body := callAdmin(edgeCertMux(), http.MethodPost, "/internal/edge-certificate", `{"hosts":["api.example.com"],"ttlSeconds":86400}`, user.SystemPrivilegedGroup)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	var issued supervisor.EdgeCertificate
	if err := json.Unmarshal([]byte(body), &issued); err != nil || issued.Cert == "" || issued.Key == "" || issued.ServerCA == "" || issued.ClientCA == "" {
		t.Fatalf("body %s", body)
	}
}

func TestEdgeCertificateRejectsMissingHostsAndNegativeTTL(t *testing.T) {
	for _, body := range []string{`{}`, `{"hosts":["a.example.com"],"ttlSeconds":-1}`} {
		if code, _ := callAdmin(edgeCertMux(), http.MethodPost, "/internal/edge-certificate", body, user.SystemPrivilegedGroup); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, code)
		}
	}
}
