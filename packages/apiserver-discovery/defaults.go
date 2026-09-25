package discovery

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	discoveryv1 "k8s.io/kubernetes/pkg/apis/discovery/v1"
)

func init() {
	utilruntime.Must(discoveryv1.RegisterDefaults(scheme.Scheme))
}
