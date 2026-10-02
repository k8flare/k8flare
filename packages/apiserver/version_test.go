package apiserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestVersionEndpointReportsMirroredKubernetesVersion(t *testing.T) {
	want := version.Info{Major: "1", Minor: "36", GitVersion: "v1.36.4+k8flare", Platform: "js/wasm", GoVersion: "go1.26", Compiler: "gc"}
	if versionInfo != want {
		t.Fatalf("versionInfo %+v want %+v", versionInfo, want)
	}
	rec := adminGet(handlerWithDisabled(t, ""), "/version", "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("/version: %d %s", rec.Code, rec.Body.String())
	}
	var got version.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("/version %+v want %+v", got, want)
	}
}
