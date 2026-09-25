package authorization

import (
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func TestAttributesFromResource(t *testing.T) {
	got := attributesFrom(authorizer.AttributesRecord{}, &authorizationv1.ResourceAttributes{
		Namespace:   "ns",
		Verb:        "get",
		Group:       "apps",
		Version:     "v1",
		Resource:    "deployments",
		Subresource: "status",
		Name:        "web",
	}, nil)
	if !got.ResourceRequest || got.Namespace != "ns" || got.Verb != "get" || got.APIGroup != "apps" || got.Resource != "deployments" || got.Name != "web" {
		t.Fatalf("%+v", got)
	}
}

func TestResourceRules(t *testing.T) {
	got := resourceRules([]authorizer.ResourceRuleInfo{&authorizer.DefaultResourceRuleInfo{
		Verbs:     []string{"get", "list"},
		APIGroups: []string{""},
		Resources: []string{"pods"},
	}})
	if len(got) != 1 || got[0].Resources[0] != "pods" || got[0].Verbs[0] != "get" {
		t.Fatalf("%+v", got)
	}
}

func TestExtraFrom(t *testing.T) {
	got := extraFrom(map[string]authorizationv1.ExtraValue{"scopes": {"user"}})
	if got["scopes"][0] != "user" {
		t.Fatalf("%v", got)
	}
}
