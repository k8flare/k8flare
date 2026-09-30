package edgehost

import (
	"net/http"
	"net/url"
	"testing"
)

func lookup(t *Table, host, method, path string, header http.Header, query string) *Rule {
	q, _ := url.ParseQuery(query)
	return t.Lookup(host, method, path, header, q)
}

func backendNamed(name string) []Backend {
	return []Backend{{Namespace: "default", Name: name, Port: 80, Weight: 1}}
}

func TestSortRulesHostSpecificityThenPath(t *testing.T) {
	table := &Table{Rules: []Rule{
		{Source: "a", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Backends: backendNamed("any")},
		{Source: "b", Host: "*.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Backends: backendNamed("wild")},
		{Source: "c", Host: "app.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Backends: backendNamed("exact")},
		{Source: "d", Host: "app.example.com", Path: &pathMatch{Type: "PathPrefix", Value: "/api"}, Backends: backendNamed("api")},
		{Source: "e", Host: "app.example.com", Path: &pathMatch{Type: "Exact", Value: "/api"}, Backends: backendNamed("apiexact")},
	}}
	table.Prepare()
	cases := []struct{ host, path, want string }{
		{"app.example.com", "/api", "apiexact"},
		{"app.example.com", "/api/x", "api"},
		{"app.example.com", "/other", "exact"},
		{"x.example.com", "/other", "wild"},
		{"other.org", "/", "any"},
	}
	for _, c := range cases {
		rule := lookup(table, c.host, "GET", c.path, http.Header{}, "")
		if rule == nil || rule.Backends[0].Name != c.want {
			t.Fatalf("%s%s: got %+v want %s", c.host, c.path, rule, c.want)
		}
	}
}

func TestSortRulesGatewayTieBreakers(t *testing.T) {
	prefix := &pathMatch{Type: "PathPrefix", Value: "/x"}
	table := &Table{Rules: []Rule{
		{Source: "HTTPRoute/default/z", Created: "2024-01-01T00:00:00Z", Path: prefix, Backends: backendNamed("plain")},
		{Source: "HTTPRoute/default/y", Created: "2024-01-01T00:00:00Z", Path: prefix, Method: "GET", Backends: backendNamed("method")},
		{Source: "HTTPRoute/default/w", Created: "2024-01-01T00:00:00Z", Path: prefix, Method: "GET", Headers: []ValueMatch{{Name: "X-A", Value: "1"}}, Backends: backendNamed("header")},
		{Source: "HTTPRoute/default/v", Created: "2024-01-01T00:00:00Z", Path: prefix, Method: "GET", Headers: []ValueMatch{{Name: "X-A", Value: "1"}}, Query: []ValueMatch{{Name: "q", Value: "1"}}, Backends: backendNamed("query")},
		{Source: "HTTPRoute/default/a", Created: "2025-01-01T00:00:00Z", Path: prefix, Backends: backendNamed("newer")},
	}}
	table.Prepare()
	header := http.Header{"X-A": {"1"}}
	if r := lookup(table, "h", "GET", "/x", header, "q=1"); r.Backends[0].Name != "query" {
		t.Fatal(r.Backends[0].Name)
	}
	if r := lookup(table, "h", "GET", "/x", header, ""); r.Backends[0].Name != "header" {
		t.Fatal(r.Backends[0].Name)
	}
	if r := lookup(table, "h", "GET", "/x", http.Header{}, ""); r.Backends[0].Name != "method" {
		t.Fatal(r.Backends[0].Name)
	}
	if r := lookup(table, "h", "POST", "/x", http.Header{}, ""); r.Backends[0].Name != "plain" {
		t.Fatal(r.Backends[0].Name)
	}
}

func TestLookupMatchers(t *testing.T) {
	table := &Table{Rules: []Rule{{
		Source:  "HTTPRoute/default/r",
		Path:    &pathMatch{Type: "PathPrefix", Value: "/"},
		Method:  "POST",
		Headers: []ValueMatch{{Name: "x-env", Value: "^prod-\\d+$", Type: "RegularExpression"}},
		Query:   []ValueMatch{{Name: "debug", Value: "1"}},
	}}}
	table.Prepare()
	ok := http.Header{"X-Env": {"prod-12"}}
	if lookup(table, "h", "POST", "/", ok, "debug=1") == nil {
		t.Fatal("hit")
	}
	if lookup(table, "h", "GET", "/", ok, "debug=1") != nil {
		t.Fatal("method")
	}
	if lookup(table, "h", "POST", "/", http.Header{"X-Env": {"dev"}}, "debug=1") != nil {
		t.Fatal("header regex")
	}
	if lookup(table, "h", "POST", "/", ok, "debug=2") != nil {
		t.Fatal("query")
	}
	if lookup(table, "h", "POST", "/", ok, "") != nil {
		t.Fatal("query missing")
	}
}

func TestIngressPrefixIgnoresTrailingSlash(t *testing.T) {
	rules := compileIngress(ingObject{
		Meta: gwMeta{Name: "i", Namespace: "default"},
		Spec: ingSpec{Rules: []ingRule{{Host: "a.example.com", HTTP: &ingHTTP{Paths: []ingPath{
			{Path: "/foo/", PathType: "Prefix", Backend: ingBackend{Service: &ingService{Name: "s", Port: ingPort{Number: 80}}}},
		}}}}},
	})
	table := &Table{Rules: rules}
	table.Prepare()
	for path, want := range map[string]bool{"/foo": true, "/foo/": true, "/foo/bar": true, "/foobar": false, "/": false} {
		if got := lookup(table, "a.example.com", "GET", path, http.Header{}, "") != nil; got != want {
			t.Fatalf("%s: %v", path, got)
		}
	}
}

func TestIngressDefaultBackendIsLastResort(t *testing.T) {
	rules := compileIngress(ingObject{
		Meta: gwMeta{Name: "i", Namespace: "default"},
		Spec: ingSpec{
			DefaultBackend: &ingBackend{Service: &ingService{Name: "fallback", Port: ingPort{Name: "http"}}},
			Rules: []ingRule{{Host: "a.example.com", HTTP: &ingHTTP{Paths: []ingPath{
				{Path: "/x", PathType: "Exact", Backend: ingBackend{Service: &ingService{Name: "x", Port: ingPort{Number: 8080}}}},
			}}}},
		},
	})
	table := &Table{Rules: rules}
	table.Prepare()
	if r := lookup(table, "a.example.com", "GET", "/x", http.Header{}, ""); r.Backends[0].Name != "x" || r.Backends[0].Port != 8080 {
		t.Fatalf("%+v", r)
	}
	r := lookup(table, "a.example.com", "GET", "/y", http.Header{}, "")
	if r == nil || r.Backends[0].Name != "fallback" || r.Backends[0].PortName != "http" {
		t.Fatalf("%+v", r)
	}
	if r := lookup(table, "z.example.com", "GET", "/y", http.Header{}, ""); r == nil || r.Backends[0].Name != "fallback" {
		t.Fatalf("%+v", r)
	}
}

func TestIngressResourceBackendIsInvalid(t *testing.T) {
	rules := compileIngress(ingObject{
		Meta: gwMeta{Name: "i", Namespace: "default"},
		Spec: ingSpec{DefaultBackend: &ingBackend{Resource: []byte(`{"kind":"StorageBucket"}`)}},
	})
	if len(rules) != 1 || rules[0].Invalid == "" {
		t.Fatalf("%+v", rules)
	}
}

func TestPickBackendHonorsWeights(t *testing.T) {
	backends := []Backend{{Name: "a", Weight: 3}, {Name: "off", Weight: 0}, {Name: "b", Weight: 1}}
	want := []string{"a", "a", "a", "b"}
	for n, name := range want {
		if got := pickBackend(backends, n); got == nil || got.Name != name {
			t.Fatalf("%d: %+v", n, got)
		}
	}
	if pickBackend([]Backend{{Name: "z", Weight: 0}}, 0) != nil {
		t.Fatal("all zero")
	}
	if total := totalWeight(backends); total != 4 {
		t.Fatal(total)
	}
}

func TestApplyFilters(t *testing.T) {
	newReq := func(target string) *http.Request {
		r, _ := http.NewRequest("GET", target, nil)
		r.Host = r.URL.Host
		r.Header.Set("X-Old", "1")
		r.Header.Set("X-Keep", "k")
		return r
	}
	rule := &Rule{
		Path: &pathMatch{Type: "PathPrefix", Value: "/api"},
		Filters: []Filter{
			{Type: "RequestHeaderModifier", RequestHeaderModifier: &HeaderModifier{
				Set:    []HeaderValue{{Name: "X-Set", Value: "s"}},
				Add:    []HeaderValue{{Name: "X-Keep", Value: "more"}},
				Remove: []string{"X-Old"},
			}},
			{Type: "URLRewrite", URLRewrite: &URLRewrite{Hostname: "backend.internal", Path: &PathModifier{Type: "ReplacePrefixMatch", ReplacePrefixMatch: "/v2"}}},
		},
	}
	out := applyFilters(rule, newReq("https://app.example.com/api/users?a=1"))
	if out.Redirect != "" || out.Path != "/v2/users" || out.Host != "backend.internal" {
		t.Fatalf("%+v", out)
	}
	if out.Header.Get("X-Set") != "s" || out.Header.Get("X-Old") != "" || len(out.Header.Values("X-Keep")) != 2 {
		t.Fatalf("%v", out.Header)
	}
	full := &Rule{Path: &pathMatch{Type: "PathPrefix", Value: "/"}, Filters: []Filter{{Type: "URLRewrite", URLRewrite: &URLRewrite{Path: &PathModifier{Type: "ReplaceFullPath", ReplaceFullPath: "/fixed"}}}}}
	if got := applyFilters(full, newReq("https://h/anything")); got.Path != "/fixed" {
		t.Fatalf("%+v", got)
	}
	root := &Rule{Path: &pathMatch{Type: "PathPrefix", Value: "/api"}, Filters: []Filter{{Type: "URLRewrite", URLRewrite: &URLRewrite{Path: &PathModifier{Type: "ReplacePrefixMatch", ReplacePrefixMatch: "/"}}}}}
	if got := applyFilters(root, newReq("https://h/api")); got.Path != "/" {
		t.Fatalf("%+v", got)
	}
	if got := applyFilters(root, newReq("https://h/api/x")); got.Path != "/x" {
		t.Fatalf("%+v", got)
	}
}

func TestApplyRedirect(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://old.example.com/a/b?x=1", nil)
	r.Host = "old.example.com"
	port := int32(8443)
	rule := &Rule{Path: &pathMatch{Type: "PathPrefix", Value: "/a"}, Filters: []Filter{{Type: "RequestRedirect", RequestRedirect: &RequestRedirect{Hostname: "new.example.com", Port: &port, StatusCode: 301, Path: &PathModifier{Type: "ReplacePrefixMatch", ReplacePrefixMatch: "/z"}}}}}
	out := applyFilters(rule, r)
	if out.Redirect != "https://new.example.com:8443/z/b?x=1" || out.Status != 301 {
		t.Fatalf("%+v", out)
	}
	plain := &Rule{Filters: []Filter{{Type: "RequestRedirect", RequestRedirect: &RequestRedirect{Scheme: "http"}}}}
	out = applyFilters(plain, r)
	if out.Redirect != "http://old.example.com/a/b?x=1" || out.Status != 302 {
		t.Fatalf("%+v", out)
	}
}

func TestUnsupportedFilterInvalidatesRule(t *testing.T) {
	if reason := unsupportedFilter([]Filter{{Type: "RequestMirror"}}); reason == "" {
		t.Fatal("mirror")
	}
	if reason := unsupportedFilter([]Filter{{Type: "URLRewrite"}, {Type: "RequestRedirect"}, {Type: "RequestHeaderModifier"}}); reason != "" {
		t.Fatal(reason)
	}
}

func TestGatewayPrefixIgnoresTrailingSlash(t *testing.T) {
	route := routeIn("default", "r", nil, "", 80)
	route.Spec.Rules[0].Matches = []gwMatch{{Path: &pathMatch{Type: "PathPrefix", Value: "/abc/"}}}
	table := &Table{Rules: compileRoute(route, nil, nil, map[string][]svcPort{"default/web": {{Port: 80}}})}
	table.Prepare()
	if lookup(table, "h", "GET", "/abc", http.Header{}, "") == nil || lookup(table, "h", "GET", "/abc/d", http.Header{}, "") == nil || lookup(table, "h", "GET", "/abcd", http.Header{}, "") != nil {
		t.Fatalf("%+v", table.Rules[0].Path)
	}
}

func TestPreparedRegexPathIsNotRecompiled(t *testing.T) {
	table := &Table{Rules: []Rule{{Path: &pathMatch{Type: "RegularExpression", Value: "^/v[0-9]+/"}}}}
	table.Prepare()
	if table.Rules[0].Path.re == nil || lookup(table, "h", "GET", "/v2/x", http.Header{}, "") == nil || lookup(table, "h", "GET", "/x", http.Header{}, "") != nil {
		t.Fatal("regex path")
	}
}
