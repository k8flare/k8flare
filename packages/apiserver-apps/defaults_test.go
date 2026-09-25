package apps

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsStatefulSetRevisionHistoryLimit(t *testing.T) {
	ss := &appsv1.StatefulSet{}
	scheme.Scheme.Default(ss)
	if ss.Spec.RevisionHistoryLimit == nil || *ss.Spec.RevisionHistoryLimit != 10 {
		t.Fatalf("revisionHistoryLimit=%v", ss.Spec.RevisionHistoryLimit)
	}
	if ss.Spec.PodManagementPolicy != appsv1.OrderedReadyPodManagement {
		t.Fatalf("podManagementPolicy=%q", ss.Spec.PodManagementPolicy)
	}
}
