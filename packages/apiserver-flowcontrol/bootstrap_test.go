package flowcontrol

import (
	"testing"

	flowcontrolbootstrap "k8s.io/apiserver/pkg/apis/flowcontrol/bootstrap"
)

func TestUpstreamAPFBootstrapCounts(t *testing.T) {
	if got := len(flowcontrolbootstrap.MandatoryPriorityLevelConfigurations) + len(flowcontrolbootstrap.SuggestedPriorityLevelConfigurations); got != 8 {
		t.Fatalf("prioritylevelconfigurations = %d", got)
	}
	if got := len(flowcontrolbootstrap.MandatoryFlowSchemas) + len(flowcontrolbootstrap.SuggestedFlowSchemas); got != 11 {
		t.Fatalf("flowschemas = %d", got)
	}
	names := map[string]bool{}
	for _, plc := range append(flowcontrolbootstrap.MandatoryPriorityLevelConfigurations, flowcontrolbootstrap.SuggestedPriorityLevelConfigurations...) {
		if plc.Name == "" || names[plc.Name] {
			t.Fatalf("prioritylevelconfiguration name %q", plc.Name)
		}
		names[plc.Name] = true
	}
	for _, fs := range append(flowcontrolbootstrap.MandatoryFlowSchemas, flowcontrolbootstrap.SuggestedFlowSchemas...) {
		if fs.Name == "" || names["fs:"+fs.Name] {
			t.Fatalf("flowschema name %q", fs.Name)
		}
		names["fs:"+fs.Name] = true
	}
}
