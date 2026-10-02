//go:build !js

package openapi

import (
	"net/http"

	installer "github.com/k8flare/k8flare/packages/apiserver-installer"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"github.com/k8flare/k8flare/packages/kubeversion"
	"github.com/k8flare/k8flare/packages/openapi/definitions"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/apiserver/pkg/server/routes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// SpecHandler builds the OpenAPI documents from the live REST surface. It needs
// the whole installer closure, so it is host-only: the worker serves what
// bakeopenapi rendered with this instead.
func SpecHandler() (http.Handler, error) {
	installed, err := installer.Install(http.NewServeMux(), registry.Deps{})
	if err != nil {
		return nil, err
	}
	container := installed.Container
	namer := openapinamer.NewDefinitionNamer(scheme.Scheme)
	info := &spec.Info{InfoProps: spec.InfoProps{Title: "Kubernetes", Version: kubeversion.GitVersion}}
	oa := routes.OpenAPI{
		Config: &common.Config{
			ProtocolList:          []string{"https"},
			Info:                  info,
			DefaultResponse:       &spec.Response{ResponseProps: spec.ResponseProps{Description: "Default Response."}},
			GetOperationIDAndTags: openapinamer.GetOperationIDAndTags,
			GetDefinitionName:     namer.GetDefinitionName,
			GetDefinitions:        definitions.GetOpenAPIDefinitions,
		},
		V3Config: &common.OpenAPIV3Config{
			Info:                  info,
			DefaultResponse:       &spec3.Response{ResponseProps: spec3.ResponseProps{Description: "Default Response."}},
			GetOperationIDAndTags: openapinamer.GetOperationIDAndTags,
			GetDefinitionName:     namer.GetDefinitionName,
			GetDefinitions:        definitions.GetOpenAPIDefinitions,
		},
	}
	m := mux.NewPathRecorderMux("openapi")
	oa.InstallV2(container, m)
	oa.InstallV3(container, m)
	return m, nil
}
