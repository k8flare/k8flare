package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strconv"

	"golang.org/x/tools/go/packages"
)

const controllersParam = "controllers"

type needs map[string]map[string]bool

func (n needs) add(controller, resource string) {
	if n[controller] == nil {
		n[controller] = map[string]bool{}
	}
	n[controller][resource] = true
}

func (n needs) sorted(controller string) []string {
	var out []string
	for r := range n[controller] {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func extract(pkgs []*packages.Package, kinds kindResources) (needs, error) {
	out := needs{}
	for _, pkg := range pkgs {
		builders := registeredBuilders(pkg)
		if len(builders) == 0 {
			return nil, fmt.Errorf("%s: no function is handed to Register", pkg.PkgPath)
		}
		for _, fn := range builders {
			w := &walker{pkg: pkg, kinds: kinds, out: out, owner: gateOwner(fn.Body)}
			ast.Walk(w, fn.Body)
			if w.err != nil {
				return nil, w.err
			}
		}
	}
	return out, nil
}

func registeredBuilders(pkg *packages.Package) []*ast.FuncDecl {
	names := map[string]bool{}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Register" {
				return true
			}
			if arg, ok := call.Args[0].(*ast.Ident); ok {
				names[arg.Name] = true
			}
			return true
		})
	}
	var out []*ast.FuncDecl
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && names[fn.Name.Name] {
				out = append(out, fn)
			}
		}
	}
	return out
}

func gateOwner(body *ast.BlockStmt) string {
	for _, stmt := range body.List {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok {
			continue
		}
		not, ok := ifStmt.Cond.(*ast.UnaryExpr)
		if !ok || not.Op != token.NOT {
			continue
		}
		if name := controllerIndex(not.X); name != "" {
			return name
		}
	}
	return ""
}

func controllerIndex(expr ast.Expr) string {
	index, ok := expr.(*ast.IndexExpr)
	if !ok {
		return ""
	}
	base, ok := index.X.(*ast.Ident)
	if !ok || base.Name != controllersParam {
		return ""
	}
	lit, ok := index.Index.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	name, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return name
}

func positiveGuard(cond ast.Expr) string {
	switch c := cond.(type) {
	case *ast.ParenExpr:
		return positiveGuard(c.X)
	case *ast.BinaryExpr:
		if c.Op != token.LAND {
			return ""
		}
		if name := positiveGuard(c.X); name != "" {
			return name
		}
		return positiveGuard(c.Y)
	}
	return controllerIndex(cond)
}

type walker struct {
	pkg   *packages.Package
	kinds kindResources
	out   needs
	owner string
	err   error
}

func (w *walker) Visit(node ast.Node) ast.Visitor {
	if w.err != nil {
		return nil
	}
	switch n := node.(type) {
	case *ast.IfStmt:
		name := positiveGuard(n.Cond)
		if name == "" {
			return w
		}
		inner := &walker{pkg: w.pkg, kinds: w.kinds, out: w.out, owner: name}
		if n.Init != nil {
			ast.Walk(w, n.Init)
		}
		ast.Walk(w, n.Cond)
		ast.Walk(inner, n.Body)
		if inner.err != nil {
			w.err = inner.err
		}
		if n.Else != nil {
			ast.Walk(w, n.Else)
		}
		return nil
	case *ast.CallExpr:
		for _, arg := range n.Args {
			w.handedOver(arg)
		}
	case *ast.CompositeLit:
		for _, elt := range n.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				elt = kv.Value
			}
			w.handedOver(elt)
		}
	}
	return w
}

func (w *walker) handedOver(expr ast.Expr) {
	pkgPath, kind, ok := informerKind(w.pkg.TypesInfo.TypeOf(expr))
	if !ok {
		return
	}
	position := w.pkg.Fset.Position(expr.Pos())
	if w.owner == "" {
		w.err = fmt.Errorf("%s: an informer for %s.%s is handed over outside every controllers[...] guard", position, pkgPath, kind)
		return
	}
	resource, ok := w.kinds[kindKey{pkg: pkgPath, kind: kind}]
	if !ok {
		w.err = fmt.Errorf("controller %s (%s): the informer for %s.%s has no resource in the served registry", w.owner, position, pkgPath, kind)
		return
	}
	w.out.add(w.owner, resource)
}

func informerKind(t types.Type) (pkgPath, kind string, ok bool) {
	if t == nil {
		return "", "", false
	}
	lister := listerOf(t)
	if lister == nil {
		return "", "", false
	}
	list := methodOf(lister, "List")
	if list == nil {
		return "", "", false
	}
	sig := list.Type().(*types.Signature)
	if sig.Params().Len() != 1 || sig.Results().Len() != 2 {
		return "", "", false
	}
	slice, ok := sig.Results().At(0).Type().(*types.Slice)
	if !ok {
		return "", "", false
	}
	ptr, ok := slice.Elem().(*types.Pointer)
	if !ok {
		return "", "", false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", "", false
	}
	return named.Obj().Pkg().Path(), named.Obj().Name(), true
}

func listerOf(t types.Type) types.Type {
	method := methodOf(t, "Lister")
	if method == nil {
		return nil
	}
	sig := method.Type().(*types.Signature)
	if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return nil
	}
	return sig.Results().At(0).Type()
}

func methodOf(t types.Type, name string) *types.Func {
	candidates := []types.Type{t}
	if _, isInterface := t.Underlying().(*types.Interface); !isInterface {
		candidates = append(candidates, types.NewPointer(t))
	}
	for _, candidate := range candidates {
		sel := types.NewMethodSet(candidate).Lookup(nil, name)
		if sel == nil {
			continue
		}
		if fn, ok := sel.Obj().(*types.Func); ok {
			return fn
		}
	}
	return nil
}
