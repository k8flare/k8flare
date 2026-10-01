package workloads

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestStatefulSetGapFollowsWhatTheControllerWillStillChange(t *testing.T) {
	settled := appsv1.StatefulSetStatus{ObservedGeneration: 2, Replicas: 3, ReadyReplicas: 3, UpdatedReplicas: 3, CurrentRevision: "new", UpdateRevision: "new"}
	heldBack := appsv1.StatefulSetStatus{ObservedGeneration: 2, Replicas: 3, ReadyReplicas: 3, UpdatedReplicas: 1, CurrentRevision: "old", UpdateRevision: "new"}
	rolling := func(partition int32) appsv1.StatefulSetUpdateStrategy {
		return appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType, RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: ptr.To(partition)}}
	}
	for _, tc := range []struct {
		name     string
		strategy appsv1.StatefulSetUpdateStrategy
		status   appsv1.StatefulSetStatus
		want     bool
	}{
		{"settled", appsv1.StatefulSetUpdateStrategy{}, settled, false},
		{"rolling update under way", appsv1.StatefulSetUpdateStrategy{}, heldBack, true},
		{"on delete leaves old pods", appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType}, heldBack, false},
		{"partition reached", rolling(2), heldBack, false},
		{"partition not reached", rolling(1), heldBack, true},
		{"unready replica", appsv1.StatefulSetUpdateStrategy{}, appsv1.StatefulSetStatus{ObservedGeneration: 2, Replicas: 3, ReadyReplicas: 2, UpdatedReplicas: 3, CurrentRevision: "new", UpdateRevision: "new"}, true},
		{"spec not observed", appsv1.StatefulSetUpdateStrategy{}, appsv1.StatefulSetStatus{ObservedGeneration: 1, Replicas: 3, ReadyReplicas: 3, UpdatedReplicas: 3, CurrentRevision: "new", UpdateRevision: "new"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default", Generation: 2},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3), UpdateStrategy: tc.strategy},
				Status:     tc.status,
			}
			got, err := statefulSetGap(context.Background(), fake.NewSimpleClientset(set))
			if err != nil || got != tc.want {
				t.Fatalf("gap = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}
