package registry

import (
	"fmt"
	"net/http"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	requestInfoResolver = &genericapirequest.RequestInfoFactory{APIPrefixes: sets.NewString("api", "apis"), GrouplessAPIPrefixes: sets.NewString("api")}
	immortalNamespaces  = sets.NewString(metav1.NamespaceDefault, metav1.NamespaceSystem, metav1.NamespacePublic)
)

func NamespaceLifecycle(client *kine.Client) func(http.Handler) http.Handler {
	namespaces := kine.NewStorage(client, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.Namespace{} })
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info, err := requestInfoResolver.NewRequestInfo(r)
			if err != nil || !info.IsResourceRequest {
				next.ServeHTTP(w, r)
				return
			}
			if info.Verb == "delete" && info.Resource == "namespaces" && info.Subresource == "" && immortalNamespaces.Has(info.Name) {
				writeStatus(w, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, info.Name, fmt.Errorf("this namespace may not be deleted")))
				return
			}
			if info.Verb != "create" || info.Namespace == "" || info.Resource == "namespaces" || isLocalSubjectAccessReview(info) {
				next.ServeHTTP(w, r)
				return
			}
			namespace := &corev1.Namespace{}
			if err := namespaces.Get(r.Context(), "/namespaces/"+info.Namespace, storage.GetOptions{}, namespace); err != nil {
				if storage.IsNotFound(err) {
					err = apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, info.Namespace)
				}
				writeStatus(w, err)
				return
			}
			if namespace.Status.Phase != corev1.NamespaceTerminating {
				next.ServeHTTP(w, r)
				return
			}
			forbidden := apierrors.NewForbidden(schema.GroupResource{Group: info.APIGroup, Resource: info.Resource}, info.Name, fmt.Errorf("unable to create new content in namespace %s because it is being terminated", info.Namespace))
			forbidden.ErrStatus.Details.Causes = append(forbidden.ErrStatus.Details.Causes, metav1.StatusCause{
				Type:    corev1.NamespaceTerminatingCause,
				Message: fmt.Sprintf("namespace %s is being terminated", info.Namespace),
				Field:   "metadata.namespace",
			})
			writeStatus(w, forbidden)
		})
	}
}

func isLocalSubjectAccessReview(info *genericapirequest.RequestInfo) bool {
	return info.APIGroup == "authorization.k8s.io" && info.Resource == "localsubjectaccessreviews"
}

func writeStatus(w http.ResponseWriter, err error) {
	apiStatus, ok := err.(apierrors.APIStatus)
	if !ok {
		apiStatus = apierrors.NewInternalError(err)
	}
	status := apiStatus.Status()
	status.TypeMeta = metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}
	responsewriters.WriteRawJSON(int(status.Code), status, w)
}
