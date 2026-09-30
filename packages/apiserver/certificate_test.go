package apiserver

import (
	"encoding/json"
	"net/http"
	"testing"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
	"k8s.io/apiserver/pkg/authentication/user"
)

func certificateMux() *http.ServeMux {
	mux := http.NewServeMux()
	installCertificates(mux, supervisor.NewVault(fakeVaultStore()))
	return mux
}

func TestCertificateRoutesAreAdminOnly(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/internal/certificate/rotate-ca"},
		{http.MethodGet, "/internal/certificate/check"},
	} {
		if code, _ := callAdmin(certificateMux(), c.method, c.path, "", user.AllAuthenticated); code != http.StatusForbidden {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}

func TestRotateCAThenCheckListsBothGenerations(t *testing.T) {
	mux := certificateMux()
	code, body := callAdmin(mux, http.MethodPost, "/internal/certificate/rotate-ca", "", user.SystemPrivilegedGroup)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	code, body = callAdmin(mux, http.MethodGet, "/internal/certificate/check", "", user.SystemPrivilegedGroup)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	var out struct {
		Items []supervisor.CAStatus `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || len(out.Items) != 6 {
		t.Fatalf("%v %s", err, body)
	}
}
