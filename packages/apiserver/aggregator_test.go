package apiserver

import "testing"

func TestAPIGroupAndVersion(t *testing.T) {
	cases := []struct {
		path, group, version string
	}{
		{"/apis/wardle.example.com/v1/namespaces/default/flunders", "wardle.example.com", "v1"},
		{"/apis/wardle.example.com/v1", "wardle.example.com", "v1"},
		{"/apis/wardle.example.com", "wardle.example.com", ""},
		{"/apis/apps/v1/deployments", "apps", "v1"},
		{"/api/v1/pods", "", ""},
	}
	for _, tc := range cases {
		g, v := apiGroupAndVersion(tc.path)
		if g != tc.group || v != tc.version {
			t.Fatalf("%s: got %q %q want %q %q", tc.path, g, v, tc.group, tc.version)
		}
	}
}

func TestAPIServiceName(t *testing.T) {
	if got := apiServiceName("wardle.example.com", "v1"); got != "v1.wardle.example.com" {
		t.Fatalf("got %q", got)
	}
	if got := apiServiceName("", "v1"); got != "v1." {
		t.Fatalf("got %q", got)
	}
}
