package authz

import (
	"sort"
	"strings"
)

func selectorKind(selector string) string {
	if selector == "" {
		return "none"
	}
	if i := strings.Index(selector, "="); i > 0 {
		return strings.TrimSuffix(selector[:i], "!")
	}
	return selector
}

func distinct(values []string) string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func (q question) selectsName() bool {
	return selectorKind(q.selector) == "metadata.name" && q.name == strings.TrimPrefix(q.selector, "metadata.name=")
}

func (q question) reachable() bool {
	if q.path != "" {
		return true
	}
	if q.scope == namespaced && q.namespace == "" && q.verb != "list" && q.verb != "watch" {
		return false
	}
	switch q.verb {
	case "get", "update", "patch", "delete":
		return q.name != ""
	case "list", "watch":
		if q.subresource != "" {
			return false
		}
		if q.name == "" {
			return selectorKind(q.selector) != "metadata.name"
		}
		return q.selectsName()
	case "create":
		return (q.subresource == "") == (q.name == "")
	case "deletecollection":
		return q.name == "" && q.subresource == ""
	}
	return false
}
