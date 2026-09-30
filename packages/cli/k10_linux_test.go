//go:build linux

package main

import (
	"testing"

	"github.com/k3s-io/k3s/pkg/clientaccess"
)

func TestK10TokenMatchesK3sClientAccess(t *testing.T) {
	f := newFakeCluster(t)
	f.respond["POST /internal/tokens"] = `{"id":"abc123","token":"abc123.0123456789abcdef"}`
	out, err := f.run(t, "token", "create")
	if err != nil {
		t.Fatal(err)
	}
	want, err := clientaccess.FormatTokenBytes("abc123.0123456789abcdef", []byte(f.respond["GET /cacerts"]))
	if err != nil {
		t.Fatal(err)
	}
	if out != want+"\n" {
		t.Fatalf("output %q, k3s formats %q", out, want)
	}
}
