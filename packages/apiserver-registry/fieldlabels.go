package registry

import (
	"fmt"
	"strconv"
	"strings"

	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
)

// Field selectors are read straight from the object's JSON form, the way
// upstream evaluates a CRD's selectableFields. Which labels a kind accepts
// is decided before this point by the scheme's field label conversions
// (registered in defaults.go from upstream). Event's "source" is the one
// label whose name is not its JSON path.
var fieldAliases = map[string]string{"source": "source.component"}

var eventsV1FieldAliases = map[string]string{
	"reportingComponent":             "reportingController",
	"involvedObject.kind":            "regarding.kind",
	"involvedObject.namespace":       "regarding.namespace",
	"involvedObject.name":            "regarding.name",
	"involvedObject.uid":             "regarding.uid",
	"involvedObject.apiVersion":      "regarding.apiVersion",
	"involvedObject.resourceVersion": "regarding.resourceVersion",
	"involvedObject.fieldPath":       "regarding.fieldPath",
}

func attrsFor(selector fields.Selector) storage.AttrFunc {
	requirements := selector.Requirements()
	return func(obj runtime.Object) (labels.Set, fields.Set, error) {
		m, err := meta.Accessor(obj)
		if err != nil {
			return nil, nil, err
		}
		set := fields.Set{"metadata.name": m.GetName(), "metadata.namespace": m.GetNamespace()}
		if len(requirements) == 0 {
			return m.GetLabels(), set, nil
		}
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range requirements {
			if _, ok := set[r.Field]; ok {
				continue
			}
			path := r.Field
			if alias, ok := fieldAliases[path]; ok {
				path = alias
			}
			if _, ok := obj.(*eventsv1.Event); ok {
				if alias, ok := eventsV1FieldAliases[path]; ok {
					path = alias
				}
			}
			value := lookupField(u, strings.Split(path, "."))
			if value == "" && (r.Value == "true" || r.Value == "false") {
				value = "false"
			}
			set[r.Field] = value
		}
		return m.GetLabels(), set, nil
	}
}

func lookupField(v any, path []string) string {
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		if v, ok = m[p]; !ok {
			return ""
		}
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}
