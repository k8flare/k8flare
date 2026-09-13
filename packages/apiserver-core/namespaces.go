package core

import (
	"net/http"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

var systemNamespaces = []string{"default", "kube-system", "kube-public", "kube-node-lease"}

func ensureNamespaces(store *genericregistry.Store, next http.Handler) http.Handler {
	var once sync.Once
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			ctx := genericapirequest.WithNamespace(r.Context(), metav1.NamespaceNone)
			for _, name := range systemNamespaces {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
				if _, err := store.Create(ctx, ns, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
					println("apiserver: create namespace", name, ":", err.Error())
				}
			}
		})
		next.ServeHTTP(w, r)
	})
}
