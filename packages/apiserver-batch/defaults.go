package batch

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	batchv1 "k8s.io/kubernetes/pkg/apis/batch/v1"
)

func init() {
	utilruntime.Must(batchv1.RegisterDefaults(scheme.Scheme))
}
