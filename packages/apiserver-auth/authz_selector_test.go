package auth

import (
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

func TestAuthorizerFieldSelector(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/pods?fieldSelector=spec.nodeName%3Dk8flare-c1", nil)
	ctx := genericapirequest.WithUser(req.Context(), &user.DefaultInfo{Name: "system:node:k8flare-c1"})
	ctx = genericapirequest.WithRequestInfo(ctx, &genericapirequest.RequestInfo{IsResourceRequest: true, Verb: "list", Resource: "pods"})
	req = req.WithContext(ctx)
	attrs, err := authorizerAttributes(req)
	if err != nil {
		t.Fatal(err)
	}
	reqs, err := attrs.GetFieldSelector()
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 || reqs[0].Field != "spec.nodeName" || reqs[0].Operator != selection.Equals || reqs[0].Value != "k8flare-c1" {
		t.Fatalf("reqs: %#v", reqs)
	}
}
