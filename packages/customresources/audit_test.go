package customresources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
)

func TestForwardedAuditObjects(t *testing.T) {
	srv := httptest.NewServer(&memoryKine{})
	defer srv.Close()
	h, err := NewHandler(Config{Kine: &http.Client{Transport: rewriteTo{srv.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions", strings.NewReader(helmChartCRD))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Remote-User", "alice")
	req.Header.Set("X-Remote-Group", "system:masters")
	auth.ForwardAuditConfig(req.Header, audit.RequestAuditConfig{Level: auditinternal.LevelRequestResponse})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rec.Code, rec.Body)
	}
	request, response, _ := auth.TakeForwardedAuditObjects(rec.Header())
	for name, raw := range map[string][]byte{"request": request, "response": response} {
		var obj struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil || obj.Kind != "CustomResourceDefinition" || obj.Metadata.Name != "helmcharts.helm.cattle.io" {
			t.Fatalf("%s object is not the CRD: %v %s", name, err, raw)
		}
	}
}
