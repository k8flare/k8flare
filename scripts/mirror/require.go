package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// requireDecls fails the mirror when upstream's file no longer declares the
// given functions and types with these signatures. Parameter names and
// comments are not compared.
func requireDecls(path string, decls ...string) op {
	return op{kind: "requireDecls", path: path, required: decls}
}

func applyRequireDecls(dst string, o op) error {
	data, err := os.ReadFile(filepath.Join(dst, o.path))
	if err != nil {
		return err
	}
	return checkDecls(o.path, data, o.required...)
}

func checkDecls(path string, src []byte, required ...string) error {
	have, err := declSignatures(path, src)
	if err != nil {
		return err
	}
	var problems []string
	for _, r := range required {
		want, err := declSignatures("required", []byte("package p\n"+r+"\n"))
		if err != nil {
			return fmt.Errorf("cannot parse required declaration %q: %w", r, err)
		}
		for name, sig := range want {
			got, ok := have[name]
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s: declaration %s not found in upstream file", path, name))
			case got != sig:
				problems = append(problems, fmt.Sprintf("%s: declaration %s changed upstream: have %s, want %s", path, name, got, sig))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func declSignatures(path string, src []byte) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	sigs := map[string]string{}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			sig := funcSignature(fset, d.Type)
			if d.Recv != nil && len(d.Recv.List) == 1 {
				sig = "(" + render(fset, d.Recv.List[0].Type) + ") " + sig
			}
			sigs[funcName(d)] = sig
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, s := range d.Specs {
				ts := s.(*ast.TypeSpec)
				sig := typeSignature(fset, ts.Type)
				if ts.Assign.IsValid() {
					sig = "= " + sig
				}
				sigs[ts.Name.Name] = sig
			}
		}
	}
	return sigs, nil
}

func funcSignature(fset *token.FileSet, ft *ast.FuncType) string {
	sig := "func"
	if ft.TypeParams != nil && len(ft.TypeParams.List) > 0 {
		sig += "[" + typeParams(fset, ft.TypeParams) + "]"
	}
	sig += "(" + fieldTypes(fset, ft.Params) + ")"
	if ft.Results != nil && len(ft.Results.List) > 0 {
		sig += " (" + fieldTypes(fset, ft.Results) + ")"
	}
	return sig
}

func typeParams(fset *token.FileSet, list *ast.FieldList) string {
	if list == nil {
		return ""
	}
	var params []string
	for _, f := range list.List {
		var names []string
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		param := typeSignature(fset, f.Type)
		if len(names) > 0 {
			param = strings.Join(names, ", ") + " " + param
		}
		params = append(params, param)
	}
	return strings.Join(params, ", ")
}

func fieldTypes(fset *token.FileSet, list *ast.FieldList) string {
	if list == nil {
		return ""
	}
	var types []string
	for _, f := range list.List {
		for i := 0; i < max(1, len(f.Names)); i++ {
			types = append(types, typeSignature(fset, f.Type))
		}
	}
	return strings.Join(types, ", ")
}

func typeSignature(fset *token.FileSet, t ast.Expr) string {
	switch t := t.(type) {
	case *ast.FuncType:
		return funcSignature(fset, t)
	case *ast.InterfaceType:
		return "interface{" + memberSignatures(fset, t.Methods, false) + "}"
	case *ast.StructType:
		return "struct{" + memberSignatures(fset, t.Fields, true) + "}"
	}
	return render(fset, t)
}

func memberSignatures(fset *token.FileSet, list *ast.FieldList, named bool) string {
	var members []string
	for _, f := range list.List {
		var names []string
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		member := typeSignature(fset, f.Type)
		if _, isFunc := f.Type.(*ast.FuncType); isFunc && !named {
			member = strings.Join(names, ",") + strings.TrimPrefix(member, "func")
		} else if len(names) > 0 {
			member = strings.Join(names, ",") + " " + member
		}
		members = append(members, member)
	}
	return strings.Join(members, "; ")
}
