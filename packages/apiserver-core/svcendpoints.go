package core

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/util/dryrun"
)

func deleteServiceEndpoints(svc *corev1.Service, options *metav1.DeleteOptions) {
	if svc == nil || serviceEndpoints.store == nil {
		return
	}
	if options != nil && dryrun.IsDryRun(options.DryRun) {
		return
	}
	ctx := genericapirequest.WithNamespace(genericapirequest.NewContext(), svc.Namespace)
	if _, _, err := serviceEndpoints.store.Delete(ctx, svc.Name, rest.ValidateAllObjectFunc, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		println("services: delete endpoints", svc.Namespace+"/"+svc.Name+":", err.Error())
	}
}
