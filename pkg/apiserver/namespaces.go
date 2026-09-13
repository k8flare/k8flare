package apiserver

import (
	"context"
	"net/http"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

var systemNamespaces = []string{"default", "kube-system", "kube-public", "kube-node-lease"}

// ensureNamespaces creates the namespaces every cluster starts with, on
// the first request that reaches the API.
func ensureNamespaces(kine *KineClient, next http.Handler) http.Handler {
	var once sync.Once
	s := NewKineStorage(kine, scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), func() runtime.Object { return &corev1.Namespace{} })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			for _, name := range systemNamespaces {
				createNamespace(r.Context(), s, name)
			}
		})
		next.ServeHTTP(w, r)
	})
}

func createNamespace(ctx context.Context, s *KineStorage, name string) {
	ns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: uuid.NewUUID(), CreationTimestamp: metav1.Now()},
		Spec:       corev1.NamespaceSpec{Finalizers: []corev1.FinalizerName{corev1.FinalizerKubernetes}},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
	err := s.Create(ctx, "/namespaces/"+name, ns, nil, 0)
	if err != nil && !storage.IsExist(err) {
		println("apiserver: create namespace", name, ":", err.Error())
	}
}
