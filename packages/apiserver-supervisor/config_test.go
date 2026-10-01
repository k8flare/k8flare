//go:build !js

package supervisor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k3s-io/k3s/pkg/daemons/config"
)

func TestConfigLetsTheAgentStartTheNetworkPolicyController(t *testing.T) {
	s := New(NewVault(fakeKine(t)), "join")
	mux := http.NewServeMux()
	s.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/v1-k3s/config", nil)
	req.Header.Set("Authorization", "Bearer join")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var control config.Control
	if err := json.Unmarshal(rr.Body.Bytes(), &control); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("config: %d %v %s", rr.Code, err, rr.Body.String())
	}
	if control.DisableNPC {
		t.Fatal("the network policy controller is disabled")
	}
	if control.ServiceIPRange == nil || control.ServiceNodePortRange == nil || control.ServiceNodePortRange.Size == 0 {
		t.Fatalf("the agent rejects this config as a down-level server: ServiceIPRange=%v ServiceNodePortRange=%v", control.ServiceIPRange, control.ServiceNodePortRange)
	}
	if got := control.ServiceNodePortRange.String(); got != "30000-32767" {
		t.Fatalf("node port range: %s", got)
	}
}
