package customresources

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apiserver/pkg/admission"
)

type denyAdmit struct{}

func (denyAdmit) Handles(admission.Operation) bool { return true }

func (denyAdmit) Admit(_ context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	return admission.NewForbidden(a, fmt.Errorf("denied by test"))
}

func (denyAdmit) Validate(_ context.Context, a admission.Attributes, _ admission.ObjectInterfaces) error {
	return admission.NewForbidden(a, fmt.Errorf("denied by test"))
}

func TestAdmitCRDWritesDeniesCreate(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := admitCRDWrites(denyAdmit{}, inner)
	req := httptest.NewRequest(http.MethodPost, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions", bytes.NewBufferString(`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"denieds.focus.example.com"}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated {
		t.Fatalf("create was allowed: %s", rec.Body.String())
	}
}

func TestBypassCRDGateSkipsReconcileOnWrites(t *testing.T) {
	path := "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com"
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !bypassCRDGate(httptest.NewRequest(method, path, nil)) {
			t.Fatalf("%s should bypass gate", method)
		}
	}
	if !bypassCRDGate(httptest.NewRequest(http.MethodGet, path+"/status", nil)) {
		t.Fatal("status should bypass gate")
	}
	if bypassCRDGate(httptest.NewRequest(http.MethodGet, path, nil)) {
		t.Fatal("GET spec should stay on gate")
	}
}
