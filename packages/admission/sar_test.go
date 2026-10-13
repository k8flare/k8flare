package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSARAuthorizerSendsNamespace(t *testing.T) {
	var sent authorizationv1.SubjectAccessReview
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Fatal(err)
		}
		sent.Status.Allowed = true
		body, _ := json.Marshal(sent)
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}
	decision, _, err := (&sarAuthorizer{client: client}).Authorize(context.Background(), authorizer.AttributesRecord{
		User:            &user.DefaultInfo{Name: "system:node:n1"},
		Verb:            "request-serviceaccounts-token-audience",
		Namespace:       "ns",
		Resource:        "aud",
		Name:            "sa",
		ResourceRequest: true,
	})
	if err != nil || decision != authorizer.DecisionAllow {
		t.Fatalf("decision %v, err %v", decision, err)
	}
	if got := sent.Spec.ResourceAttributes.Namespace; got != "ns" {
		t.Fatalf("namespace sent %q, want ns", got)
	}
}
