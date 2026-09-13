package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
)

// TableSource asks the printers Worker for the kubectl table of an
// encoded object or list of the given API group ("core" for the legacy
// group). It is set by the wasm entrypoint; without it, tables fall back to
// the name and age columns.
var TableSource func(ctx context.Context, group string, object []byte) ([]byte, error)

// ErrNoTableSource is what TableSource returns when the printers Worker is
// not bound.
var ErrNoTableSource = errors.New("no printers binding")

type tableConvertor struct {
	gv       schema.GroupVersion
	encoder  runtime.Encoder
	fallback rest.TableConvertor
}

func newTableConvertor(gv schema.GroupVersion, gr schema.GroupResource, encoder runtime.Encoder) rest.TableConvertor {
	return tableConvertor{gv: gv, encoder: encoder, fallback: rest.NewDefaultTableConvertor(gr)}
}

func (c tableConvertor) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	if TableSource == nil {
		return c.fallback.ConvertToTable(ctx, obj, tableOptions)
	}
	group := strings.TrimSuffix(c.gv.Group, ".k8s.io")
	if group == "" {
		group = "core"
	}
	encoded, err := runtime.Encode(c.encoder, obj)
	if err != nil {
		return nil, err
	}
	raw, err := TableSource(ctx, group, encoded)
	if errors.Is(err, ErrNoTableSource) {
		return c.fallback.ConvertToTable(ctx, obj, tableOptions)
	}
	if err != nil {
		return nil, fmt.Errorf("printers: %w", err)
	}
	var table metav1.Table
	if err := json.Unmarshal(raw, &table); err != nil {
		return nil, fmt.Errorf("printers: decode table: %w", err)
	}
	items := []runtime.Object{obj}
	if meta.IsListType(obj) {
		if items, err = meta.ExtractList(obj); err != nil {
			return nil, err
		}
		if l, err := meta.ListAccessor(obj); err == nil {
			table.ResourceVersion = l.GetResourceVersion()
			table.Continue = l.GetContinue()
			table.RemainingItemCount = l.GetRemainingItemCount()
		}
	} else if m, err := meta.Accessor(obj); err == nil {
		table.ResourceVersion = m.GetResourceVersion()
	}
	if len(table.Rows) != len(items) {
		return nil, fmt.Errorf("printers: %d rows for %d objects", len(table.Rows), len(items))
	}
	includeObject := metav1.IncludeMetadata
	if opts, ok := tableOptions.(*metav1.TableOptions); ok && opts != nil && opts.IncludeObject != "" {
		includeObject = opts.IncludeObject
	}
	for i, item := range items {
		switch includeObject {
		case metav1.IncludeNone:
		case metav1.IncludeObject:
			table.Rows[i].Object = runtime.RawExtension{Object: item}
		default:
			table.Rows[i].Object = runtime.RawExtension{Object: partialMetadata(item)}
		}
	}
	return &table, nil
}

func partialMetadata(obj runtime.Object) runtime.Object {
	m, _ := meta.Accessor(obj)
	partial := &metav1.PartialObjectMetadata{}
	partial.SetGroupVersionKind(metav1.SchemeGroupVersion.WithKind("PartialObjectMetadata"))
	partial.Name = m.GetName()
	partial.Namespace = m.GetNamespace()
	partial.UID = m.GetUID()
	partial.ResourceVersion = m.GetResourceVersion()
	partial.CreationTimestamp = m.GetCreationTimestamp()
	partial.DeletionTimestamp = m.GetDeletionTimestamp()
	partial.Labels = m.GetLabels()
	partial.Annotations = m.GetAnnotations()
	partial.OwnerReferences = m.GetOwnerReferences()
	partial.Finalizers = m.GetFinalizers()
	partial.Generation = m.GetGeneration()
	return partial
}
