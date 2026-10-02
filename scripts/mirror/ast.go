package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// keepDeclsJS keeps upstream's file for host builds and adds a js-only file
// that holds only the named declarations of upstream's file, unchanged.
func keepDeclsJS(path string, names ...string) op {
	return op{kind: "astJS", path: path, decls: fileEdit{keep: names}}
}

// stubFuncsJS is keepDeclsJS plus: the named functions keep their signature
// and return zero values, with errText (declared once as errVar) as the last
// error result.
func stubFuncsJS(path string, keep []string, errVar, errText string, stubs ...string) op {
	return op{kind: "astJS", path: path, decls: fileEdit{keep: keep, stubs: stubs, errVar: errVar, errText: errText}}
}

func applyDeclsAST(dst string, o op) error {
	data, err := keepHostOnly(dst, o.path)
	if err != nil {
		return err
	}
	e := o.decls
	out, err := rewrite(data, e)
	if err != nil {
		return fmt.Errorf("%s: %w", o.path, err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dst, o.path)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, jsName(o.path)), out, 0o644)
}

type fileEdit struct {
	keep    []string
	stubs   []string
	errVar  string
	errText string
}

func keepDecls(src []byte, names []string) ([]byte, error) {
	return rewrite(src, fileEdit{keep: names})
}

func stubFuncs(src []byte, keep []string, errVar, errText string, stubs []string) ([]byte, error) {
	return rewrite(src, fileEdit{keep: keep, stubs: stubs, errVar: errVar, errText: errText})
}

type posSpan struct{ from, to token.Pos }

func (s posSpan) contains(p token.Pos) bool { return s.from <= p && p <= s.to }

func rewrite(src []byte, e fileEdit) ([]byte, error) {
	out, err := rewriteDecls(src, e)
	if err != nil {
		return nil, err
	}
	return append([]byte(jsTag), out...), nil
}

func rewriteDecls(src []byte, e fileEdit) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "upstream.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	stubSet := toSet(e.stubs)
	wanted := toSet(append(append([]string{}, e.keep...), e.stubs...))
	found := map[string]bool{}
	notFunc := map[string]bool{}
	types := typeSpecs(file)
	var dropped []posSpan
	var kept []ast.Decl
	var problems []string
	needsErr := false

	for _, d := range file.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			name := funcName(d)
			if !wanted[name] {
				continue
			}
			found[name] = true
			if stubSet[name] {
				dropped = append(dropped, posSpan{d.Body.Lbrace, d.Body.Rbrace})
				if stubBody(fset, d, types, e.errVar) {
					needsErr = true
				}
			}
			kept = append(kept, d)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				kept = append(kept, d)
				continue
			}
			var specs []ast.Spec
			for _, s := range d.Specs {
				matched, err := specMatches(s, wanted, found, stubSet, notFunc)
				if err != nil {
					problems = append(problems, err.Error())
					continue
				}
				if !matched {
					dropped = append(dropped, posSpan{specStart(s), s.End()})
					continue
				}
				specs = append(specs, s)
			}
			if len(specs) > 0 {
				d.Specs = specs
				kept = append(kept, d)
			}
		}
	}

	if names := missing(wanted, found); len(names) > 0 {
		problems = append(problems, "declaration not found in upstream file: "+strings.Join(names, ", "))
	}
	if names := keys(notFunc); len(names) > 0 {
		problems = append(problems, "not a function, cannot stub: "+strings.Join(names, ", "))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}

	file.Doc = nil
	file.Decls = kept
	file.Comments = commentsWithin(file.Comments, kept, dropped)
	for _, imp := range append([]*ast.ImportSpec{}, file.Imports...) {
		path, _ := strconv.Unquote(imp.Path.Value)
		if !astutil.UsesImport(file, path) {
			name := ""
			if imp.Name != nil {
				name = imp.Name.Name
			}
			astutil.DeleteNamedImport(fset, file, name, path)
		}
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	if needsErr {
		if formatted, err = declareError(formatted, e.errVar, e.errText); err != nil {
			return nil, err
		}
	}
	return formatted, nil
}

func toSet(names []string) map[string]bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return set
}

func keys(set map[string]bool) []string {
	var names []string
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func missing(wanted, found map[string]bool) []string {
	absent := map[string]bool{}
	for n := range wanted {
		if !found[n] {
			absent[n] = true
		}
	}
	return keys(absent)
}

func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	return receiverName(d.Recv.List[0].Type) + "." + d.Name.Name
}

func receiverName(t ast.Expr) string {
	switch t := t.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.ParenExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func specMatches(s ast.Spec, wanted, found, stubs, notFunc map[string]bool) (bool, error) {
	var names []string
	switch s := s.(type) {
	case *ast.TypeSpec:
		names = []string{s.Name.Name}
	case *ast.ValueSpec:
		for _, n := range s.Names {
			names = append(names, n.Name)
		}
	}
	var kept, extra []string
	for _, n := range names {
		if wanted[n] {
			found[n] = true
			kept = append(kept, n)
			if stubs[n] {
				notFunc[n] = true
			}
		} else {
			extra = append(extra, n)
		}
	}
	if len(kept) > 0 && len(extra) > 0 {
		return false, fmt.Errorf("spec declaring %s also declares extra names: %s", strings.Join(names, ", "), strings.Join(extra, ", "))
	}
	return len(kept) > 0, nil
}

func specStart(s ast.Spec) token.Pos {
	switch s := s.(type) {
	case *ast.TypeSpec:
		if s.Doc != nil {
			return s.Doc.Pos()
		}
	case *ast.ValueSpec:
		if s.Doc != nil {
			return s.Doc.Pos()
		}
	}
	return s.Pos()
}

func declStart(d ast.Decl) token.Pos {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Doc != nil {
			return d.Doc.Pos()
		}
	case *ast.GenDecl:
		if d.Doc != nil {
			return d.Doc.Pos()
		}
	}
	return d.Pos()
}

func commentsWithin(comments []*ast.CommentGroup, kept []ast.Decl, dropped []posSpan) []*ast.CommentGroup {
	var out []*ast.CommentGroup
	for _, cg := range comments {
		inside := false
		for _, d := range kept {
			if (posSpan{declStart(d), d.End()}).contains(cg.Pos()) {
				inside = true
			}
		}
		for _, s := range dropped {
			if s.contains(cg.Pos()) {
				inside = false
			}
		}
		if inside {
			out = append(out, cg)
		}
	}
	return out
}

func typeSpecs(file *ast.File) map[string]ast.Expr {
	types := map[string]ast.Expr{}
	for _, d := range file.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
			for _, s := range g.Specs {
				ts := s.(*ast.TypeSpec)
				types[ts.Name.Name] = ts.Type
			}
		}
	}
	return types
}

func stubBody(fset *token.FileSet, d *ast.FuncDecl, types map[string]ast.Expr, errVar string) (usesErr bool) {
	body := &ast.BlockStmt{Lbrace: d.Body.Lbrace, Rbrace: d.Body.Rbrace}
	d.Body = body
	if d.Type.Results == nil {
		return false
	}
	var results []ast.Expr
	for _, f := range d.Type.Results.List {
		for i := 0; i < max(1, len(f.Names)); i++ {
			results = append(results, ast.NewIdent(zeroValue(fset, f.Type, types, 0)))
		}
	}
	if last := d.Type.Results.List[len(d.Type.Results.List)-1]; errVar != "" && isIdent(last.Type, "error") {
		results[len(results)-1] = ast.NewIdent(errVar)
		usesErr = true
	}
	body.List = []ast.Stmt{&ast.ReturnStmt{Results: results}}
	return usesErr
}

func isIdent(t ast.Expr, name string) bool {
	id, ok := t.(*ast.Ident)
	return ok && id.Name == name
}

func zeroValue(fset *token.FileSet, t ast.Expr, types map[string]ast.Expr, depth int) string {
	switch t := t.(type) {
	case *ast.ParenExpr:
		return zeroValue(fset, t.X, types, depth)
	case *ast.StarExpr, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		return "nil"
	case *ast.ArrayType:
		if t.Len == nil {
			return "nil"
		}
		return printed(fset, t) + "{}"
	case *ast.StructType:
		return printed(fset, t) + "{}"
	case *ast.Ident:
		switch t.Name {
		case "bool":
			return "false"
		case "string":
			return `""`
		case "error", "any":
			return "nil"
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune", "float32", "float64", "complex64", "complex128":
			return "0"
		}
		underlying, ok := types[t.Name]
		if !ok || depth > 8 {
			return "*new(" + t.Name + ")"
		}
		if _, isStruct := underlying.(*ast.StructType); isStruct {
			return t.Name + "{}"
		}
		if _, isArray := underlying.(*ast.ArrayType); isArray && underlying.(*ast.ArrayType).Len != nil {
			return t.Name + "{}"
		}
		return zeroValue(fset, underlying, types, depth+1)
	}
	return "*new(" + printed(fset, t) + ")"
}

func printed(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		panic(err)
	}
	return buf.String()
}

func declareError(src []byte, name, text string) ([]byte, error) {
	src = append(src, fmt.Sprintf("\nvar %s = errors.New(%s)\n", name, strconv.Quote(text))...)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "stubbed.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	astutil.AddImport(fset, file, "errors")
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, err
	}
	return format.Source(buf.Bytes())
}
