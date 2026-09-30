package edgehost

import (
	"encoding/json"
	"testing"
)

func decodeStatus(t *testing.T, res edgeResult, key string) map[string]any {
	t.Helper()
	for _, u := range res.Statuses {
		if u.Key == key {
			raw, _ := json.Marshal(u.Status)
			var out map[string]any
			_ = json.Unmarshal(raw, &out)
			return out
		}
	}
	t.Fatalf("no status for %s", key)
	return nil
}

func condOf(t *testing.T, conds any, typ string) map[string]any {
	t.Helper()
	for _, c := range conds.([]any) {
		m := c.(map[string]any)
		if m["type"] == typ {
			return m
		}
	}
	t.Fatalf("no %s condition", typ)
	return nil
}

func edgeFixture() edgeState {
	var st edgeState
	st.Classes = []gwClass{{Key: "c", Meta: gwMeta{Name: "k8flare"}}}
	st.Classes[0].Spec.ControllerName = gatewayController
	gw := gwObject{Key: "gw", Meta: gwMeta{Name: "edge", Namespace: "infra"}}
	gw.Spec.GatewayClassName = "k8flare"
	gw.Spec.Listeners = []gwListener{{Name: "http", Protocol: "HTTP", Hostname: "*.example.com"}}
	st.Gateways = []gwObject{gw}
	st.Services = map[string][]svcPort{"infra/web": {{Name: "http", Port: 80}}}
	return st
}

func routeIn(ns, name string, hostnames []string, refNS string, backendPort int32) gwRoute {
	r := gwRoute{Key: "route/" + ns + "/" + name, Meta: gwMeta{Name: name, Namespace: ns, CreationTimestamp: "2024-01-01T00:00:00Z"}, Obj: map[string]json.RawMessage{}}
	r.Spec.ParentRefs = []gwParentRef{{Name: "edge", Namespace: "infra"}}
	r.Spec.Hostnames = hostnames
	port := backendPort
	r.Spec.Rules = []gwRouteRule{{BackendRefs: []gwBackendRef{{Name: "web", Namespace: refNS, Port: &port}}}}
	return r
}

func TestRouteFromAnotherNamespaceIsNotAllowedByDefaultListener(t *testing.T) {
	st := edgeFixture()
	st.Routes = []gwRoute{routeIn("apps", "r", []string{"a.example.com"}, "infra", 80)}
	res := evaluateEdge(st, "now")
	parents := decodeStatus(t, res, "route/apps/r")["parents"].([]any)
	acc := condOf(t, parents[0].(map[string]any)["conditions"], "Accepted")
	if acc["status"] != "False" || acc["reason"] != "NotAllowedByListeners" {
		t.Fatalf("%v", acc)
	}
	if len(res.Table.Rules) != 0 {
		t.Fatalf("%+v", res.Table.Rules)
	}
}

func TestRouteInAllowedNamespaceIsAcceptedAndCompiled(t *testing.T) {
	st := edgeFixture()
	st.Gateways[0].Spec.Listeners[0].AllowedRoutes.Namespaces.From = "All"
	st.Routes = []gwRoute{routeIn("infra", "r", []string{"a.example.com", "b.other.org"}, "", 80)}
	res := evaluateEdge(st, "now")
	parents := decodeStatus(t, res, "route/infra/r")["parents"].([]any)
	conds := parents[0].(map[string]any)["conditions"]
	if condOf(t, conds, "Accepted")["status"] != "True" || condOf(t, conds, "ResolvedRefs")["status"] != "True" {
		t.Fatalf("%v", conds)
	}
	if len(res.Table.Rules) != 1 || res.Table.Rules[0].Host != "a.example.com" {
		t.Fatalf("%+v", res.Table.Rules)
	}
	gwStatus := decodeStatus(t, res, "gw")
	listener := gwStatus["listeners"].([]any)[0].(map[string]any)
	if listener["attachedRoutes"].(float64) != 1 {
		t.Fatalf("%v", listener)
	}
	if got := gwStatus["addresses"].([]any); len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestRouteWithoutHostnamesInheritsListenerHostname(t *testing.T) {
	st := edgeFixture()
	st.Routes = []gwRoute{routeIn("infra", "r", nil, "", 80)}
	res := evaluateEdge(st, "now")
	if len(res.Table.Rules) != 1 || res.Table.Rules[0].Host != "*.example.com" {
		t.Fatalf("%+v", res.Table.Rules)
	}
}

func TestRouteSectionNameMismatch(t *testing.T) {
	st := edgeFixture()
	route := routeIn("infra", "r", nil, "", 80)
	route.Spec.ParentRefs[0].SectionName = "other"
	st.Routes = []gwRoute{route}
	parents := decodeStatus(t, evaluateEdge(st, "now"), "route/infra/r")["parents"].([]any)
	if acc := condOf(t, parents[0].(map[string]any)["conditions"], "Accepted"); acc["reason"] != "NoMatchingParent" {
		t.Fatalf("%v", acc)
	}
}

func TestResolvedRefsReasons(t *testing.T) {
	st := edgeFixture()
	missing := routeIn("infra", "missing", nil, "", 80)
	missing.Spec.Rules[0].BackendRefs[0].Name = "nope"
	badPort := routeIn("infra", "port", nil, "", 9999)
	badKind := routeIn("infra", "kind", nil, "", 80)
	badKind.Spec.Rules[0].BackendRefs[0].Kind = "Bucket"
	crossNS := routeIn("infra", "cross", nil, "other", 80)
	st.Services["other/web"] = []svcPort{{Name: "http", Port: 80}}
	mirror := routeIn("infra", "mirror", nil, "", 80)
	mirror.Spec.Rules[0].Filters = []Filter{{Type: "RequestMirror"}}
	st.Routes = []gwRoute{missing, badPort, badKind, crossNS, mirror}
	res := evaluateEdge(st, "now")
	want := map[string]string{
		"route/infra/missing": "BackendNotFound",
		"route/infra/port":    "BackendNotFound",
		"route/infra/kind":    "InvalidKind",
		"route/infra/cross":   "RefNotPermitted",
		"route/infra/mirror":  "UnsupportedValue",
	}
	for key, reason := range want {
		parents := decodeStatus(t, res, key)["parents"].([]any)
		rr := condOf(t, parents[0].(map[string]any)["conditions"], "ResolvedRefs")
		if rr["status"] != "False" || rr["reason"] != reason {
			t.Fatalf("%s: %v", key, rr)
		}
	}
}

func TestReferenceGrantPermitsCrossNamespaceBackend(t *testing.T) {
	st := edgeFixture()
	st.Services["other/web"] = []svcPort{{Name: "http", Port: 80}}
	grant := gwGrant{Meta: gwMeta{Name: "g", Namespace: "other"}}
	grant.Spec.From = []struct {
		Group     string `json:"group"`
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
	}{{Group: gatewayGroup, Kind: "HTTPRoute", Namespace: "infra"}}
	grant.Spec.To = []struct {
		Group string `json:"group"`
		Kind  string `json:"kind"`
		Name  string `json:"name"`
	}{{Kind: "Service"}}
	st.Grants = []gwGrant{grant}
	st.Routes = []gwRoute{routeIn("infra", "cross", nil, "other", 80)}
	res := evaluateEdge(st, "now")
	parents := decodeStatus(t, res, "route/infra/cross")["parents"].([]any)
	if rr := condOf(t, parents[0].(map[string]any)["conditions"], "ResolvedRefs"); rr["status"] != "True" {
		t.Fatalf("%v", rr)
	}
	if len(res.Table.Rules) != 1 || res.Table.Rules[0].Backends[0].Namespace != "other" {
		t.Fatalf("%+v", res.Table.Rules)
	}
}

func TestRouteOfAnotherControllerIsIgnored(t *testing.T) {
	st := edgeFixture()
	st.Classes[0].Spec.ControllerName = "example.com/other"
	st.Routes = []gwRoute{routeIn("infra", "r", nil, "", 80)}
	res := evaluateEdge(st, "now")
	if len(res.Statuses) != 0 || len(res.Table.Rules) != 0 {
		t.Fatalf("%+v %+v", res.Statuses, res.Table)
	}
}

func TestUnsupportedListenerProtocol(t *testing.T) {
	st := edgeFixture()
	st.Gateways[0].Spec.Listeners = append(st.Gateways[0].Spec.Listeners, gwListener{Name: "tcp", Protocol: "TCP"})
	listeners := decodeStatus(t, evaluateEdge(st, "now"), "gw")["listeners"].([]any)
	acc := condOf(t, listeners[1].(map[string]any)["conditions"], "Accepted")
	if acc["status"] != "False" || acc["reason"] != "UnsupportedProtocol" {
		t.Fatalf("%v", acc)
	}
}

func ingressIn(name, class string) ingObject {
	ing := ingObject{Key: "ing/" + name, Meta: gwMeta{Name: name, Namespace: "default"}, Obj: map[string]json.RawMessage{}}
	ing.Spec.IngressClassName = &class
	ing.Spec.Rules = []ingRule{{Host: "shop.example.com", HTTP: &ingHTTP{Paths: []ingPath{{Path: "/", PathType: "Prefix", Backend: ingBackend{Service: &ingService{Name: "web", Port: ingPort{Name: "http"}}}}}}}}
	return ing
}

func TestIngressOwnershipStatusAndTable(t *testing.T) {
	var st edgeState
	own := ingClass{Meta: gwMeta{Name: "k8flare"}}
	own.Spec.Controller = gatewayController
	foreign := ingClass{Meta: gwMeta{Name: "nginx"}}
	foreign.Spec.Controller = "k8s.io/ingress-nginx"
	st.IngClasses = []ingClass{own, foreign}
	viaAnnotation := ingressIn("legacy", "")
	viaAnnotation.Spec.IngressClassName = nil
	viaAnnotation.Meta.Annotations = map[string]string{"kubernetes.io/ingress.class": "k8flare"}
	st.Ingresses = []ingObject{ingressIn("mine", "k8flare"), ingressIn("theirs", "nginx"), ingressIn("classless", ""), viaAnnotation}
	res := evaluateEdge(st, "now")
	if len(res.Statuses) != 2 {
		t.Fatalf("%d", len(res.Statuses))
	}
	lb := decodeStatus(t, res, "ing/mine")["loadBalancer"].(map[string]any)["ingress"].([]any)
	if len(lb) != 1 || lb[0].(map[string]any)["hostname"] != "shop.example.com" {
		t.Fatalf("%v", lb)
	}
	if len(res.Table.Rules) != 2 {
		t.Fatalf("%+v", res.Table.Rules)
	}
}

func TestIngressWithoutHostAdvertisesNoAddress(t *testing.T) {
	ing := ingressIn("bare", "k8flare")
	ing.Spec.Rules[0].Host = ""
	if got := ingressHostnames(ing); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestSecondPassWritesNothing(t *testing.T) {
	st := edgeFixture()
	st.Gateways[0].Spec.Listeners[0].AllowedRoutes.Namespaces.From = "All"
	st.Routes = []gwRoute{routeIn("apps", "r", []string{"a.example.com"}, "infra", 80)}
	own := ingClass{Meta: gwMeta{Name: "k8flare"}}
	own.Spec.Controller = gatewayController
	st.IngClasses = []ingClass{own}
	st.Ingresses = []ingObject{ingressIn("web", "k8flare")}
	first := evaluateEdge(st, "t1")
	for _, u := range first.Statuses {
		raw, _ := json.Marshal(u.Status)
		switch {
		case u.Key == "c":
			_ = json.Unmarshal(raw, &st.Classes[0].Status)
			st.Classes[0].Obj = map[string]json.RawMessage{"status": raw}
		case u.Key == "gw":
			_ = json.Unmarshal(raw, &st.Gateways[0].Status)
			st.Gateways[0].Obj = map[string]json.RawMessage{"status": raw}
		case u.Key == st.Routes[0].Key:
			_ = json.Unmarshal(raw, &st.Routes[0].Status)
			st.Routes[0].Obj = map[string]json.RawMessage{"status": raw}
		case u.Key == st.Ingresses[0].Key:
			st.Ingresses[0].Obj = map[string]json.RawMessage{"status": raw}
		}
	}
	second := evaluateEdge(st, "t2")
	if len(second.Statuses) != len(first.Statuses) || len(first.Statuses) != 4 {
		t.Fatalf("%d %d", len(first.Statuses), len(second.Statuses))
	}
	for _, u := range second.Statuses {
		raw, _ := json.Marshal(u.Status)
		if !statusUnchanged(u.Obj["status"], raw) {
			t.Fatalf("%s rewrites on the second pass:\n%s\n%s", u.Key, u.Obj["status"], raw)
		}
	}
}

func TestStatusUnchangedIgnoresKeyOrder(t *testing.T) {
	if !statusUnchanged(json.RawMessage(`{"b":1,"a":[{"x":"y"}]}`), []byte(`{"a":[{"x":"y"}],"b":1}`)) {
		t.Fatal("same")
	}
	if statusUnchanged(json.RawMessage(`{"a":1}`), []byte(`{"a":2}`)) {
		t.Fatal("different")
	}
	if statusUnchanged(nil, []byte(`{"a":2}`)) {
		t.Fatal("absent")
	}
}
