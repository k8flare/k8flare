package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync/atomic"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const componentTokenPrefix = "component:"

const ComponentKeyPath = "/vault/component-key"

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

func unprivileged(name string) func() user.Info {
	return func() user.Info {
		return &user.DefaultInfo{Name: name, Groups: []string{user.AllAuthenticated}}
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
	"nodetunnel":   privileged("system:k8flare:nodetunnel"),
	"queues":       unprivileged("system:k8flare:queues"),
	"nodes":        unprivileged("system:k8flare:nodes"),
}

func MintComponentToken(key []byte, component string) string {
	return componentTokenPrefix + component + ":" + componentMAC(key, component)
}

func componentMAC(key []byte, component string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("k8flare-component:" + component))
	return hex.EncodeToString(mac.Sum(nil))
}

type ComponentTokens struct {
	Store *kine.Client
	key   atomic.Pointer[[]byte]
}

func (c *ComponentTokens) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	rest, ok := strings.CutPrefix(token, componentTokenPrefix)
	if !ok {
		return nil, false, nil
	}
	component, mac, ok := strings.Cut(rest, ":")
	identity, known := componentIdentities[component]
	if !ok || !known {
		return nil, false, nil
	}
	key, err := c.componentKey(ctx)
	if err == kine.ErrNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !hmac.Equal([]byte(mac), []byte(componentMAC(key, component))) {
		return nil, false, nil
	}
	return &authenticator.Response{User: identity()}, true, nil
}

func (c *ComponentTokens) componentKey(ctx context.Context) ([]byte, error) {
	if key := c.key.Load(); key != nil {
		return *key, nil
	}
	kv, _, err := c.Store.Get(ctx, ComponentKeyPath)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil, err
	}
	c.key.Store(&key)
	return key, nil
}
