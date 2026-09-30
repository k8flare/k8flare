package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const componentTokenPrefix = "component:"

func serviceAccountController(name string) func() user.Info {
	return func() user.Info {
		return &user.DefaultInfo{
			Name:   "system:serviceaccount:kube-system:" + name,
			Groups: []string{"system:serviceaccounts", "system:serviceaccounts:kube-system", user.AllAuthenticated},
		}
	}
}

func privileged(name string) func() user.Info {
	return func() user.Info {
		return &user.DefaultInfo{Name: name, Groups: []string{user.SystemPrivilegedGroup, user.AllAuthenticated}}
	}
}

var componentIdentities = map[string]func() user.Info{
	"scheduler": func() user.Info {
		return &user.DefaultInfo{Name: user.KubeScheduler, Groups: []string{user.AllAuthenticated}}
	},
	"gc":           serviceAccountController("generic-garbage-collector"),
	"hpa":          serviceAccountController("horizontal-pod-autoscaler"),
	"attachdetach": serviceAccountController("attachdetach-controller"),
	"admission":    privileged("system:apiserver"),
	"workloads":    privileged(user.KubeControllerManager),
	"addons":       privileged("system:k8flare:addons"),
	"hookecho":     privileged("system:k8flare:hookecho"),
	"podkubelet":   privileged("system:k8flare:podkubelet"),
}

func MintComponentToken(key []byte, component string) string {
	return componentTokenPrefix + component + ":" + componentMAC(key, component)
}

func componentMAC(key []byte, component string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("k8flare-component:" + component))
	return hex.EncodeToString(mac.Sum(nil))
}

type ComponentTokens struct{ Key []byte }

func (c ComponentTokens) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	rest, ok := strings.CutPrefix(token, componentTokenPrefix)
	if !ok || len(c.Key) == 0 {
		return nil, false, nil
	}
	component, mac, ok := strings.Cut(rest, ":")
	identity, known := componentIdentities[component]
	if !ok || !known || !hmac.Equal([]byte(mac), []byte(componentMAC(c.Key, component))) {
		return nil, false, nil
	}
	return &authenticator.Response{User: identity()}, true, nil
}
