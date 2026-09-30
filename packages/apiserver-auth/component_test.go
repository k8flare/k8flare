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

func TestComponentTokensAuthenticate(t *testing.T) {
	key := []byte("change-me")
	a := ComponentTokens{Key: key}
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
	key := []byte("change-me")
	a := ComponentTokens{Key: key}
	scheduler := MintComponentToken(key, "scheduler")
	for name, token := range map[string]string{
		"other component name":  strings.Replace(scheduler, "scheduler", "gc", 1),
		"unknown component":     MintComponentToken(key, "nope"),
		"wrong key":             MintComponentToken([]byte("other"), "scheduler"),
		"truncated":             scheduler[:len(scheduler)-1],
		"not a component token": "admin",
		"no mac":                "component:scheduler:",
	} {
		if _, ok, err := a.AuthenticateToken(t.Context(), token); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
	if _, ok, _ := (ComponentTokens{}).AuthenticateToken(t.Context(), MintComponentToken(nil, "scheduler")); ok {
		t.Error("empty key authenticated")
	}
}
