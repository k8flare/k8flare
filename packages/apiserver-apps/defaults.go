package apps

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	appsv1 "k8s.io/kubernetes/pkg/apis/apps/v1"
)

func init() {
	utilruntime.Must(appsv1.RegisterDefaults(scheme.Scheme))
}
