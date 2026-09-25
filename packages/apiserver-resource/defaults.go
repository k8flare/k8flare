package resource

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	resourcev1 "k8s.io/kubernetes/pkg/apis/resource/v1"
)

func init() {
	utilruntime.Must(resourcev1.RegisterDefaults(scheme.Scheme))
}
