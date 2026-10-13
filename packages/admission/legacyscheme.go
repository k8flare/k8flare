package admission

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/apis/authentication"
	authenticationv1 "k8s.io/kubernetes/pkg/apis/authentication/v1"
	"k8s.io/kubernetes/pkg/apis/coordination"
	coordinationv1 "k8s.io/kubernetes/pkg/apis/coordination/v1"
	"k8s.io/kubernetes/pkg/apis/policy"
	policyv1 "k8s.io/kubernetes/pkg/apis/policy/v1"
	"k8s.io/kubernetes/pkg/apis/resource"
	resourcev1 "k8s.io/kubernetes/pkg/apis/resource/v1"
)

func init() {
	scheme := legacyscheme.Scheme
	utilruntime.Must(authentication.AddToScheme(scheme))
	utilruntime.Must(authenticationv1.AddToScheme(scheme))
	utilruntime.Must(coordination.AddToScheme(scheme))
	utilruntime.Must(coordinationv1.AddToScheme(scheme))
	utilruntime.Must(policy.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(resource.AddToScheme(scheme))
	utilruntime.Must(resourcev1.AddToScheme(scheme))
}
