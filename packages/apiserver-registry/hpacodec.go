package registry

import (
	"fmt"
	"io"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

type hpaStorageCodec struct{}

func (hpaStorageCodec) Identifier() runtime.Identifier { return "autoscaling.v2" }

func (hpaStorageCodec) Encode(obj runtime.Object, w io.Writer) error {
	stored, err := convertHPA(obj, hpaV2Dest(obj))
	if err != nil {
		return err
	}
	return scheme.Codecs.LegacyCodec(autoscalingv2.SchemeGroupVersion).Encode(stored, w)
}

func (hpaStorageCodec) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	stored, gvk, err := scheme.Codecs.UniversalDeserializer().Decode(data, defaults, nil)
	if err != nil {
		return nil, gvk, err
	}
	out := into
	if out == nil {
		out = hpaV2Dest(stored)
	}
	converted, err := convertHPA(stored, out)
	if err != nil {
		return nil, gvk, err
	}
	served := schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: converted.GetObjectKind().GroupVersionKind().Kind}
	return converted, &served, nil
}

func convertHPA(in, out runtime.Object) (runtime.Object, error) {
	if err := scheme.Scheme.Convert(in, out, nil); err == nil {
		return out, nil
	}
	hub, err := hpaHub(in)
	if err != nil {
		return nil, err
	}
	if err := scheme.Scheme.Convert(in, hub, nil); err != nil {
		return nil, fmt.Errorf("hpa convert %T -> hub: %w", in, err)
	}
	if err := scheme.Scheme.Convert(hub, out, nil); err != nil {
		return nil, fmt.Errorf("hpa convert hub -> %T: %w", out, err)
	}
	return out, nil
}

func hpaHub(obj runtime.Object) (runtime.Object, error) {
	kind := "HorizontalPodAutoscaler"
	switch obj.(type) {
	case *autoscalingv1.HorizontalPodAutoscalerList, *autoscalingv2.HorizontalPodAutoscalerList:
		kind = "HorizontalPodAutoscalerList"
	}
	return scheme.Scheme.New(schema.GroupVersion{Group: "autoscaling", Version: runtime.APIVersionInternal}.WithKind(kind))
}

func hpaV1Dest(obj runtime.Object) runtime.Object {
	switch obj.(type) {
	case *autoscalingv1.HorizontalPodAutoscalerList, *autoscalingv2.HorizontalPodAutoscalerList:
		return &autoscalingv1.HorizontalPodAutoscalerList{}
	default:
		return &autoscalingv1.HorizontalPodAutoscaler{}
	}
}

func hpaV2Dest(obj runtime.Object) runtime.Object {
	switch obj.(type) {
	case *autoscalingv1.HorizontalPodAutoscalerList, *autoscalingv2.HorizontalPodAutoscalerList:
		return &autoscalingv2.HorizontalPodAutoscalerList{}
	default:
		return &autoscalingv2.HorizontalPodAutoscaler{}
	}
}
