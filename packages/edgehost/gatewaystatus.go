package edgehost

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

const (
	classPrefix   = "/registry/gateway.networking.k8s.io/gatewayclasses/"
	gatewayPrefix = "/registry/gateway.networking.k8s.io/gateways/"
	routePrefix   = "/registry/gateway.networking.k8s.io/httproutes/"
)

func ProvisionGateways(w http.ResponseWriter, r *http.Request, store *kine.Client) {
	if store == nil {
		http.Error(w, "storage unavailable", http.StatusBadGateway)
		return
	}
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	hit := false
	for _, key := range body.Keys {
		if strings.HasPrefix(key, classPrefix) || strings.HasPrefix(key, gatewayPrefix) || strings.HasPrefix(key, routePrefix) {
			hit = true
			break
		}
	}
	if !hit {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !provisionGateways(r.Context(), store, time.Now().UTC().Format(time.RFC3339Nano)) {
		http.Error(w, "status update failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type gwMeta struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Generation *int64 `json:"generation"`
}

type gwCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
}

type gwClass struct {
	Key string
	Rev int64
	Obj map[string]json.RawMessage
	Meta gwMeta
	Spec struct {
		ControllerName string `json:"controllerName"`
	}
	Status struct {
		Conditions []gwCondition `json:"conditions"`
	}
}

type gwObject struct {
	Key string
	Rev int64
	Obj map[string]json.RawMessage
	Meta gwMeta
	Spec struct {
		GatewayClassName string `json:"gatewayClassName"`
		Listeners        []struct {
			Name string `json:"name"`
		} `json:"listeners"`
	}
	Status struct {
		Conditions []gwCondition `json:"conditions"`
	}
}

type gwRoute struct {
	Key string
	Rev int64
	Obj map[string]json.RawMessage
	Meta gwMeta
	Spec struct {
		ParentRefs []struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Group     string `json:"group"`
			Kind      string `json:"kind"`
		} `json:"parentRefs"`
		Hostnames []string `json:"hostnames"`
	}
	Status struct {
		Parents []struct {
			ParentRef  struct{ Name string `json:"name"` } `json:"parentRef"`
			Conditions []gwCondition                    `json:"conditions"`
		} `json:"parents"`
	}
}

func provisionGateways(ctx context.Context, store *kine.Client, now string) bool {
	classes := listClasses(ctx, store)
	gateways := listGateways(ctx, store)
	routes := listRoutes(ctx, store)
	ok := true
	for _, cls := range classes {
		if cls.Spec.ControllerName != gatewayController {
			continue
		}
		status := map[string]any{"conditions": []gwCondition{condition("Accepted", "True", "Accepted", "Handled by k8flare.com/edge", cls.Meta.Generation, now, cls.Status.Conditions)}}
		if !writeStatus(ctx, store, cls.Key, cls.Rev, cls.Obj, status) {
			ok = false
		}
	}
	for _, gw := range gateways {
		cls := findClass(classes, gw.Spec.GatewayClassName)
		if cls == nil || cls.Spec.ControllerName != gatewayController {
			continue
		}
		var attached []gwRoute
		for _, route := range routes {
			if routeParents(route, gw) {
				attached = append(attached, route)
			}
		}
		hosts := map[string]bool{}
		var addresses []any
		for _, route := range attached {
			for _, host := range route.Spec.Hostnames {
				if hosts[host] {
					continue
				}
				hosts[host] = true
				addresses = append(addresses, map[string]string{"type": "Hostname", "value": host})
			}
		}
		if addresses == nil {
			addresses = []any{}
		}
		listeners := []any{}
		for _, l := range gw.Spec.Listeners {
			listeners = append(listeners, map[string]any{
				"name": l.Name, "attachedRoutes": len(attached),
				"supportedKinds": []any{map[string]string{"group": "gateway.networking.k8s.io", "kind": "HTTPRoute"}},
				"conditions": []gwCondition{
					condition("Accepted", "True", "Accepted", "Listener accepted", gw.Meta.Generation, now, nil),
					condition("Programmed", "True", "Programmed", "Listener programmed", gw.Meta.Generation, now, nil),
					condition("ResolvedRefs", "True", "ResolvedRefs", "Listener refs resolved", gw.Meta.Generation, now, nil),
				},
			})
		}
		status := map[string]any{
			"addresses": addresses,
			"conditions": []gwCondition{
				condition("Accepted", "True", "Accepted", "GatewayClass accepted", gw.Meta.Generation, now, gw.Status.Conditions),
				condition("Programmed", "True", "Programmed", "Listeners programmed", gw.Meta.Generation, now, gw.Status.Conditions),
			},
			"listeners": listeners,
		}
		if !writeStatus(ctx, store, gw.Key, gw.Rev, gw.Obj, status) {
			ok = false
		}
	}
	for _, route := range routes {
		parent := firstOwnedParent(route, gateways, classes)
		if parent == nil {
			continue
		}
		var pref struct {
			Name, Namespace, Group, Kind string
		}
		for _, p := range route.Spec.ParentRefs {
			if p.Name == parent.Meta.Name {
				pref.Name, pref.Namespace, pref.Group, pref.Kind = p.Name, p.Namespace, p.Group, p.Kind
				break
			}
		}
		if pref.Name == "" {
			pref.Name = parent.Meta.Name
		}
		if pref.Group == "" {
			pref.Group = "gateway.networking.k8s.io"
		}
		if pref.Kind == "" {
			pref.Kind = "Gateway"
		}
		if pref.Namespace == "" {
			pref.Namespace = parent.Meta.Namespace
		}
		var prev []gwCondition
		for _, p := range route.Status.Parents {
			if p.ParentRef.Name == parent.Meta.Name {
				prev = p.Conditions
			}
		}
		status := map[string]any{"parents": []any{map[string]any{
			"parentRef":      map[string]string{"group": pref.Group, "kind": pref.Kind, "name": pref.Name, "namespace": pref.Namespace},
			"controllerName": gatewayController,
			"conditions": []gwCondition{
				condition("Accepted", "True", "Accepted", "Route attached", route.Meta.Generation, now, prev),
				condition("ResolvedRefs", "True", "ResolvedRefs", "Backend refs resolved", route.Meta.Generation, now, prev),
			},
		}}}
		if !writeStatus(ctx, store, route.Key, route.Rev, route.Obj, status) {
			ok = false
		}
	}
	return ok
}

func condition(typ, status, reason, message string, gen *int64, now string, prev []gwCondition) gwCondition {
	c := gwCondition{Type: typ, Status: status, Reason: reason, Message: message, LastTransitionTime: now, ObservedGeneration: gen}
	for _, old := range prev {
		if old.Type == typ && old.Status == status && old.Reason == reason && old.Message == message && sameGen(old.ObservedGeneration, gen) {
			c.LastTransitionTime = old.LastTransitionTime
		}
	}
	return c
}

func sameGen(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func routeParents(route gwRoute, gw gwObject) bool {
	if gw.Meta.Name == "" {
		return false
	}
	ns := gw.Meta.Namespace
	if ns == "" {
		ns = "default"
	}
	for _, p := range route.Spec.ParentRefs {
		pns := p.Namespace
		if pns == "" {
			pns = route.Meta.Namespace
		}
		if pns == "" {
			pns = "default"
		}
		if p.Name == gw.Meta.Name && pns == ns {
			return true
		}
	}
	return false
}

func firstOwnedParent(route gwRoute, gateways []gwObject, classes []gwClass) *gwObject {
	for _, p := range route.Spec.ParentRefs {
		if p.Name == "" {
			continue
		}
		ns := p.Namespace
		if ns == "" {
			ns = route.Meta.Namespace
		}
		if ns == "" {
			ns = "default"
		}
		for i := range gateways {
			gwns := gateways[i].Meta.Namespace
			if gwns == "" {
				gwns = "default"
			}
			if gateways[i].Meta.Name == p.Name && gwns == ns {
				if cls := findClass(classes, gateways[i].Spec.GatewayClassName); cls != nil && cls.Spec.ControllerName == gatewayController {
					return &gateways[i]
				}
			}
		}
	}
	return nil
}

func findClass(classes []gwClass, name string) *gwClass {
	for i := range classes {
		if classes[i].Meta.Name == name {
			return &classes[i]
		}
	}
	return nil
}

func writeStatus(ctx context.Context, store *kine.Client, key string, rev int64, obj map[string]json.RawMessage, status any) bool {
	encoded, err := json.Marshal(status)
	if err != nil {
		return false
	}
	obj["status"] = encoded
	out, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	_, err = store.Put(ctx, key, out, rev)
	return err == nil
}

func listClasses(ctx context.Context, store *kine.Client) []gwClass {
	var out []gwClass
	for _, kv := range listKV(ctx, store, classPrefix) {
		var item gwClass
		if !decodeObj(kv.Value, &item.Obj, &item.Meta, &item.Spec, &item.Status) {
			continue
		}
		item.Key, item.Rev = kv.Key, kv.ModRevision
		out = append(out, item)
	}
	return out
}

func listGateways(ctx context.Context, store *kine.Client) []gwObject {
	var out []gwObject
	for _, kv := range listKV(ctx, store, gatewayPrefix) {
		var item gwObject
		if !decodeObj(kv.Value, &item.Obj, &item.Meta, &item.Spec, &item.Status) {
			continue
		}
		item.Key, item.Rev = kv.Key, kv.ModRevision
		out = append(out, item)
	}
	return out
}

func listRoutes(ctx context.Context, store *kine.Client) []gwRoute {
	var out []gwRoute
	for _, kv := range listKV(ctx, store, routePrefix) {
		var item gwRoute
		if !decodeObj(kv.Value, &item.Obj, &item.Meta, &item.Spec, &item.Status) {
			continue
		}
		item.Key, item.Rev = kv.Key, kv.ModRevision
		out = append(out, item)
	}
	return out
}

func listKV(ctx context.Context, store *kine.Client, prefix string) []kine.KV {
	kvs, _, _, err := store.List(ctx, prefix, "", 500)
	if err != nil {
		return nil
	}
	return kvs
}

func decodeObj(value string, obj *map[string]json.RawMessage, meta, spec, status any) bool {
	raw, ok := decodeRaw(value)
	if !ok || json.Unmarshal(raw, obj) != nil {
		return false
	}
	_ = json.Unmarshal((*obj)["metadata"], meta)
	_ = json.Unmarshal((*obj)["spec"], spec)
	_ = json.Unmarshal((*obj)["status"], status)
	return true
}
