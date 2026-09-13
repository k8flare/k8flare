//go:build !js

package apiserver_test

import (
	"encoding/json"
	"testing"
)

// TestOpenAPI fetches the documents the way kubectl does: v2 as protobuf
// through the discovery client, and v3 per group through the v3 client.
func TestOpenAPI(t *testing.T) {
	cs := startDev(t)
	v2, err := cs.Discovery().OpenAPISchema()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, def := range v2.GetDefinitions().GetAdditionalProperties() {
		if def.GetName() == "io.k8s.api.core.v1.Pod" {
			found = true
		}
	}
	if !found {
		t.Error("v2 lacks io.k8s.api.core.v1.Pod")
	}
	paths, err := cs.Discovery().OpenAPIV3().Paths()
	if err != nil {
		t.Fatal(err)
	}
	gv, ok := paths["apis/coordination.k8s.io/v1"]
	if !ok {
		t.Fatalf("v3 root lacks apis/coordination.k8s.io/v1; got %d groups", len(paths))
	}
	raw, err := gv.Schema("application/json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Components.Schemas["io.k8s.api.coordination.v1.Lease"]; !ok {
		t.Error("v3 coordination doc lacks the Lease schema")
	}
}
