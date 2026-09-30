package edgehost

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ValueMatch struct {
	Type  string `json:"type,omitempty"`
	Name  string `json:"name"`
	Value string `json:"value"`
	re    *regexp.Regexp
}

type HeaderValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HeaderModifier struct {
	Set    []HeaderValue `json:"set,omitempty"`
	Add    []HeaderValue `json:"add,omitempty"`
	Remove []string      `json:"remove,omitempty"`
}

type PathModifier struct {
	Type               string `json:"type"`
	ReplaceFullPath    string `json:"replaceFullPath,omitempty"`
	ReplacePrefixMatch string `json:"replacePrefixMatch,omitempty"`
}

type RequestRedirect struct {
	Scheme     string        `json:"scheme,omitempty"`
	Hostname   string        `json:"hostname,omitempty"`
	Port       *int32        `json:"port,omitempty"`
	StatusCode int           `json:"statusCode,omitempty"`
	Path       *PathModifier `json:"path,omitempty"`
}

type URLRewrite struct {
	Hostname string        `json:"hostname,omitempty"`
	Path     *PathModifier `json:"path,omitempty"`
}

type Filter struct {
	Type                   string           `json:"type"`
	RequestHeaderModifier  *HeaderModifier  `json:"requestHeaderModifier,omitempty"`
	ResponseHeaderModifier *HeaderModifier  `json:"responseHeaderModifier,omitempty"`
	RequestRedirect        *RequestRedirect `json:"requestRedirect,omitempty"`
	URLRewrite             *URLRewrite      `json:"urlRewrite,omitempty"`
}

type Backend struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Port      int32  `json:"port,omitempty"`
	PortName  string `json:"portName,omitempty"`
	Weight    int32  `json:"weight"`
}

type Rule struct {
	Source   string       `json:"source"`
	Created  string       `json:"created,omitempty"`
	Host     string       `json:"host,omitempty"`
	Fallback bool         `json:"fallback,omitempty"`
	Path     *pathMatch   `json:"path,omitempty"`
	Method   string       `json:"method,omitempty"`
	Headers  []ValueMatch `json:"headers,omitempty"`
	Query    []ValueMatch `json:"query,omitempty"`
	Filters  []Filter     `json:"filters,omitempty"`
	Backends []Backend    `json:"backends,omitempty"`
	Invalid  string       `json:"invalid,omitempty"`
}

type Table struct {
	Rules []Rule `json:"rules"`
}

func (t *Table) Prepare() {
	for i := range t.Rules {
		if p := t.Rules[i].Path; p != nil && p.Type == "RegularExpression" {
			p.re, _ = regexp.Compile(p.Value)
		}
		prepareMatches(t.Rules[i].Headers)
		prepareMatches(t.Rules[i].Query)
	}
	sort.SliceStable(t.Rules, func(i, j int) bool { return ruleBefore(&t.Rules[i], &t.Rules[j]) })
}

func prepareMatches(matches []ValueMatch) {
	for i := range matches {
		if matches[i].Type == "RegularExpression" {
			matches[i].re, _ = regexp.Compile(matches[i].Value)
		}
	}
}

func hostRank(host string) int {
	switch {
	case host == "":
		return 2
	case strings.HasPrefix(host, "*."):
		return 1
	}
	return 0
}

func pathClass(m *pathMatch) (class, length int) {
	if m == nil {
		return 1, 0
	}
	switch m.Type {
	case "Exact":
		return 0, 0
	case "RegularExpression":
		return 2, 0
	}
	return 1, len(m.Value)
}

func ruleBefore(a, b *Rule) bool {
	if a.Fallback != b.Fallback {
		return !a.Fallback
	}
	if ra, rb := hostRank(a.Host), hostRank(b.Host); ra != rb {
		return ra < rb
	}
	if len(a.Host) != len(b.Host) {
		return len(a.Host) > len(b.Host)
	}
	ca, la := pathClass(a.Path)
	cb, lb := pathClass(b.Path)
	if ca != cb {
		return ca < cb
	}
	if la != lb {
		return la > lb
	}
	if (a.Method != "") != (b.Method != "") {
		return a.Method != ""
	}
	if len(a.Headers) != len(b.Headers) {
		return len(a.Headers) > len(b.Headers)
	}
	if len(a.Query) != len(b.Query) {
		return len(a.Query) > len(b.Query)
	}
	if a.Created != b.Created {
		return a.Created < b.Created
	}
	return a.Source < b.Source
}

func (t *Table) Lookup(host, method, path string, header http.Header, query url.Values) *Rule {
	for i := range t.Rules {
		rule := &t.Rules[i]
		if rule.Host != "" && !MatchHostname(host, []string{rule.Host}) {
			continue
		}
		if !MatchPath(path, rule.Path) {
			continue
		}
		if rule.Method != "" && !strings.EqualFold(rule.Method, method) {
			continue
		}
		if !matchAll(rule.Headers, func(name string) (string, bool) {
			v := header.Values(name)
			if len(v) == 0 {
				return "", false
			}
			return v[0], true
		}) {
			continue
		}
		if !matchAll(rule.Query, func(name string) (string, bool) {
			if !query.Has(name) {
				return "", false
			}
			return query.Get(name), true
		}) {
			continue
		}
		return rule
	}
	return nil
}

func matchAll(matches []ValueMatch, get func(string) (string, bool)) bool {
	for _, m := range matches {
		got, ok := get(m.Name)
		if !ok {
			return false
		}
		if m.Type == "RegularExpression" {
			if m.re == nil || !m.re.MatchString(got) {
				return false
			}
			continue
		}
		if got != m.Value {
			return false
		}
	}
	return true
}

func totalWeight(backends []Backend) int {
	total := 0
	for _, b := range backends {
		if b.Weight > 0 {
			total += int(b.Weight)
		}
	}
	return total
}

func pickBackend(backends []Backend, n int) *Backend {
	for i := range backends {
		if backends[i].Weight <= 0 {
			continue
		}
		if n < int(backends[i].Weight) {
			return &backends[i]
		}
		n -= int(backends[i].Weight)
	}
	return nil
}

var supportedFilters = map[string]bool{
	"RequestHeaderModifier":  true,
	"ResponseHeaderModifier": true,
	"RequestRedirect":        true,
	"URLRewrite":             true,
}

func unsupportedFilter(filters []Filter) string {
	for _, f := range filters {
		if !supportedFilters[f.Type] {
			return "unsupported filter type " + f.Type
		}
	}
	return ""
}

type filterOutcome struct {
	Redirect       string
	Status         int
	Host           string
	Path           string
	Header         http.Header
	ResponseHeader []*HeaderModifier
}

func applyFilters(rule *Rule, r *http.Request) filterOutcome {
	out := filterOutcome{Host: RequestHost(r), Path: r.URL.Path, Header: r.Header}
	for _, f := range rule.Filters {
		switch {
		case f.RequestHeaderModifier != nil:
			modifyHeader(r.Header, f.RequestHeaderModifier)
		case f.ResponseHeaderModifier != nil:
			out.ResponseHeader = append(out.ResponseHeader, f.ResponseHeaderModifier)
		case f.URLRewrite != nil:
			if f.URLRewrite.Hostname != "" {
				out.Host = f.URLRewrite.Hostname
			}
			out.Path = rewritePath(out.Path, rule.Path, f.URLRewrite.Path)
		case f.RequestRedirect != nil:
			out.Redirect, out.Status = redirectLocation(rule, r, f.RequestRedirect)
			return out
		}
	}
	return out
}

func modifyHeader(h http.Header, m *HeaderModifier) {
	for _, v := range m.Set {
		h.Set(v.Name, v.Value)
	}
	for _, v := range m.Add {
		h.Add(v.Name, v.Value)
	}
	for _, name := range m.Remove {
		h.Del(name)
	}
}

func rewritePath(path string, matched *pathMatch, mod *PathModifier) string {
	if mod == nil {
		return path
	}
	switch mod.Type {
	case "ReplaceFullPath":
		return mod.ReplaceFullPath
	case "ReplacePrefixMatch":
		prefix := ""
		if matched != nil {
			prefix = strings.TrimSuffix(matched.Value, "/")
		}
		result := strings.TrimSuffix(mod.ReplacePrefixMatch, "/") + strings.TrimPrefix(path, prefix)
		if result == "" {
			return "/"
		}
		return result
	}
	return path
}

func redirectLocation(rule *Rule, r *http.Request, redirect *RequestRedirect) (string, int) {
	scheme := redirect.Scheme
	if scheme == "" {
		scheme = "https"
	}
	host := redirect.Hostname
	if host == "" {
		host = RequestHost(r)
	}
	if redirect.Port != nil {
		p := int(*redirect.Port)
		if !(scheme == "http" && p == 80) && !(scheme == "https" && p == 443) {
			host += ":" + strconv.Itoa(p)
		}
	}
	status := redirect.StatusCode
	if status == 0 {
		status = http.StatusFound
	}
	return scheme + "://" + host + rewritePath(r.URL.Path, rule.Path, redirect.Path) + queryOf(r.URL), status
}
