package autoscaling

import (
	"testing"

	registrytest "github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "autoscaling", Version: "v2"}, metav1.APIResource{Name: "horizontalpodautoscalers", Kind: "HorizontalPodAutoscaler", Namespaced: true})
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "h", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web", APIVersion: "apps/v1"},
			MaxReplicas:    0,
		},
	}
	scheme.Scheme.Default(hpa)
	obj := hpa
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "spec.maxReplicas", "must be set and greater than 0")
}
