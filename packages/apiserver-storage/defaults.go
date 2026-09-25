package storage

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	storagev1 "k8s.io/kubernetes/pkg/apis/storage/v1"
)

func init() {
	utilruntime.Must(storagev1.RegisterDefaults(scheme.Scheme))
}
