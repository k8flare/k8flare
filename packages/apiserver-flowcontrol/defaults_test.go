package flowcontrol

import (
	"testing"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDefaultsFlowSchemaAndPriorityLevel(t *testing.T) {
	schema := &flowcontrolv1.FlowSchema{}
	scheme.Scheme.Default(schema)
	if schema.Spec.MatchingPrecedence != 1000 {
		t.Fatalf("precedence=%d", schema.Spec.MatchingPrecedence)
	}
	level := &flowcontrolv1.PriorityLevelConfiguration{Spec: flowcontrolv1.PriorityLevelConfigurationSpec{
		Limited: &flowcontrolv1.LimitedPriorityLevelConfiguration{LimitResponse: flowcontrolv1.LimitResponse{Queuing: &flowcontrolv1.QueuingConfiguration{}}},
	}}
	scheme.Scheme.Default(level)
	if level.Spec.Limited.NominalConcurrencyShares == nil || *level.Spec.Limited.NominalConcurrencyShares != 30 {
		t.Fatalf("shares=%v", level.Spec.Limited.NominalConcurrencyShares)
	}
	if level.Spec.Limited.LimitResponse.Queuing.Queues != 64 || level.Spec.Limited.LimitResponse.Queuing.HandSize != 8 || level.Spec.Limited.LimitResponse.Queuing.QueueLengthLimit != 50 {
		t.Fatalf("queuing=%+v", level.Spec.Limited.LimitResponse.Queuing)
	}
}
