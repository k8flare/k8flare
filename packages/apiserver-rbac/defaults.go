package rbac

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	rbacv1 "k8s.io/kubernetes/pkg/apis/rbac/v1"
)

func init() {
	utilruntime.Must(rbacv1.RegisterDefaults(scheme.Scheme))
}
