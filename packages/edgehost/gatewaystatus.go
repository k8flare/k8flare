package edgehost

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

const (
	classPrefix   = "/registry/gateway.networking.k8s.io/gatewayclasses/"
	gatewayPrefix = "/registry/gateway.networking.k8s.io/gateways/"
	routePrefix   = "/registry/gateway.networking.k8s.io/httproutes/"
	grantPrefix   = "/registry/gateway.networking.k8s.io/referencegrants/"

	gatewayAPIPrefix   = "/registry/gateway.networking.k8s.io/"
	ingressPrefix      = "/registry/ingresses/"
	ingressClassPrefix = "/registry/ingressclasses/"
	routeTableKey      = "/k8flare/edge/routes"
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
		if strings.HasPrefix(key, gatewayAPIPrefix) || strings.HasPrefix(key, ingressPrefix) || strings.HasPrefix(key, ingressClassPrefix) || strings.HasPrefix(key, servicePrefix) {
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
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Generation        *int64            `json:"generation"`
	CreationTimestamp string            `json:"creationTimestamp"`
	Annotations       map[string]string `json:"annotations"`
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
	Key  string
	Rev  int64
	Obj  map[string]json.RawMessage
	Meta gwMeta
	Spec struct {
		ControllerName string `json:"controllerName"`
	}
	Status struct {
		Conditions []gwCondition `json:"conditions"`
	}
}

type gwObject struct {
	Key  string
	Rev  int64
	Obj  map[string]json.RawMessage
	Meta gwMeta
	Spec struct {
		GatewayClassName string       `json:"gatewayClassName"`
		Listeners        []gwListener `json:"listeners"`
	}
	Status struct {
		Conditions []gwCondition `json:"conditions"`
	}
}

type gwRoute struct {
	Key  string
	Rev  int64
	Obj  map[string]json.RawMessage
	Meta gwMeta
	Spec struct {
		ParentRefs []gwParentRef `json:"parentRefs"`
		Hostnames  []string      `json:"hostnames"`
		Rules      []gwRouteRule `json:"rules"`
	}
	Status struct {
		Parents []struct {
			ParentRef struct {
				Name        string `json:"name"`
				SectionName string `json:"sectionName"`
			} `json:"parentRef"`
			Conditions []gwCondition `json:"conditions"`
		} `json:"parents"`
	}
}

func provisionGateways(ctx context.Context, store *kine.Client, now string) bool {
	state := edgeState{
		Classes:    listClasses(ctx, store),
		Gateways:   listGateways(ctx, store),
		Routes:     listRoutes(ctx, store),
		Grants:     listGrants(ctx, store),
		IngClasses: listIngressClasses(ctx, store),
		Ingresses:  listIngresses(ctx, store),
	}
	state.Services = loadBackendServices(ctx, store, state)
	result := evaluateEdge(state, now)
	ok := true
	for _, u := range result.Statuses {
		if !writeStatus(ctx, store, u.Key, u.Rev, u.Obj, u.Status) {
			ok = false
		}
	}
	if !writeTable(ctx, store, result.Table) {
		ok = false
	}
	return ok
}

func gatewayKey(gw gwObject) string { return gw.Meta.namespace() + "/" + gw.Meta.Name }

type parentResult struct {
	ref gwParentRef
	gw  *gwObject
	att attachment
}

func evaluateEdge(st edgeState, now string) edgeResult {
	var res edgeResult
	for _, cls := range st.Classes {
		if cls.Spec.ControllerName != gatewayController {
			continue
		}
		status := map[string]any{"conditions": []gwCondition{condition("Accepted", "True", "Accepted", "Handled by k8flare.com/edge", cls.Meta.Generation, now, cls.Status.Conditions)}}
		res.Statuses = append(res.Statuses, statusUpdate{cls.Key, cls.Rev, cls.Obj, status})
	}
	owned := map[string]*gwObject{}
	for i := range st.Gateways {
		gw := &st.Gateways[i]
		if cls := findClass(st.Classes, gw.Spec.GatewayClassName); cls != nil && cls.Spec.ControllerName == gatewayController {
			owned[gatewayKey(*gw)] = gw
		}
	}
	attached := map[string]map[string]int{}
	gwHosts := map[string]map[string]bool{}
	var rules []Rule
	for _, route := range st.Routes {
		var parents []parentResult
		for _, ref := range route.Spec.ParentRefs {
			if (ref.Group != "" && ref.Group != gatewayGroup) || (ref.Kind != "" && ref.Kind != "Gateway") {
				continue
			}
			ns := ref.Namespace
			if ns == "" {
				ns = route.Meta.namespace()
			}
			gw := owned[ns+"/"+ref.Name]
			if gw == nil {
				continue
			}
			att := attach(route, ref, *gw)
			parents = append(parents, parentResult{ref, gw, att})
			if !att.Accepted {
				continue
			}
			key := gatewayKey(*gw)
			if attached[key] == nil {
				attached[key] = map[string]int{}
				gwHosts[key] = map[string]bool{}
			}
			for _, l := range att.Listeners {
				attached[key][l]++
			}
			for _, h := range route.Spec.Hostnames {
				gwHosts[key][h] = true
			}
		}
		if len(parents) == 0 {
			continue
		}
		status, hosts, accepted := routeStatus(route, parents, st, now)
		res.Statuses = append(res.Statuses, statusUpdate{route.Key, route.Rev, route.Obj, status})
		if accepted {
			rules = append(rules, compileRoute(route, hosts, st.Grants, st.Services)...)
		}
	}
	for _, gw := range owned {
		res.Statuses = append(res.Statuses, statusUpdate{gw.Key, gw.Rev, gw.Obj, gatewayStatus(gw, attached[gatewayKey(*gw)], gwHosts[gatewayKey(*gw)], now)})
	}
	for _, ing := range st.Ingresses {
		if !ownsIngress(ing, st.IngClasses) {
			continue
		}
		rules = append(rules, compileIngress(ing)...)
		entries := []map[string]string{}
		for _, h := range ingressHostnames(ing) {
			entries = append(entries, map[string]string{"hostname": h})
		}
		res.Statuses = append(res.Statuses, statusUpdate{ing.Key, ing.Rev, ing.Obj, map[string]any{"loadBalancer": map[string]any{"ingress": entries}}})
	}
	res.Table = Table{Rules: rules}
	res.Table.Prepare()
	if res.Table.Rules == nil {
		res.Table.Rules = []Rule{}
	}
	sort.Slice(res.Statuses, func(i, j int) bool { return res.Statuses[i].Key < res.Statuses[j].Key })
	return res
}

func routeStatus(route gwRoute, parents []parentResult, st edgeState, now string) (status map[string]any, hosts []string, accepted bool) {
	reason, message := resolvedRefs(route, st.Grants, st.Services)
	resolved := "True"
	if reason != "ResolvedRefs" {
		resolved = "False"
	}
	var entries []any
	anyHostless := false
	hostSet := map[string]bool{}
	for _, p := range parents {
		var prev []gwCondition
		for _, old := range route.Status.Parents {
			if old.ParentRef.Name == p.ref.Name && old.ParentRef.SectionName == p.ref.SectionName {
				prev = old.Conditions
			}
		}
		acceptedStatus := "False"
		if p.att.Accepted {
			acceptedStatus = "True"
			accepted = true
			if len(p.att.Hosts) == 0 {
				anyHostless = true
			}
			for _, h := range p.att.Hosts {
				hostSet[h] = true
			}
		}
		ref := map[string]string{"group": gatewayGroup, "kind": "Gateway", "name": p.ref.Name, "namespace": p.gw.Meta.namespace()}
		if p.ref.SectionName != "" {
			ref["sectionName"] = p.ref.SectionName
		}
		entries = append(entries, map[string]any{
			"parentRef":      ref,
			"controllerName": gatewayController,
			"conditions": []gwCondition{
				condition("Accepted", acceptedStatus, p.att.Reason, p.att.Message, route.Meta.Generation, now, prev),
				condition("ResolvedRefs", resolved, reason, message, route.Meta.Generation, now, prev),
			},
		})
	}
	if !anyHostless {
		hosts = sortedKeys(hostSet)
	}
	return map[string]any{"parents": entries}, hosts, accepted
}

func gatewayStatus(gw *gwObject, attached map[string]int, hosts map[string]bool, now string) map[string]any {
	addresses := []any{}
	for _, host := range sortedKeys(hosts) {
		addresses = append(addresses, map[string]string{"type": "Hostname", "value": host})
	}
	listeners := []any{}
	for _, l := range gw.Spec.Listeners {
		kinds := []any{}
		acceptedStatus, acceptedReason, acceptedMessage := "True", "Accepted", "Listener accepted"
		programmedStatus, programmedReason, programmedMessage := "True", "Programmed", "Listener programmed"
		if l.Protocol == "HTTP" || l.Protocol == "HTTPS" {
			kinds = append(kinds, map[string]string{"group": gatewayGroup, "kind": routeKindTag})
		} else {
			acceptedStatus, acceptedReason, acceptedMessage = "False", "UnsupportedProtocol", "only HTTP and HTTPS listeners are served"
			programmedStatus, programmedReason, programmedMessage = "False", "Invalid", "Listener is not accepted"
		}
		listeners = append(listeners, map[string]any{
			"name": l.Name, "attachedRoutes": attached[l.Name],
			"supportedKinds": kinds,
			"conditions": []gwCondition{
				condition("Accepted", acceptedStatus, acceptedReason, acceptedMessage, gw.Meta.Generation, now, nil),
				condition("Programmed", programmedStatus, programmedReason, programmedMessage, gw.Meta.Generation, now, nil),
				condition("ResolvedRefs", "True", "ResolvedRefs", "Listener refs resolved; TLS is terminated by Cloudflare", gw.Meta.Generation, now, nil),
			},
		})
	}
	return map[string]any{
		"addresses": addresses,
		"conditions": []gwCondition{
			condition("Accepted", "True", "Accepted", "GatewayClass accepted", gw.Meta.Generation, now, gw.Status.Conditions),
			condition("Programmed", "True", "Programmed", "Listeners programmed", gw.Meta.Generation, now, gw.Status.Conditions),
		},
		"listeners": listeners,
	}
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

func writeStatus(ctx context.Context, store *kine.Client, key string, rev int64, obj map[string]json.RawMessage, status any) bool {
	encoded, err := json.Marshal(status)
	if err != nil {
		return false
	}
	if statusUnchanged(obj["status"], encoded) {
		return true
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

func listGrants(ctx context.Context, store *kine.Client) []gwGrant {
	var out []gwGrant
	for _, kv := range listKV(ctx, store, grantPrefix) {
		var obj map[string]json.RawMessage
		var item gwGrant
		if !decodeObj(kv.Value, &obj, &item.Meta, &item.Spec, &struct{}{}) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func listIngressClasses(ctx context.Context, store *kine.Client) []ingClass {
	var out []ingClass
	for _, kv := range listKV(ctx, store, ingressClassPrefix) {
		var obj map[string]json.RawMessage
		var item ingClass
		if !decodeObj(kv.Value, &obj, &item.Meta, &item.Spec, &struct{}{}) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func listIngresses(ctx context.Context, store *kine.Client) []ingObject {
	var out []ingObject
	for _, kv := range listKV(ctx, store, ingressPrefix) {
		var item ingObject
		if !decodeObj(kv.Value, &item.Obj, &item.Meta, &item.Spec, &item.Status) {
			continue
		}
		item.Key, item.Rev = kv.Key, kv.ModRevision
		out = append(out, item)
	}
	return out
}

func loadBackendServices(ctx context.Context, store *kine.Client, st edgeState) map[string][]svcPort {
	wanted := map[string]bool{}
	for _, route := range st.Routes {
		for _, rule := range route.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				ns := ref.Namespace
				if ns == "" {
					ns = route.Meta.namespace()
				}
				wanted[serviceKey(ns, ref.Name)] = true
			}
		}
	}
	out := map[string][]svcPort{}
	for key := range wanted {
		kv, _, err := store.Get(ctx, servicePrefix+key)
		if err != nil || kv == nil {
			continue
		}
		var svc struct {
			Spec struct {
				Ports []svcPort `json:"ports"`
			} `json:"spec"`
		}
		if decodeKV(kv.Value, &svc) {
			out[key] = svc.Spec.Ports
		}
	}
	return out
}

func writeTable(ctx context.Context, store *kine.Client, table Table) bool {
	encoded, err := json.Marshal(table)
	if err != nil {
		return false
	}
	kv, _, err := store.Get(ctx, routeTableKey)
	if err == kine.ErrNotFound || (err == nil && kv == nil) {
		_, err = store.Put(ctx, routeTableKey, encoded, 0)
		return err == nil
	}
	if err != nil {
		return false
	}
	if raw, ok := decodeRaw(kv.Value); ok && bytes.Equal(raw, encoded) {
		return true
	}
	_, err = store.Put(ctx, routeTableKey, encoded, kv.ModRevision)
	return err == nil
}

func statusUnchanged(current json.RawMessage, next []byte) bool {
	var a, b any
	if json.Unmarshal(current, &a) != nil || json.Unmarshal(next, &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
