//go:build js

// GOOS=js overlay: the etcd3-backed storage factory does not build for wasm
// (etcd client, go-systemd) and k8flare supplies its own storage.Interface.
package factory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

type DestroyFunc func()

var errNoEtcd = fmt.Errorf("storagebackend/factory: etcd storage is not available in this build")

func Create(c storagebackend.ConfigForResource, newFunc, newListFunc func() runtime.Object, resourcePrefix string) (storage.Interface, DestroyFunc, error) {
	return nil, nil, errNoEtcd
}

func CreateHealthCheck(c storagebackend.Config, stopCh <-chan struct{}) (func() error, error) {
	return nil, errNoEtcd
}

func CreateReadyCheck(c storagebackend.Config, stopCh <-chan struct{}) (func() error, error) {
	return nil, errNoEtcd
}
