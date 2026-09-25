package edgehost

import (
	"encoding/json"
	"testing"
)

func TestMergePodEnvKeepsExisting(t *testing.T) {
	pod := []byte(`{"apiVersion":"v1","kind":"Pod","spec":{"containers":[{"name":"app","env":[{"name":"A","value":"1"}]}],"initContainers":[{"name":"init"}]}}`)
	out, err := MergePodEnv(pod, map[string]string{"A": "nope", "B": "2"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Spec struct {
			Containers []struct {
				Env []podEnv `json:"env"`
			} `json:"containers"`
			InitContainers []struct {
				Env []podEnv `json:"env"`
			} `json:"initContainers"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Containers) != 1 || len(doc.Spec.Containers[0].Env) != 2 || doc.Spec.Containers[0].Env[0].Value != "1" || doc.Spec.Containers[0].Env[1].Name != "B" {
		t.Fatalf("%s", out)
	}
	init := doc.Spec.InitContainers
	if len(init) != 1 || len(init[0].Env) != 2 || init[0].Env[0].Name != "A" || init[0].Env[0].Value != "nope" || init[0].Env[1].Name != "B" {
		t.Fatalf("%+v", init)
	}
}
