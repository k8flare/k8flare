package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/k8flare/k8flare/scripts/internal/upstream"
)

type kindKey struct {
	pkg  string
	kind string
}

type kindResources map[kindKey]string

func loadKinds(path string) (kindResources, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	served := servedLiteral(file)
	if served == nil {
		return nil, fmt.Errorf("%s: no var Served composite literal", path)
	}
	kinds := kindResources{}
	for _, elt := range served.Elts {
		entry, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("%s: Served element is not a literal", path)
		}
		gv, resources, err := servedEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		pkg := upstream.SchemeExternal(gv)
		for _, r := range resources {
			if strings.Contains(r.name, "/") {
				continue
			}
			kinds[kindKey{pkg: pkg, kind: r.kind}] = r.name
		}
	}
	return kinds, nil
}

type servedResource struct {
	name string
	kind string
}

func servedLiteral(file *ast.File) *ast.CompositeLit {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			if len(value.Names) == 1 && value.Names[0].Name == "Served" && len(value.Values) == 1 {
				lit, _ := value.Values[0].(*ast.CompositeLit)
				return lit
			}
		}
	}
	return nil
}

func servedEntry(entry *ast.CompositeLit) (string, []servedResource, error) {
	var group, version string
	var resources []servedResource
	for _, elt := range entry.Elts {
		kv := elt.(*ast.KeyValueExpr)
		switch kv.Key.(*ast.Ident).Name {
		case "GV":
			gv := kv.Value.(*ast.CompositeLit)
			for _, f := range gv.Elts {
				field := f.(*ast.KeyValueExpr)
				value, err := strconv.Unquote(field.Value.(*ast.BasicLit).Value)
				if err != nil {
					return "", nil, err
				}
				switch field.Key.(*ast.Ident).Name {
				case "Group":
					group = value
				case "Version":
					version = value
				}
			}
		case "Resources":
			for _, r := range kv.Value.(*ast.CompositeLit).Elts {
				res, err := servedResourceOf(r.(*ast.CompositeLit))
				if err != nil {
					return "", nil, err
				}
				resources = append(resources, res)
			}
		}
	}
	gv := version
	if group != "" {
		gv = group + "/" + version
	}
	return gv, resources, nil
}

func servedResourceOf(lit *ast.CompositeLit) (servedResource, error) {
	var res servedResource
	for _, elt := range lit.Elts {
		kv := elt.(*ast.KeyValueExpr)
		basic, ok := kv.Value.(*ast.BasicLit)
		if !ok {
			continue
		}
		value, err := strconv.Unquote(basic.Value)
		if err != nil {
			return res, err
		}
		switch kv.Key.(*ast.Ident).Name {
		case "Name":
			res.name = value
		case "Kind":
			res.kind = value
		}
	}
	return res, nil
}
