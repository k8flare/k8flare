package upstream

import (
	"strings"
	"testing"
)

func TestParseVersions(t *testing.T) {
	got, err := parseVersions("versions.mod", []byte("module m\n\nrequire (\n\texample.com/a v1.2.3\n\texample.com/b v0.0.0-20250101000000-abcdef123456\n)\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"example.com/a": "v1.2.3", "example.com/b": "v0.0.0-20250101000000-abcdef123456"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for module, version := range want {
		if got[module] != version {
			t.Fatalf("%s: got %q want %q", module, got[module], version)
		}
	}
}

func TestParseVersionsRejectsDuplicates(t *testing.T) {
	_, err := parseVersions("versions.mod", []byte("module m\n\nrequire (\n\texample.com/a v1.2.3\n\texample.com/a v1.2.4\n)\n"))
	if err == nil || !strings.Contains(err.Error(), "example.com/a") {
		t.Fatalf("got %v", err)
	}
}

func TestVersionOfNamesMissingModule(t *testing.T) {
	_, err := VersionOf("example.com/missing")
	if err == nil || !strings.Contains(err.Error(), "example.com/missing") {
		t.Fatalf("got %v", err)
	}
}

func TestEmbeddedVersionsCarryKubernetes(t *testing.T) {
	version, err := VersionOf(Module)
	if err != nil {
		t.Fatal(err)
	}
	if version != Version {
		t.Fatalf("got %q want %q", version, Version)
	}
}
