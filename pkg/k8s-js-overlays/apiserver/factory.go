// js/wasm overlay: etcd-backed storage construction is not available on
// this platform; k8flare supplies its own storage.Interface directly.
package factory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

type DestroyFunc func()

func Create(c storagebackend.ConfigForResource, newFunc, newListFunc func() runtime.Object, resourcePrefix string) (storage.Interface, DestroyFunc, error) {
	return nil, nil, fmt.Errorf("storagebackend/factory: etcd storage is not supported on js/wasm")
}

func CreateHealthCheck(c storagebackend.Config, stopCh <-chan struct{}) (func() error, error) {
	return nil, fmt.Errorf("storagebackend/factory: not supported on js/wasm")
}

func CreateReadyCheck(c storagebackend.Config, stopCh <-chan struct{}) (func() error, error) {
	return nil, fmt.Errorf("storagebackend/factory: not supported on js/wasm")
}
