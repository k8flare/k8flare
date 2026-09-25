package admissionregistration

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	admissionregistrationv1 "k8s.io/kubernetes/pkg/apis/admissionregistration/v1"
)

func init() {
	utilruntime.Must(admissionregistrationv1.RegisterDefaults(scheme.Scheme))
}
