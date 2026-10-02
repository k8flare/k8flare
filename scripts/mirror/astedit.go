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
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type source struct {
	path     string
	overlays string
	data     []byte
	fset     *token.FileSet
	file     *ast.File
}

type edit func(s *source) error

type span struct {
	start, end int
	text       string
}

func patchAST(path string, edits ...edit) op {
	return op{kind: "patchAST", path: path, astOps: edits}
}

func patchJSAST(path string, edits ...edit) op {
	return op{kind: "patchJSAST", path: path, astOps: edits}
}

func applyAST(dst, overlays string, o op) error {
	var data []byte
	var err error
	if o.kind == "patchJSAST" {
		data, err = keepHostOnly(dst, o.path)
	} else {
		data, err = os.ReadFile(filepath.Join(dst, o.path))
	}
	if err != nil {
		return err
	}
	out, err := runEdits(o.path, overlays, data, o.astOps)
	if err != nil {
		return err
	}
	if o.kind == "patchJSAST" {
		return os.WriteFile(filepath.Join(dst, jsName(o.path)), append([]byte(jsTag), out...), 0o644)
	}
	return os.WriteFile(filepath.Join(dst, o.path), out, 0o644)
}

func runEdits(path, overlays string, data []byte, edits []edit) ([]byte, error) {
	s := &source{path: path, overlays: overlays, data: data}
	if err := s.reparse(); err != nil {
		return nil, err
	}
	for _, e := range edits {
		if err := e(s); err != nil {
			return nil, err
		}
	}
	out, err := format.Source(s.data)
	if err != nil {
		return nil, fmt.Errorf("%s: edited file does not format: %w", path, err)
	}
	return out, nil
}

func (s *source) reparse() error {
	s.fset = token.NewFileSet()
	f, err := parser.ParseFile(s.fset, s.path, s.data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	s.file = f
	return nil
}

func (s *source) off(p token.Pos) int { return s.fset.Position(p).Offset }

func (s *source) lineStart(off int) int { return bytes.LastIndexByte(s.data[:off], '\n') + 1 }

func (s *source) lineEnd(off int) int {
	i := bytes.IndexByte(s.data[off:], '\n')
	if i < 0 {
		return len(s.data)
	}
	return off + i + 1
}

func (s *source) wholeLines(start, end int) (int, int, bool) {
	ls, le := s.lineStart(start), s.lineEnd(end)
	if len(bytes.TrimSpace(s.data[ls:start])) != 0 || len(bytes.TrimSpace(s.data[end:le])) != 0 {
		return start, end, false
	}
	return ls, le, true
}

func (s *source) splice(spans []span) error {
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	for i := 1; i < len(spans); i++ {
		if spans[i].end > spans[i-1].start {
			return fmt.Errorf("%s: overlapping edits at offsets %d and %d", s.path, spans[i].start, spans[i-1].start)
		}
	}
	out := append([]byte{}, s.data...)
	for _, sp := range spans {
		out = append(out[:sp.start], append([]byte(sp.text), out[sp.end:]...)...)
	}
	s.data = out
	return s.reparse()
}

func (s *source) apply(spans []span, replaced ...string) error {
	if err := s.splice(spans); err != nil {
		return err
	}
	return s.dropUnusedImports(replaced)
}

func pkgNames(nodes ...ast.Node) []string {
	var names []string
	for _, n := range nodes {
		if n == nil || reflect.ValueOf(n).IsNil() {
			continue
		}
		ast.Inspect(n, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					names = append(names, x.Name)
				}
			}
			return true
		})
	}
	return names
}

func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	p, _ := strconv.Unquote(spec.Path.Value)
	return p[strings.LastIndex(p, "/")+1:]
}

func (s *source) usesPackage(name string) bool {
	used := false
	for _, d := range s.file.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		ast.Inspect(d, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == name {
					used = true
				}
			}
			return !used
		})
	}
	return used
}

func (s *source) dropUnusedImports(candidates []string) error {
	want := map[string]bool{}
	for _, c := range candidates {
		want[c] = true
	}
	var spans []span
	for _, imp := range s.file.Imports {
		name := importName(imp)
		if !want[name] || s.usesPackage(name) {
			continue
		}
		start, end := s.off(imp.Pos()), s.off(imp.End())
		start, end, _ = s.wholeLines(start, end)
		spans = append(spans, span{start, end, ""})
	}
	if len(spans) == 0 {
		return nil
	}
	return s.splice(spans)
}

func render(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	printer.Fprint(&b, fset, n)
	return b.String()
}

type fragment struct {
	stmt     ast.Stmt
	text     string
	exprType reflect.Type
}

func parseFragment(src string) (*fragment, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", "package p\nfunc _() {\n"+src+"\n}\n", 0)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %q: %w", src, err)
	}
	body := f.Decls[0].(*ast.FuncDecl).Body
	if len(body.List) != 1 {
		return nil, fmt.Errorf("%q must be exactly one statement or expression", src)
	}
	fr := &fragment{stmt: body.List[0], text: render(fset, body.List[0])}
	if es, ok := fr.stmt.(*ast.ExprStmt); ok {
		fr.exprType = reflect.TypeOf(es.X)
	}
	return fr, nil
}

type match struct {
	node       ast.Node
	start, end int
	stmt       bool
}

func (s *source) matches(root ast.Node, fr *fragment) []match {
	var found []match
	stmtType := reflect.TypeOf(fr.stmt)
	ast.Inspect(root, func(n ast.Node) bool {
		var isStmt bool
		switch n.(type) {
		case ast.Stmt:
			isStmt = true
		case ast.Expr:
		default:
			return true
		}
		t := reflect.TypeOf(n)
		if t != stmtType && (fr.exprType == nil || t != fr.exprType) {
			return true
		}
		if render(s.fset, n) != fr.text {
			return true
		}
		m := match{n, s.off(n.Pos()), s.off(n.End()), isStmt}
		if k := len(found) - 1; k >= 0 && found[k].start == m.start && found[k].end == m.end {
			found[k].stmt = found[k].stmt || m.stmt
			if m.stmt {
				found[k].node = m.node
			}
			return true
		}
		found = append(found, m)
		return true
	})
	return found
}

func (s *source) findFunc(name string) (*ast.FuncDecl, error) {
	recv, fn, isMethod := strings.Cut(name, ".")
	if !isMethod {
		recv, fn = "", name
	}
	for _, d := range s.file.Decls {
		f, ok := d.(*ast.FuncDecl)
		if !ok || f.Name.Name != fn || f.Body == nil {
			continue
		}
		if recv == "" && f.Recv == nil {
			return f, nil
		}
		if recv != "" && f.Recv != nil && len(f.Recv.List) == 1 && receiverName(f.Recv.List[0].Type) == recv {
			return f, nil
		}
	}
	return nil, fmt.Errorf("%s: function %s not found", s.path, name)
}

func receiverName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
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

func (s *source) findStruct(typ string) (*ast.StructType, error) {
	for _, d := range s.file.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE {
			continue
		}
		for _, sp := range g.Specs {
			ts := sp.(*ast.TypeSpec)
			if ts.Name.Name != typ {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, fmt.Errorf("%s: type %s is not a struct", s.path, typ)
			}
			return st, nil
		}
	}
	return nil, fmt.Errorf("%s: type %s not found", s.path, typ)
}

func (s *source) findField(typ, name string) (*ast.Field, error) {
	st, err := s.findStruct(typ)
	if err != nil {
		return nil, err
	}
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.Name == name {
				return f, nil
			}
		}
	}
	return nil, fmt.Errorf("%s: field %s.%s not found", s.path, typ, name)
}

func (s *source) one(where string, found []match, what string) (match, error) {
	switch len(found) {
	case 0:
		return match{}, fmt.Errorf("%s: %s: no match for %s", s.path, where, what)
	case 1:
		return found[0], nil
	}
	return match{}, fmt.Errorf("%s: %s: %d matches for %s, expected one", s.path, where, len(found), what)
}

func replaceSelector(pkg, name, to string) edit {
	return func(s *source) error {
		var spans []span
		ast.Inspect(s.file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == pkg && sel.Sel.Name == name {
				spans = append(spans, span{s.off(sel.Pos()), s.off(sel.End()), to})
				return false
			}
			return true
		})
		if len(spans) == 0 {
			return fmt.Errorf("%s: selector %s.%s not found", s.path, pkg, name)
		}
		return s.apply(spans, pkg)
	}
}

func (s *source) funcBody(fn string) (ast.Node, string, error) {
	if fn == "" {
		return s.file, "file", nil
	}
	f, err := s.findFunc(fn)
	if err != nil {
		return nil, "", err
	}
	return f.Body, "function " + fn, nil
}

func replaceNode(fn, from, to string) edit {
	return func(s *source) error {
		fr, err := parseFragment(from)
		if err != nil {
			return err
		}
		root, where, err := s.funcBody(fn)
		if err != nil {
			return err
		}
		m, err := s.one(where, s.matches(root, fr), from)
		if err != nil {
			return err
		}
		start, end := m.start, m.end
		if to == "" {
			if !m.stmt {
				return fmt.Errorf("%s: %s: %q is an expression and cannot be removed", s.path, where, from)
			}
			start, end, _ = s.wholeLines(start, end)
		}
		return s.apply([]span{{start, end, to}}, pkgNames(m.node)...)
	}
}

type anchorPlace int

const (
	anywhere anchorPlace = iota
	topLevel
)

func (s *source) anchor(fn, anchor string, place anchorPlace) (match, error) {
	fr, err := parseFragment(anchor)
	if err != nil {
		return match{}, err
	}
	f, err := s.findFunc(fn)
	if err != nil {
		return match{}, err
	}
	var stmts []match
	for _, m := range s.matches(f.Body, fr) {
		if !m.stmt {
			continue
		}
		if place == topLevel && !isDirectChild(f.Body, m.node) {
			continue
		}
		stmts = append(stmts, m)
	}
	return s.one("function "+fn, stmts, "statement "+anchor)
}

func isDirectChild(body *ast.BlockStmt, n ast.Node) bool {
	for _, st := range body.List {
		if ast.Node(st) == n {
			return true
		}
	}
	return false
}

func insertBefore(fn, anchor, code string) edit { return insertAt(fn, anchor, code, anywhere, true) }

func insertAfter(fn, anchor, code string) edit { return insertAt(fn, anchor, code, anywhere, false) }

func insertBeforeTopLevel(fn, anchor, code string) edit {
	return insertAt(fn, anchor, code, topLevel, true)
}

func insertAt(fn, anchor, code string, place anchorPlace, before bool) edit {
	return func(s *source) error {
		m, err := s.anchor(fn, anchor, place)
		if err != nil {
			return err
		}
		at := s.lineEnd(m.end)
		if before {
			at = s.lineStart(m.start)
		}
		return s.splice([]span{{at, at, strings.Trim(code, "\n") + "\n"}})
	}
}

func insertAtStart(fn, code string) edit {
	return func(s *source) error {
		f, err := s.findFunc(fn)
		if err != nil {
			return err
		}
		at := s.off(f.Body.Lbrace) + 1
		return s.splice([]span{{at, at, "\n" + strings.Trim(code, "\n")}})
	}
}

func replaceCallArg(fn, callee string, index int, to string) edit {
	return func(s *source) error {
		want, err := parser.ParseExpr(callee)
		if err != nil {
			return fmt.Errorf("cannot parse %q: %w", callee, err)
		}
		wantText := render(token.NewFileSet(), want)
		f, err := s.findFunc(fn)
		if err != nil {
			return err
		}
		var calls []*ast.CallExpr
		ast.Inspect(f.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok && render(s.fset, c.Fun) == wantText {
				calls = append(calls, c)
			}
			return true
		})
		if len(calls) != 1 {
			return fmt.Errorf("%s: function %s: %d calls to %s, expected one", s.path, fn, len(calls), callee)
		}
		if index >= len(calls[0].Args) {
			return fmt.Errorf("%s: function %s: call to %s has no argument %d", s.path, fn, callee, index)
		}
		arg := calls[0].Args[index]
		return s.apply([]span{{s.off(arg.Pos()), s.off(arg.End()), to}}, pkgNames(arg)...)
	}
}

func dropIfBranch(fn, cond, bodyUses string) edit {
	return func(s *source) error {
		wantCond, err := parser.ParseExpr(cond)
		if err != nil {
			return fmt.Errorf("cannot parse %q: %w", cond, err)
		}
		wantUse, err := parser.ParseExpr(bodyUses)
		if err != nil {
			return fmt.Errorf("cannot parse %q: %w", bodyUses, err)
		}
		condText, useText := render(token.NewFileSet(), wantCond), render(token.NewFileSet(), wantUse)
		f, err := s.findFunc(fn)
		if err != nil {
			return err
		}
		var found []*ast.IfStmt
		ast.Inspect(f.Body, func(n ast.Node) bool {
			st, ok := n.(*ast.IfStmt)
			if !ok || render(s.fset, st.Cond) != condText {
				return true
			}
			uses := false
			ast.Inspect(st.Body, func(n ast.Node) bool {
				if e, ok := n.(*ast.SelectorExpr); ok && render(s.fset, e) == useText {
					uses = true
				}
				return !uses
			})
			if uses {
				found = append(found, st)
			}
			return true
		})
		what := fmt.Sprintf("if %s { ... %s ... }", cond, bodyUses)
		if len(found) != 1 {
			return fmt.Errorf("%s: function %s: %d statements matching %s, expected one", s.path, fn, len(found), what)
		}
		st := found[0]
		dropped := pkgNames(st.Init, st.Cond, st.Body)
		switch e := st.Else.(type) {
		case nil:
			start, end, _ := s.wholeLines(s.off(st.Pos()), s.off(st.End()))
			return s.apply([]span{{start, end, ""}}, dropped...)
		case *ast.IfStmt:
			return s.apply([]span{{s.off(st.Pos()), s.off(e.Pos()), ""}}, dropped...)
		}
		return fmt.Errorf("%s: function %s: %s has a plain else block, which is not supported", s.path, fn, what)
	}
}

func replaceParamType(fn, param, to string) edit {
	return func(s *source) error {
		f, err := s.findFunc(fn)
		if err != nil {
			return err
		}
		lists := []*ast.FieldList{f.Type.Params}
		if f.Recv != nil {
			lists = append(lists, f.Recv)
		}
		for _, l := range lists {
			for _, fld := range l.List {
				for _, n := range fld.Names {
					if n.Name == param {
						return s.apply([]span{{s.off(fld.Type.Pos()), s.off(fld.Type.End()), to}}, pkgNames(fld.Type)...)
					}
				}
			}
		}
		return fmt.Errorf("%s: function %s: parameter %s not found", s.path, fn, param)
	}
}

func replaceFieldType(typ, name, to string) edit {
	return func(s *source) error {
		f, err := s.findField(typ, name)
		if err != nil {
			return err
		}
		return s.apply([]span{{s.off(f.Type.Pos()), s.off(f.Type.End()), to}}, pkgNames(f.Type)...)
	}
}

func removeField(typ, name string) edit {
	return func(s *source) error {
		f, err := s.findField(typ, name)
		if err != nil {
			return err
		}
		if len(f.Names) != 1 {
			return fmt.Errorf("%s: field %s.%s shares its declaration with other names", s.path, typ, name)
		}
		start, end, _ := s.wholeLines(s.off(f.Pos()), s.off(f.End()))
		return s.apply([]span{{start, end, ""}}, pkgNames(f.Type)...)
	}
}

func addField(typ, field string) edit {
	return func(s *source) error {
		st, err := s.findStruct(typ)
		if err != nil {
			return err
		}
		closing := s.off(st.Fields.Closing)
		at := s.lineStart(closing)
		if len(bytes.TrimSpace(s.data[at:closing])) != 0 {
			return fmt.Errorf("%s: type %s: closing brace is not on its own line", s.path, typ)
		}
		return s.splice([]span{{at, at, "\t" + field + "\n"}})
	}
}

func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func addImport(alias, path string) edit {
	return func(s *source) error {
		var decl *ast.GenDecl
		for _, d := range s.file.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT && g.Lparen.IsValid() {
				decl = g
				break
			}
		}
		if decl == nil {
			return fmt.Errorf("%s: no parenthesized import declaration to add %s to", s.path, path)
		}
		line := "\t"
		if alias != "" {
			line += alias + " "
		}
		line += strconv.Quote(path) + "\n"
		var last *ast.ImportSpec
		for _, sp := range decl.Specs {
			imp := sp.(*ast.ImportSpec)
			p, _ := strconv.Unquote(imp.Path.Value)
			sameName := (imp.Name == nil && alias == "") || (imp.Name != nil && imp.Name.Name == alias)
			if p == path && sameName {
				return nil
			}
			if isStdlib(p) == isStdlib(path) {
				last = imp
			}
		}
		if last == nil {
			at := s.lineStart(s.off(decl.Rparen))
			return s.splice([]span{{at, at, "\n" + line}})
		}
		at := s.lineEnd(s.off(last.End()))
		return s.splice([]span{{at, at, line}})
	}
}

func appendDecls(overlay string) edit {
	return func(s *source) error {
		text, err := os.ReadFile(filepath.Join(s.overlays, overlay))
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, overlay, text, parser.ParseComments)
		if err != nil {
			return err
		}
		if f.Name.Name != s.file.Name.Name {
			return fmt.Errorf("%s: package %s, but %s is in package %s", overlay, f.Name.Name, s.path, s.file.Name.Name)
		}
		bodyStart := fset.Position(f.Name.End()).Offset
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if err := addImport(alias, p)(s); err != nil {
				return err
			}
		}
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				bodyStart = fset.Position(g.End()).Offset
			}
		}
		body := strings.TrimSpace(string(text[bodyStart:]))
		return s.splice([]span{{len(s.data), len(s.data), "\n" + body + "\n"}})
	}
}
