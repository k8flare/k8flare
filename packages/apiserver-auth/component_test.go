package auth

import (
	"strings"
	"testing"

	"k8s.io/apiserver/pkg/authentication/user"
)

func TestMintComponentTokenKnownAnswer(t *testing.T) {
	got := MintComponentToken([]byte("change-me"), "scheduler")
	want := "component:scheduler:46f4359322a0658846a0540bcbd9a96a89fcbc914d6489f1597b275c40bbcfb2"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func storedComponentKey(key string) *ComponentTokens {
	client, m := newMemoryKine()
	if key != "" {
		m.data[ComponentKeyPath] = []byte(key)
	}
	return &ComponentTokens{Store: client}
}

func TestComponentTokensAuthenticate(t *testing.T) {
	key := []byte("stored-component-key")
	a := storedComponentKey(string(key))
	cases := map[string]struct {
		name   string
		groups []string
	}{
		"scheduler":    {"system:kube-scheduler", []string{user.AllAuthenticated}},
		"gc":           {"system:serviceaccount:kube-system:generic-garbage-collector", []string{"system:serviceaccounts", "system:serviceaccounts:kube-system", user.AllAuthenticated}},
		"hpa":          {"system:serviceaccount:kube-system:horizontal-pod-autoscaler", []string{"system:serviceaccounts", "system:serviceaccounts:kube-system", user.AllAuthenticated}},
		"attachdetach": {"system:serviceaccount:kube-system:attachdetach-controller", []string{"system:serviceaccounts", "system:serviceaccounts:kube-system", user.AllAuthenticated}},
		"admission":    {"system:apiserver", []string{user.SystemPrivilegedGroup, user.AllAuthenticated}},
		"workloads":    {"system:kube-controller-manager", []string{user.SystemPrivilegedGroup, user.AllAuthenticated}},
		"queues":       {"system:k8flare:queues", []string{user.AllAuthenticated}},
		"nodes":        {"system:k8flare:nodes", []string{user.AllAuthenticated}},
		"nodetunnel":   {"system:k8flare:nodetunnel", []string{user.SystemPrivilegedGroup, user.AllAuthenticated}},
	}
	for component, want := range cases {
		resp, ok, err := a.AuthenticateToken(t.Context(), MintComponentToken(key, component))
		if err != nil || !ok {
			t.Fatalf("%s: ok=%v err=%v", component, ok, err)
		}
		if resp.User.GetName() != want.name || strings.Join(resp.User.GetGroups(), ",") != strings.Join(want.groups, ",") {
			t.Errorf("%s: %s %v", component, resp.User.GetName(), resp.User.GetGroups())
		}
	}
}

func TestComponentTokensReject(t *testing.T) {
	key := []byte("stored-component-key")
	a := storedComponentKey(string(key))
	scheduler := MintComponentToken(key, "scheduler")
	for name, token := range map[string]string{
		"other component name":  strings.Replace(scheduler, "scheduler", "gc", 1),
		"unknown component":     MintComponentToken(key, "nope"),
		"wrong key":             MintComponentToken([]byte("other"), "scheduler"),
		"keyed by admin token":  MintComponentToken([]byte("admin-token"), "scheduler"),
		"truncated":             scheduler[:len(scheduler)-1],
		"not a component token": "admin",
		"no mac":                "component:scheduler:",
	} {
		if _, ok, err := a.AuthenticateToken(t.Context(), token); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
	if _, ok, err := storedComponentKey("").AuthenticateToken(t.Context(), MintComponentToken(nil, "scheduler")); ok || err != nil {
		t.Errorf("no stored key: ok=%v err=%v", ok, err)
	}
}

func TestComponentTokensReadTheKeyOnceItExists(t *testing.T) {
	client, m := newMemoryKine()
	a := &ComponentTokens{Store: client}
	token := MintComponentToken([]byte("k1"), "scheduler")
	if _, ok, _ := a.AuthenticateToken(t.Context(), token); ok {
		t.Fatal("authenticated before the key exists")
	}
	m.data[ComponentKeyPath] = []byte("k1")
	if _, ok, err := a.AuthenticateToken(t.Context(), token); !ok || err != nil {
		t.Fatalf("after the key was stored: ok=%v err=%v", ok, err)
	}
	delete(m.data, ComponentKeyPath)
	if _, ok, err := a.AuthenticateToken(t.Context(), token); !ok || err != nil {
		t.Fatalf("cached key: ok=%v err=%v", ok, err)
	}
}
