package core

import (
	"context"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

func init() {
	registry.Customizers["configmaps"] = func(store *registry.Store, _ registry.Deps) {
		store.CreateStrategy = keyCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = immutableStrategy{store.UpdateStrategy}
	}
	registry.Customizers["secrets"] = func(store *registry.Store, _ registry.Deps) {
		store.CreateStrategy = keyCreateStrategy{store.CreateStrategy}
		store.UpdateStrategy = immutableStrategy{store.UpdateStrategy}
	}
}

type keyCreateStrategy struct{ rest.RESTCreateStrategy }

func (s keyCreateStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if s.RESTCreateStrategy != nil {
		s.RESTCreateStrategy.PrepareForCreate(ctx, obj)
	}
	applySecretStringData(obj)
}

type immutableStrategy struct{ rest.RESTUpdateStrategy }

func (s immutableStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if s.RESTUpdateStrategy != nil {
		s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	}
	applySecretStringData(obj)
}

func applySecretStringData(obj runtime.Object) {
	secret, ok := obj.(*corev1.Secret)
	if !ok || len(secret.StringData) == 0 {
		return
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	for k, v := range secret.StringData {
		secret.Data[k] = []byte(v)
	}
	secret.StringData = nil
}
