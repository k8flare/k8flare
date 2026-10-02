package hpa

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/scale"
	metricsclient "k8s.io/kubernetes/pkg/controller/podautoscaler/metrics"
)

func TestSyncScalesDeployment(t *testing.T) {
	one := int32(1)
	fifty := int32(50)
	ready := metav1.NewTime(time.Now().Add(-time.Hour))
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "d"}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default", Labels: map[string]string{"app": "d"}},
		Spec: v1.PodSpec{Containers: []v1.Container{{
			Name:      "c",
			Image:     "pause",
			Resources: v1.ResourceRequirements{Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m")}},
		}}},
		Status: v1.PodStatus{
			Phase:     v1.PodRunning,
			StartTime: &ready,
			Conditions: []v1.PodCondition{{
				Type:               v1.PodReady,
				Status:             v1.ConditionTrue,
				LastTransitionTime: ready,
			}},
			ContainerStatuses: []v1.ContainerStatus{{Name: "c", Ready: true}},
		},
	}
	h := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "h", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "d"},
			MinReplicas:    &one,
			MaxReplicas:    5,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name:   v1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &fifty},
				},
			}},
		},
	}
	client := fake.NewSimpleClientset(dep, pod, h)
	scales := &deploymentScales{replicas: 1}
	metrics := staticMetrics{pods: metricsclient.PodMetricsInfo{
		"p1": {Timestamp: time.Now(), Window: time.Minute, Value: 200},
	}}
	_, err := syncFor(context.Background(), client, scales, metrics, RESTMapper(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.AutoscalingV2().HorizontalPodAutoscalers("default").Get(context.Background(), "h", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.DesiredReplicas < 2 {
		t.Fatalf("desiredReplicas = %d", got.Status.DesiredReplicas)
	}
	if scales.replicas < 2 {
		t.Fatalf("scale replicas = %d", scales.replicas)
	}
}

func TestSyncNoHPAs(t *testing.T) {
	client := fake.NewSimpleClientset()
	start := time.Now()
	res, err := syncFor(context.Background(), client, &deploymentScales{}, staticMetrics{}, RESTMapper(), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("took too long: %v", elapsed)
	}
	if got := res.Objects["horizontalpodautoscalers"]; got != 0 {
		t.Fatalf("expected 0 horizontalpodautoscalers, got %d", got)
	}
	if !res.Drained {
		t.Fatal("expected Drained to be true")
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "list" && action.GetResource().Resource == "pods" {
			t.Fatalf("unexpected list pods action: %#v", action)
		}
	}
}

type deploymentScales struct{ replicas int32 }

func (s *deploymentScales) Scales(string) scale.ScaleInterface { return s }

func (s *deploymentScales) Get(_ context.Context, _ schema.GroupResource, name string, _ metav1.GetOptions) (*autoscalingv1.Scale, error) {
	return &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       autoscalingv1.ScaleSpec{Replicas: s.replicas},
		Status:     autoscalingv1.ScaleStatus{Replicas: s.replicas, Selector: "app=d"},
	}, nil
}

func (s *deploymentScales) Update(_ context.Context, _ schema.GroupResource, scale *autoscalingv1.Scale, _ metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	s.replicas = scale.Spec.Replicas
	return scale, nil
}

func (*deploymentScales) Patch(context.Context, schema.GroupVersionResource, string, types.PatchType, []byte, metav1.PatchOptions) (*autoscalingv1.Scale, error) {
	return nil, nil
}

type staticMetrics struct{ pods metricsclient.PodMetricsInfo }

func (m staticMetrics) GetResourceMetric(context.Context, v1.ResourceName, string, labels.Selector, string) (metricsclient.PodMetricsInfo, time.Time, error) {
	return m.pods, time.Now(), nil
}

func (staticMetrics) GetRawMetric(string, string, labels.Selector, labels.Selector) (metricsclient.PodMetricsInfo, time.Time, error) {
	return nil, time.Time{}, nil
}

func (staticMetrics) GetObjectMetric(string, string, *autoscalingv2.CrossVersionObjectReference, labels.Selector) (int64, time.Time, error) {
	return 0, time.Time{}, nil
}

func (staticMetrics) GetExternalMetric(string, string, labels.Selector) ([]int64, time.Time, error) {
	return nil, time.Time{}, nil
}
