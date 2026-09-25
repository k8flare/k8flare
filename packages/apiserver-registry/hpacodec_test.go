package registry

import (
	"bytes"
	"testing"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	_ "github.com/k8flare/k8flare/packages/apiserver-autoscaling"
	"github.com/k8flare/k8flare/packages/apiserver-autoscaling/storageconv"
)

func init() { storageconv.Install() }

func TestHPAStorageCodecRoundTrip(t *testing.T) {
	in := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web", APIVersion: "apps/v1"},
			MinReplicas:    int32ptr(1),
			MaxReplicas:    5,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: "cpu",
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: int32ptr(80),
					},
				},
			}},
		},
	}
	var buf bytes.Buffer
	codec := hpaStorageCodec{}
	if err := codec.Encode(in, &buf); err != nil {
		t.Fatal(err)
	}
	stored, _, err := scheme.Codecs.UniversalDeserializer().Decode(buf.Bytes(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	storedV2, ok := stored.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		t.Fatalf("stored %T", stored)
	}
	if storedV2.Spec.MaxReplicas != 5 || storedV2.Spec.ScaleTargetRef.Name != "web" {
		t.Fatalf("stored %+v", storedV2.Spec)
	}
	if len(storedV2.Spec.Metrics) != 1 || storedV2.Spec.Metrics[0].Resource == nil || storedV2.Spec.Metrics[0].Resource.Target.AverageUtilization == nil || *storedV2.Spec.Metrics[0].Resource.Target.AverageUtilization != 80 {
		t.Fatalf("cpu %+v", storedV2.Spec.Metrics)
	}
	out := &autoscalingv2.HorizontalPodAutoscaler{}
	got, _, err := codec.Decode(buf.Bytes(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	v2 := got.(*autoscalingv2.HorizontalPodAutoscaler)
	if v2.Spec.MaxReplicas != 5 || v2.Spec.ScaleTargetRef.Name != "web" {
		t.Fatalf("v2 %+v", v2.Spec)
	}
}

func TestHPAStorageCodecReadsV1(t *testing.T) {
	cpu := int32(70)
	v1 := &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef:                 autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "api", APIVersion: "apps/v1"},
			MinReplicas:                    int32ptr(2),
			MaxReplicas:                    8,
			TargetCPUUtilizationPercentage: &cpu,
		},
	}
	data, err := runtime.Encode(scheme.Codecs.LegacyCodec(autoscalingv1.SchemeGroupVersion), v1)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := hpaStorageCodec{}.Decode(data, nil, &autoscalingv2.HorizontalPodAutoscaler{})
	if err != nil {
		t.Fatal(err)
	}
	v2 := got.(*autoscalingv2.HorizontalPodAutoscaler)
	if v2.Spec.MaxReplicas != 8 || v2.Spec.ScaleTargetRef.Name != "api" {
		t.Fatalf("v2 %+v", v2.Spec)
	}
}

func TestHPAStorageCodecPreservesConditions(t *testing.T) {
	in := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web"},
			MaxReplicas:    3,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 1,
			DesiredReplicas: 1,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{{
				Type:   autoscalingv2.AbleToScale,
				Status: corev1.ConditionTrue,
				Reason: "SucceededGetScale",
			}},
		},
	}
	var buf bytes.Buffer
	if err := (hpaStorageCodec{}).Encode(in, &buf); err != nil {
		t.Fatal(err)
	}
	out := &autoscalingv2.HorizontalPodAutoscaler{}
	got, _, err := (hpaStorageCodec{}).Decode(buf.Bytes(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	v2 := got.(*autoscalingv2.HorizontalPodAutoscaler)
	if len(v2.Status.Conditions) != 1 || v2.Status.Conditions[0].Type != autoscalingv2.AbleToScale {
		t.Fatalf("conditions %+v", v2.Status.Conditions)
	}
}

func int32ptr(n int32) *int32 { return &n }
