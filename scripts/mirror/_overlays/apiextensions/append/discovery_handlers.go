package apiserver

import (
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/discovery"
)

func NewDiscoveryHandlers(delegate http.Handler) (*versionDiscoveryHandler, *groupDiscoveryHandler) {
	return &versionDiscoveryHandler{discovery: map[schema.GroupVersion]*discovery.APIVersionHandler{}, delegate: delegate},
		&groupDiscoveryHandler{discovery: map[string]*discovery.APIGroupHandler{}, delegate: delegate}
}
