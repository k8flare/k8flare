package workloads

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	quota "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/apiserver/pkg/quota/v1/generic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func quotaInformer(factory informers.SharedInformerFactory, gvr schema.GroupVersionResource) (informers.GenericInformer, error) {
	example, ok := quotaExample(gvr)
	if !ok {
		return nil, fmt.Errorf("no quota informer for %s", gvr)
	}
	return quotaGeneric{
		resource: gvr.GroupResource(),
		informer: factory.InformerFor(example, func(kubernetes.Interface, time.Duration) cache.SharedIndexInformer {
			return newSnapshotInformer(example)
		}),
	}, nil
}

func quotaExample(gvr schema.GroupVersionResource) (runtime.Object, bool) {
	switch gvr {
	case v1.SchemeGroupVersion.WithResource("pods"):
		return &v1.Pod{}, true
	case v1.SchemeGroupVersion.WithResource("services"):
		return &v1.Service{}, true
	case v1.SchemeGroupVersion.WithResource("endpoints"):
		return &v1.Endpoints{}, true
	case v1.SchemeGroupVersion.WithResource("limitranges"):
		return &v1.LimitRange{}, true
	case v1.SchemeGroupVersion.WithResource("persistentvolumeclaims"):
		return &v1.PersistentVolumeClaim{}, true
	case v1.SchemeGroupVersion.WithResource("configmaps"):
		return &v1.ConfigMap{}, true
	case v1.SchemeGroupVersion.WithResource("resourcequotas"):
		return &v1.ResourceQuota{}, true
	case v1.SchemeGroupVersion.WithResource("replicationcontrollers"):
		return &v1.ReplicationController{}, true
	case v1.SchemeGroupVersion.WithResource("secrets"):
		return &v1.Secret{}, true
	case v1.SchemeGroupVersion.WithResource("serviceaccounts"):
		return &v1.ServiceAccount{}, true
	case appsv1.SchemeGroupVersion.WithResource("replicasets"):
		return &appsv1.ReplicaSet{}, true
	case appsv1.SchemeGroupVersion.WithResource("deployments"):
		return &appsv1.Deployment{}, true
	case appsv1.SchemeGroupVersion.WithResource("statefulsets"):
		return &appsv1.StatefulSet{}, true
	case appsv1.SchemeGroupVersion.WithResource("daemonsets"):
		return &appsv1.DaemonSet{}, true
	case batchv1.SchemeGroupVersion.WithResource("jobs"):
		return &batchv1.Job{}, true
	case batchv1.SchemeGroupVersion.WithResource("cronjobs"):
		return &batchv1.CronJob{}, true
	case policyv1.SchemeGroupVersion.WithResource("poddisruptionbudgets"):
		return &policyv1.PodDisruptionBudget{}, true
	case networkingv1.SchemeGroupVersion.WithResource("ingresses"):
		return &networkingv1.Ingress{}, true
	case networkingv1.SchemeGroupVersion.WithResource("networkpolicies"):
		return &networkingv1.NetworkPolicy{}, true
	case rbacv1.SchemeGroupVersion.WithResource("roles"):
		return &rbacv1.Role{}, true
	case rbacv1.SchemeGroupVersion.WithResource("rolebindings"):
		return &rbacv1.RoleBinding{}, true
	default:
		return nil, false
	}
}

func addQuotaCountEvaluators(registry quota.Registry, factory informers.SharedInformerFactory) {
	listerFor := generic.ListerFuncForResourceFunc(func(gvr schema.GroupVersionResource) (informers.GenericInformer, error) {
		return quotaInformer(factory, gvr)
	})
	for _, gvr := range quotaCountGVRs {
		if registry.Get(gvr.GroupResource()) != nil {
			continue
		}
		registry.Add(generic.NewObjectCountEvaluator(gvr.GroupResource(), generic.ListResourceUsingListerFunc(listerFor, gvr), ""))
	}
}

var quotaCountGVRs = []schema.GroupVersionResource{
	appsv1.SchemeGroupVersion.WithResource("replicasets"),
	appsv1.SchemeGroupVersion.WithResource("deployments"),
	appsv1.SchemeGroupVersion.WithResource("statefulsets"),
	appsv1.SchemeGroupVersion.WithResource("daemonsets"),
	batchv1.SchemeGroupVersion.WithResource("jobs"),
	batchv1.SchemeGroupVersion.WithResource("cronjobs"),
	policyv1.SchemeGroupVersion.WithResource("poddisruptionbudgets"),
	v1.SchemeGroupVersion.WithResource("serviceaccounts"),
	networkingv1.SchemeGroupVersion.WithResource("ingresses"),
	networkingv1.SchemeGroupVersion.WithResource("networkpolicies"),
	rbacv1.SchemeGroupVersion.WithResource("roles"),
	rbacv1.SchemeGroupVersion.WithResource("rolebindings"),
	v1.SchemeGroupVersion.WithResource("endpoints"),
	v1.SchemeGroupVersion.WithResource("limitranges"),
}

type quotaGeneric struct {
	resource schema.GroupResource
	informer cache.SharedIndexInformer
}

func (g quotaGeneric) Informer() cache.SharedIndexInformer { return g.informer }

func (g quotaGeneric) Lister() cache.GenericLister {
	return cache.NewGenericLister(g.informer.GetIndexer(), g.resource)
}
