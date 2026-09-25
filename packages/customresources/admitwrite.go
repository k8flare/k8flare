package customresources

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	apiextensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiserver/pkg/admission"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
)

func admitCRDWrites(plugin admission.Interface, next http.Handler) http.Handler {
	if plugin == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			next.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(r.URL.Path, "/customresourcedefinitions") || strings.Contains(r.URL.Path, "/status") {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if _, _, err := apiextensionsapiserver.Codecs.UniversalDeserializer().Decode(body, nil, crd); err != nil {
			_ = json.Unmarshal(body, crd)
		}
		op := admission.Create
		if r.Method != http.MethodPost {
			op = admission.Update
		}
		user, _ := genericapirequest.UserFrom(r.Context())
		attrs := admission.NewAttributesRecord(
			crd, nil,
			apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"),
			"", crd.Name,
			apiextensionsv1.SchemeGroupVersion.WithResource("customresourcedefinitions"),
			"",
			op, nil, false, user,
		)
		if mutating, ok := plugin.(admission.MutationInterface); ok && mutating.Handles(op) {
			if err := mutating.Admit(r.Context(), attrs, nil); err != nil {
				responsewriters.ErrorNegotiated(err, apiextensionsapiserver.Codecs, apiextensionsv1.SchemeGroupVersion, w, r)
				return
			}
		}
		if validating, ok := plugin.(admission.ValidationInterface); ok && validating.Handles(op) {
			if err := validating.Validate(r.Context(), attrs, nil); err != nil {
				responsewriters.ErrorNegotiated(err, apiextensionsapiserver.Codecs, apiextensionsv1.SchemeGroupVersion, w, r)
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}
