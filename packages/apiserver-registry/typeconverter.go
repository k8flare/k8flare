package registry

import (
	"encoding/json"
	"sync"

	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

var (
	typeConverterMu      sync.Mutex
	openAPIDefinitions   = map[string]*spec.Schema{}
	builtTypeConverter   managedfields.TypeConverter
	deducedTypeConverter = managedfields.NewDeducedTypeConverter()
)

func UseOpenAPIDefinitions(definitionsJSON []byte) error {
	var definitions map[string]*spec.Schema
	if err := json.Unmarshal(definitionsJSON, &definitions); err != nil {
		return err
	}
	typeConverterMu.Lock()
	defer typeConverterMu.Unlock()
	for name, definition := range definitions {
		openAPIDefinitions[name] = definition
	}
	builtTypeConverter = nil
	return nil
}

func TypeConverter() (managedfields.TypeConverter, error) {
	typeConverterMu.Lock()
	defer typeConverterMu.Unlock()
	if len(openAPIDefinitions) == 0 {
		return deducedTypeConverter, nil
	}
	if builtTypeConverter == nil {
		converter, err := managedfields.NewTypeConverter(openAPIDefinitions, false)
		if err != nil {
			return nil, err
		}
		builtTypeConverter = converter
	}
	return builtTypeConverter, nil
}
