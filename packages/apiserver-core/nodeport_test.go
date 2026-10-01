package core

import (
	"testing"

	supervisor "github.com/k8flare/k8flare/packages/apiserver-supervisor"
)

func TestNodePortsAreAllocatedFromTheRangeAgentsAreTold(t *testing.T) {
	told := supervisor.ServiceNodePortRange
	if int(nodePortMin) != told.Base || int(nodePortMax) != told.Base+told.Size-1 {
		t.Fatalf("allocating from %d-%d, agents are told %s", nodePortMin, nodePortMax, told.String())
	}
}
