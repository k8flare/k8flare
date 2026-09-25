package core

import (
	"context"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
	apicore "k8s.io/kubernetes/pkg/apis/core"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
)

var validateScheme = runtime.NewScheme()

func init() {
	utilruntime.Must(corev1.AddToScheme(validateScheme))
	utilruntime.Must(apicore.AddToScheme(validateScheme))
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

func (s keyCreateStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	var errs field.ErrorList
	if s.RESTCreateStrategy != nil {
		errs = s.RESTCreateStrategy.Validate(ctx, obj)
	}
	return append(errs, validateCreate(obj)...)
}

type immutableStrategy struct{ rest.RESTUpdateStrategy }

func (s immutableStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if s.RESTUpdateStrategy != nil {
		s.RESTUpdateStrategy.PrepareForUpdate(ctx, obj, old)
	}
	applySecretStringData(obj)
}

func (s immutableStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	var errs field.ErrorList
	if s.RESTUpdateStrategy != nil {
		errs = s.RESTUpdateStrategy.ValidateUpdate(ctx, obj, old)
	}
	return append(errs, validateUpdate(obj, old)...)
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

func validateCreate(obj runtime.Object) field.ErrorList {
	switch obj.(type) {
	case *corev1.ConfigMap:
		in, err := toInternalConfigMap(obj)
		if err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
		return corevalidation.ValidateConfigMap(in)
	case *corev1.Secret:
		in, err := toInternalSecret(obj)
		if err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
		return corevalidation.ValidateSecret(in)
	default:
		return nil
	}
}

func validateUpdate(obj, old runtime.Object) field.ErrorList {
	switch obj.(type) {
	case *corev1.ConfigMap:
		neu, err := toInternalConfigMap(obj)
		if err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
		prev, err := toInternalConfigMap(old)
		if err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
		return corevalidation.ValidateConfigMapUpdate(neu, prev)
	case *corev1.Secret:
		neu, err := toInternalSecret(obj)
		if err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
		prev, err := toInternalSecret(old)
		if err != nil {
			return field.ErrorList{field.InternalError(nil, err)}
		}
		return corevalidation.ValidateSecretUpdate(neu, prev)
	default:
		return nil
	}
}

func toInternalConfigMap(obj runtime.Object) (*apicore.ConfigMap, error) {
	if in, ok := obj.(*apicore.ConfigMap); ok {
		return in, nil
	}
	out := &apicore.ConfigMap{}
	if err := validateScheme.Convert(obj, out, nil); err != nil {
		return nil, err
	}
	return out, nil
}

func toInternalSecret(obj runtime.Object) (*apicore.Secret, error) {
	if in, ok := obj.(*apicore.Secret); ok {
		return in, nil
	}
	out := &apicore.Secret{}
	if err := validateScheme.Convert(obj, out, nil); err != nil {
		return nil, err
	}
	return out, nil
}
