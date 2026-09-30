package registry

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)

var InternalScheme = runtime.NewScheme()

type Upstream struct {
	Strategy rest.RESTCreateUpdateStrategy
	Status   rest.RESTUpdateStrategy
}

var Upstreams = map[schema.GroupResource]Upstream{}

func toInternal(obj runtime.Object) (runtime.Object, error) {
	kinds, _, err := InternalScheme.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}
	internal, err := InternalScheme.New(kinds[0].GroupKind().WithVersion(runtime.APIVersionInternal))
	if err != nil {
		return nil, err
	}
	if err := InternalScheme.Convert(obj, internal, nil); err != nil {
		return nil, err
	}
	return internal, nil
}

func fromInternal(internal, obj runtime.Object) {
	_ = InternalScheme.Convert(internal, obj, nil)
}

func validateInternal(obj runtime.Object, validate func(internal runtime.Object) field.ErrorList) field.ErrorList {
	internal, err := toInternal(obj)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	return validate(internal)
}

func validateInternalUpdate(obj, old runtime.Object, validate func(internal, previous runtime.Object) field.ErrorList) field.ErrorList {
	internal, err := toInternal(obj)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	previous, err := toInternal(old)
	if err != nil {
		return field.ErrorList{field.InternalError(nil, err)}
	}
	return validate(internal, previous)
}

func warnInternal(obj runtime.Object, warn func(internal runtime.Object) []string) []string {
	internal, err := toInternal(obj)
	if err != nil {
		return nil
	}
	return warn(internal)
}

func warnInternalUpdate(obj, old runtime.Object, warn func(internal, previous runtime.Object) []string) []string {
	internal, err := toInternal(obj)
	if err != nil {
		return nil
	}
	previous, err := toInternal(old)
	if err != nil {
		return nil
	}
	return warn(internal, previous)
}

func prepareInternalUpdate(obj, old runtime.Object, prepare func(internal, previous runtime.Object)) {
	internal, err := toInternal(obj)
	if err != nil {
		return
	}
	previous, err := toInternal(old)
	if err != nil {
		return
	}
	prepare(internal, previous)
	fromInternal(internal, obj)
}

type upstreamStrategy struct {
	strategy
	up rest.RESTCreateUpdateStrategy
}

func (s upstreamStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	internal, err := toInternal(obj)
	if err != nil {
		return
	}
	s.up.PrepareForCreate(ctx, internal)
	fromInternal(internal, obj)
}

func (s upstreamStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return validateInternal(obj, func(internal runtime.Object) field.ErrorList { return s.up.Validate(ctx, internal) })
}

func (s upstreamStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return warnInternal(obj, func(internal runtime.Object) []string { return s.up.WarningsOnCreate(ctx, internal) })
}

func (s upstreamStrategy) Canonicalize(obj runtime.Object) {
	internal, err := toInternal(obj)
	if err != nil {
		return
	}
	s.up.Canonicalize(internal)
	fromInternal(internal, obj)
}

func (s upstreamStrategy) AllowCreateOnUpdate() bool { return s.up.AllowCreateOnUpdate() }

func (s upstreamStrategy) AllowUnconditionalUpdate() bool { return s.up.AllowUnconditionalUpdate() }

func (s upstreamStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	prepareInternalUpdate(obj, old, func(internal, previous runtime.Object) { s.up.PrepareForUpdate(ctx, internal, previous) })
}

func (s upstreamStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validateInternalUpdate(obj, old, func(internal, previous runtime.Object) field.ErrorList {
		return s.up.ValidateUpdate(ctx, internal, previous)
	})
}

func (s upstreamStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return warnInternalUpdate(obj, old, func(internal, previous runtime.Object) []string {
		return s.up.WarningsOnUpdate(ctx, internal, previous)
	})
}

func (s upstreamStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	if reset, ok := s.up.(rest.ResetFieldsStrategy); ok {
		return reset.GetResetFields()
	}
	return nil
}

type upstreamStatusStrategy struct {
	rest.RESTUpdateStrategy
	up rest.RESTUpdateStrategy
}

func (s upstreamStatusStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	prepareInternalUpdate(obj, old, func(internal, previous runtime.Object) { s.up.PrepareForUpdate(ctx, internal, previous) })
}

func (s upstreamStatusStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return validateInternalUpdate(obj, old, func(internal, previous runtime.Object) field.ErrorList {
		return s.up.ValidateUpdate(ctx, internal, previous)
	})
}

func (s upstreamStatusStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return warnInternalUpdate(obj, old, func(internal, previous runtime.Object) []string {
		return s.up.WarningsOnUpdate(ctx, internal, previous)
	})
}

func (s upstreamStatusStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	if reset, ok := s.up.(rest.ResetFieldsStrategy); ok {
		return reset.GetResetFields()
	}
	return nil
}
