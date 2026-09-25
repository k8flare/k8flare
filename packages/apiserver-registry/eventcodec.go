package registry

import (
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func storageCodec(gv schema.GroupVersion) runtime.Codec {
	if gv.Group == "events.k8s.io" {
		return eventStorageCodec{}
	}
	if gv.Group == "autoscaling" && gv.Version == "v2" {
		return hpaStorageCodec{}
	}
	return scheme.Codecs.LegacyCodec(gv)
}

type eventStorageCodec struct{}

func (eventStorageCodec) Identifier() runtime.Identifier { return "events.k8s.io-core" }

func (eventStorageCodec) Encode(obj runtime.Object, w io.Writer) error {
	stored, err := convertEvent(obj, coreEventDest(obj))
	if err != nil {
		return err
	}
	fillEventSource(stored)
	return scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion).Encode(stored, w)
}

func (eventStorageCodec) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	stored, gvk, err := scheme.Codecs.UniversalDeserializer().Decode(data, defaults, nil)
	if err != nil {
		return nil, gvk, err
	}
	out := into
	if out == nil {
		out = eventsDest(stored)
	}
	converted, err := convertEvent(stored, out)
	if err != nil {
		return nil, gvk, err
	}
	served := schema.GroupVersionKind{Group: "events.k8s.io", Version: "v1", Kind: converted.GetObjectKind().GroupVersionKind().Kind}
	return converted, &served, nil
}

func convertEvent(in, out runtime.Object) (runtime.Object, error) {
	if err := scheme.Scheme.Convert(in, out, nil); err == nil {
		return out, nil
	}
	hub, err := eventHub(in)
	if err != nil {
		return nil, err
	}
	if err := scheme.Scheme.Convert(in, hub, nil); err != nil {
		return nil, fmt.Errorf("event convert %T -> hub: %w", in, err)
	}
	if err := scheme.Scheme.Convert(hub, out, nil); err != nil {
		return nil, fmt.Errorf("event convert hub -> %T: %w", out, err)
	}
	return out, nil
}

func eventHub(obj runtime.Object) (runtime.Object, error) {
	kind := "Event"
	switch obj.(type) {
	case *eventsv1.EventList, *corev1.EventList:
		kind = "EventList"
	}
	return scheme.Scheme.New(schema.GroupVersion{Group: "events.k8s.io", Version: runtime.APIVersionInternal}.WithKind(kind))
}

func fillEventSource(obj runtime.Object) {
	switch e := obj.(type) {
	case *corev1.Event:
		fillOneEventSource(e)
	case *corev1.EventList:
		for i := range e.Items {
			fillOneEventSource(&e.Items[i])
		}
	}
}

func fillOneEventSource(e *corev1.Event) {
	if e.Source.Component == "" {
		e.Source.Component = e.ReportingController
	}
	if e.Source.Host == "" {
		e.Source.Host = e.ReportingInstance
	}
}

func coreEventDest(obj runtime.Object) runtime.Object {
	switch obj.(type) {
	case *eventsv1.EventList, *corev1.EventList:
		return &corev1.EventList{}
	default:
		return &corev1.Event{}
	}
}

func eventsDest(obj runtime.Object) runtime.Object {
	switch obj.(type) {
	case *corev1.EventList, *eventsv1.EventList:
		return &eventsv1.EventList{}
	default:
		return &eventsv1.Event{}
	}
}
