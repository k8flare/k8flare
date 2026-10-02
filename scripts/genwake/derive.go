package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
)

const registryRoot = "/registry/"

func quotaResources(path, function string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	var body *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == function {
			body = fn
		}
	}
	if body == nil {
		return nil, fmt.Errorf("%s: no function %s", path, function)
	}
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "WithResource" {
			return true
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if name, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, name)
			}
		}
		return true
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %s has no WithResource(\"...\") case", path, function)
	}
	return out, nil
}

func applyOutsideConstructors(derived needs, outside map[string][]string) error {
	for controller, resources := range outside {
		for _, r := range resources {
			if derived[controller][r] {
				return fmt.Errorf("exception %s -> %s is already derived from the constructors", controller, r)
			}
			derived.add(controller, r)
		}
	}
	return nil
}

func workloadPrefixes(derived needs, outside map[string][]string, routed, undelivered, extra []string) ([]string, error) {
	resources := map[string]bool{}
	for controller, set := range derived {
		for r := range set {
			if !contains(outside[controller], r) {
				resources[r] = true
			}
		}
	}
	for _, r := range append(append([]string{}, routed...), undelivered...) {
		if !resources[r] {
			return nil, fmt.Errorf("resource %s is excluded from the wake prefixes but no constructor is handed it", r)
		}
		delete(resources, r)
	}
	prefixes := map[string]bool{}
	for r := range resources {
		prefixes[registryRoot+r+"/"] = true
	}
	for _, p := range extra {
		if prefixes[p] {
			return nil, fmt.Errorf("extra prefix %s is already derived from the constructors", p)
		}
		prefixes[p] = true
	}
	var out []string
	for p := range prefixes {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
