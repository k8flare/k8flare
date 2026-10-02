package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
)

const (
	typedClientsPrefix = "k8s.io/client-go/kubernetes/typed/"
	informersPrefix    = "k8s.io/client-go/informers/"
)

type groupVersion struct {
	Group   string
	Version string
}

func (gv groupVersion) key() string { return gv.Group + "/" + gv.Version }

func keptGroupVersions(groups []keptGroup) []groupVersion {
	var out []groupVersion
	for _, g := range groups {
		for _, v := range g.Versions {
			out = append(out, groupVersion{Group: g.Name, Version: v})
		}
	}
	return out
}

func keptFactoryGroups(groups []keptGroup) []string {
	var out []string
	for _, g := range groups {
		if g.InformerFactory {
			out = append(out, g.Name)
		}
	}
	return out
}

type nodeSpan struct{ from, to token.Pos }

type pruned struct {
	name    string
	fset    *token.FileSet
	file    *ast.File
	dropped []nodeSpan
}

func parsePruned(name string, src []byte) (*pruned, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return &pruned{name: name, fset: fset, file: f}, nil
}

func (p *pruned) dropField(f *ast.Field) {
	from, to := f.Pos(), f.End()
	if f.Doc != nil {
		from = f.Doc.Pos()
	}
	if f.Comment != nil {
		to = f.Comment.End()
	}
	p.dropped = append(p.dropped, nodeSpan{from, to})
}

func (p *pruned) dropFunc(fn *ast.FuncDecl) {
	from := fn.Pos()
	if fn.Doc != nil {
		from = fn.Doc.Pos()
	}
	p.dropped = append(p.dropped, nodeSpan{from, fn.End()})
}

func (p *pruned) dropNode(n ast.Node) {
	p.dropped = append(p.dropped, nodeSpan{n.Pos(), n.End()})
}

func (p *pruned) dropImport(imp *ast.ImportSpec) {
	from, to := imp.Pos(), imp.End()
	if imp.Doc != nil {
		from = imp.Doc.Pos()
	}
	if imp.Comment != nil {
		to = imp.Comment.End()
	}
	p.dropped = append(p.dropped, nodeSpan{from, to})
}

func (p *pruned) importsUnder(prefix string) map[string]string {
	out := make(map[string]string)
	for _, imp := range p.file.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil || len(importPath) <= len(prefix) || importPath[:len(prefix)] != prefix {
			continue
		}
		out[importName(imp)] = importPath[len(prefix):]
	}
	return out
}

func (p *pruned) typeSpec(name string) *ast.TypeSpec {
	for _, decl := range p.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			if ts := spec.(*ast.TypeSpec); ts.Name.Name == name {
				return ts
			}
		}
	}
	return nil
}

func (p *pruned) function(name string) *ast.FuncDecl {
	for _, decl := range p.file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func (p *pruned) methods(receiver string) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range p.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}
		if id, ok := recv.(*ast.Ident); ok && id.Name == receiver {
			out = append(out, fn)
		}
	}
	return out
}

func (p *pruned) removeDecls(drop map[ast.Decl]bool) {
	var kept []ast.Decl
	for _, decl := range p.file.Decls {
		if !drop[decl] {
			kept = append(kept, decl)
		}
	}
	p.file.Decls = kept
}

func qualifier(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func firstResult(ft *ast.FuncType) ast.Expr {
	if ft.Results == nil || len(ft.Results.List) == 0 {
		return nil
	}
	return ft.Results.List[0].Type
}

func (p *pruned) render() ([]byte, error) {
	p.dropComments()
	p.pruneImports()
	var buf bytes.Buffer
	buf.WriteString(jsTag)
	if err := format.Node(&buf, p.fset, p.file); err != nil {
		return nil, fmt.Errorf("format %s: %w", p.name, err)
	}
	return format.Source(buf.Bytes())
}

func (p *pruned) dropComments() {
	var kept []*ast.CommentGroup
	for _, cg := range p.file.Comments {
		inside := false
		for _, s := range p.dropped {
			if cg.Pos() >= s.from && cg.End() <= s.to {
				inside = true
				break
			}
		}
		if !inside {
			kept = append(kept, cg)
		}
	}
	p.file.Comments = kept
}

func (p *pruned) pruneImports() {
	referenced := make(map[string]bool)
	for _, decl := range p.file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					referenced[id.Name] = true
				}
			}
			return true
		})
	}
	var imports []*ast.ImportSpec
	for _, decl := range p.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		var kept []ast.Spec
		for _, spec := range gen.Specs {
			imp := spec.(*ast.ImportSpec)
			if name := importName(imp); name == "_" || name == "." || referenced[name] {
				kept = append(kept, spec)
				imports = append(imports, imp)
				continue
			}
			p.dropImport(imp)
		}
		gen.Specs = kept
	}
	p.file.Imports = imports
	p.dropComments()
}

func (p *pruned) mergeDecls(src string) error {
	extra, err := parser.ParseFile(p.fset, p.name+".extra", src, 0)
	if err != nil {
		return fmt.Errorf("parse additions to %s: %w", p.name, err)
	}
	have := make(map[string]bool)
	var importDecl *ast.GenDecl
	for _, decl := range p.file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			if importDecl == nil {
				importDecl = gen
			}
			for _, spec := range gen.Specs {
				have[spec.(*ast.ImportSpec).Path.Value] = true
			}
		}
	}
	for _, decl := range extra.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			p.file.Decls = append(p.file.Decls, decl)
			continue
		}
		for _, spec := range gen.Specs {
			imp := spec.(*ast.ImportSpec)
			if have[imp.Path.Value] {
				continue
			}
			if importDecl == nil {
				return fmt.Errorf("%s has no import declaration to extend", p.name)
			}
			importDecl.Specs = append(importDecl.Specs, imp)
			p.file.Imports = append(p.file.Imports, imp)
		}
	}
	return nil
}

type presence struct {
	where map[string]map[string]bool
}

func newPresence() *presence { return &presence{where: make(map[string]map[string]bool)} }

func (pr *presence) mark(place, key string) {
	if pr.where[place] == nil {
		pr.where[place] = make(map[string]bool)
	}
	pr.where[place][key] = true
}

func (pr *presence) require(places []string, keys []string, describe func(key string) string) error {
	for _, key := range keys {
		for _, place := range places {
			if !pr.where[place][key] {
				return fmt.Errorf("%s not found in %s", describe(key), place)
			}
		}
	}
	return nil
}

func leanClientset(src []byte, keep []groupVersion) ([]byte, error) {
	p, err := parsePruned("clientset.go", src)
	if err != nil {
		return nil, err
	}
	typed := p.importsUnder(typedClientsPrefix)
	wanted := make(map[string]bool)
	var keys []string
	for _, gv := range keep {
		imported := false
		for _, key := range typed {
			imported = imported || key == gv.key()
		}
		if !imported {
			return nil, fmt.Errorf("group-version %s is not imported by clientset.go", gv.key())
		}
		wanted[gv.key()] = true
		keys = append(keys, gv.key())
	}
	judge := func(e ast.Expr) (api, kept bool, key string) {
		key, api = typed[qualifier(e)]
		return api, wanted[key], key
	}

	iface := p.typeSpec("Interface")
	if iface == nil {
		return nil, fmt.Errorf("interface Interface not found in clientset.go")
	}
	ifaceType, ok := iface.Type.(*ast.InterfaceType)
	if !ok {
		return nil, fmt.Errorf("Interface in clientset.go is not an interface")
	}
	strct := p.typeSpec("Clientset")
	if strct == nil {
		return nil, fmt.Errorf("struct Clientset not found in clientset.go")
	}
	structType, ok := strct.Type.(*ast.StructType)
	if !ok {
		return nil, fmt.Errorf("Clientset in clientset.go is not a struct")
	}
	newForConfigAndClient := p.function("NewForConfigAndClient")
	if newForConfigAndClient == nil {
		return nil, fmt.Errorf("func NewForConfigAndClient not found in clientset.go")
	}
	newClientset := p.function("New")
	if newClientset == nil {
		return nil, fmt.Errorf("func New not found in clientset.go")
	}

	seen := newPresence()

	var ifaceMethods []*ast.Field
	for _, m := range ifaceType.Methods.List {
		ft, isFunc := m.Type.(*ast.FuncType)
		if !isFunc {
			ifaceMethods = append(ifaceMethods, m)
			continue
		}
		api, kept, key := judge(firstResult(ft))
		switch {
		case !api:
			ifaceMethods = append(ifaceMethods, m)
		case kept:
			seen.mark("Interface", key)
			ifaceMethods = append(ifaceMethods, m)
		default:
			p.dropField(m)
		}
	}
	ifaceType.Methods.List = ifaceMethods

	var fields []*ast.Field
	for _, f := range structType.Fields.List {
		api, kept, key := judge(f.Type)
		switch {
		case !api:
			fields = append(fields, f)
		case kept:
			seen.mark("Clientset", key)
			fields = append(fields, f)
		default:
			p.dropField(f)
		}
	}
	structType.Fields.List = fields

	drop := make(map[ast.Decl]bool)
	for _, fn := range p.methods("Clientset") {
		api, kept, key := judge(firstResult(fn.Type))
		switch {
		case !api:
		case kept:
			seen.mark("Clientset methods", key)
		default:
			drop[fn] = true
			p.dropFunc(fn)
		}
	}
	p.removeDecls(drop)

	for _, ctor := range []struct {
		fn    *ast.FuncDecl
		place string
	}{{newForConfigAndClient, "NewForConfigAndClient"}, {newClientset, "New"}} {
		var stmts []ast.Stmt
		list := ctor.fn.Body.List
		for i := 0; i < len(list); i++ {
			stmt := list[i]
			assign, isAssign := stmt.(*ast.AssignStmt)
			if !isAssign || len(assign.Rhs) != 1 {
				stmts = append(stmts, stmt)
				continue
			}
			call, isCall := assign.Rhs[0].(*ast.CallExpr)
			if !isCall {
				stmts = append(stmts, stmt)
				continue
			}
			sel, isSel := call.Fun.(*ast.SelectorExpr)
			if !isSel {
				stmts = append(stmts, stmt)
				continue
			}
			key, api := typed[qualifier(sel)]
			if !api {
				stmts = append(stmts, stmt)
				continue
			}
			if wanted[key] {
				seen.mark(ctor.place, key)
				stmts = append(stmts, stmt)
				continue
			}
			p.dropNode(stmt)
			if len(assign.Lhs) == 2 {
				errIdent, ok := assign.Lhs[1].(*ast.Ident)
				if !ok || i+1 >= len(list) || !isErrReturnGuard(list[i+1], errIdent.Name) {
					return nil, fmt.Errorf("%s: %s: expected err != nil guard following assignment", ctor.place, key)
				}
				p.dropNode(list[i+1])
				i++
			} else if len(assign.Lhs) != 1 {
				return nil, fmt.Errorf("%s: %s: unexpected assignment shape with %d left-hand sides", ctor.place, key, len(assign.Lhs))
			}
		}
		ctor.fn.Body.List = stmts
	}

	places := []string{"Interface", "Clientset", "Clientset methods", "NewForConfigAndClient", "New"}
	if err := seen.require(places, keys, func(key string) string { return "group-version " + key }); err != nil {
		return nil, err
	}
	return p.render()
}

func isErrReturnGuard(stmt ast.Stmt, errName string) bool {
	guard, ok := stmt.(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil {
		return false
	}
	bin, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	if !(isIdent(bin.X, errName) && isIdent(bin.Y, "nil")) && !(isIdent(bin.X, "nil") && isIdent(bin.Y, errName)) {
		return false
	}
	if guard.Body == nil || len(guard.Body.List) != 1 {
		return false
	}
	ret, ok := guard.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if !isIdent(ret.Results[len(ret.Results)-1], errName) {
		return false
	}
	for _, res := range ret.Results[:len(ret.Results)-1] {
		if !isIdent(res, "nil") {
			return false
		}
	}
	return true
}

func leanInformerFactory(src []byte, keepGroups []string) ([]byte, error) {
	p, err := parsePruned("factory.go", src)
	if err != nil {
		return nil, err
	}
	groups := p.importsUnder(informersPrefix)
	wanted := make(map[string]bool)
	for _, g := range keepGroups {
		imported := false
		for _, name := range groups {
			imported = imported || name == g
		}
		if !imported {
			return nil, fmt.Errorf("group %s is not imported by factory.go", g)
		}
		wanted[g] = true
	}
	judge := func(e ast.Expr) (api, kept bool, group string) {
		group, api = groups[qualifier(e)]
		return api, wanted[group], group
	}

	iface := p.typeSpec("SharedInformerFactory")
	if iface == nil {
		return nil, fmt.Errorf("interface SharedInformerFactory not found in factory.go")
	}
	ifaceType, ok := iface.Type.(*ast.InterfaceType)
	if !ok {
		return nil, fmt.Errorf("SharedInformerFactory in factory.go is not an interface")
	}

	seen := newPresence()
	var ifaceMethods []*ast.Field
	for _, m := range ifaceType.Methods.List {
		ft, isFunc := m.Type.(*ast.FuncType)
		if !isFunc || len(m.Names) == 0 {
			ifaceMethods = append(ifaceMethods, m)
			continue
		}
		api, kept, group := judge(firstResult(ft))
		switch {
		case !api:
			ifaceMethods = append(ifaceMethods, m)
		case kept:
			seen.mark("SharedInformerFactory", group)
			ifaceMethods = append(ifaceMethods, m)
		default:
			p.dropField(m)
		}
	}
	ifaceType.Methods.List = ifaceMethods

	drop := make(map[ast.Decl]bool)
	for _, fn := range p.methods("sharedInformerFactory") {
		api, kept, group := judge(firstResult(fn.Type))
		switch {
		case !api:
		case kept:
			seen.mark("sharedInformerFactory methods", group)
		default:
			drop[fn] = true
			p.dropFunc(fn)
		}
	}
	p.removeDecls(drop)

	places := []string{"SharedInformerFactory", "sharedInformerFactory methods"}
	if err := seen.require(places, keepGroups, func(group string) string { return "group " + group }); err != nil {
		return nil, err
	}
	if err := p.mergeDecls(genericInformerStub); err != nil {
		return nil, err
	}
	return p.render()
}

func leanInformerGroup(src []byte, group string, keepVersions []string) ([]byte, error) {
	p, err := parsePruned("interface.go", src)
	if err != nil {
		return nil, err
	}
	versions := p.importsUnder(informersPrefix + group + "/")
	wanted := make(map[string]bool)
	for _, v := range keepVersions {
		imported := false
		for _, name := range versions {
			imported = imported || name == v
		}
		if !imported {
			return nil, fmt.Errorf("version %s of group %s is not imported by informers/%s/interface.go", v, group, group)
		}
		wanted[v] = true
	}
	judge := func(e ast.Expr) (api, kept bool, version string) {
		version, api = versions[qualifier(e)]
		return api, wanted[version], version
	}

	iface := p.typeSpec("Interface")
	if iface == nil {
		return nil, fmt.Errorf("interface Interface not found in informers/%s/interface.go", group)
	}
	ifaceType, ok := iface.Type.(*ast.InterfaceType)
	if !ok {
		return nil, fmt.Errorf("Interface in informers/%s/interface.go is not an interface", group)
	}

	seen := newPresence()
	var ifaceMethods []*ast.Field
	for _, m := range ifaceType.Methods.List {
		ft, isFunc := m.Type.(*ast.FuncType)
		if !isFunc {
			ifaceMethods = append(ifaceMethods, m)
			continue
		}
		api, kept, version := judge(firstResult(ft))
		switch {
		case !api:
			ifaceMethods = append(ifaceMethods, m)
		case kept:
			seen.mark("Interface", version)
			ifaceMethods = append(ifaceMethods, m)
		default:
			p.dropField(m)
		}
	}
	ifaceType.Methods.List = ifaceMethods

	drop := make(map[ast.Decl]bool)
	for _, fn := range p.methods("group") {
		api, kept, version := judge(firstResult(fn.Type))
		switch {
		case !api:
		case kept:
			seen.mark("group methods", version)
		default:
			drop[fn] = true
			p.dropFunc(fn)
		}
	}
	p.removeDecls(drop)

	places := []string{"Interface", "group methods"}
	describe := func(version string) string { return "version " + version + " of group " + group }
	if err := seen.require(places, keepVersions, describe); err != nil {
		return nil, err
	}
	return p.render()
}
