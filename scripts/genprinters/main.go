// Command genprinters splits upstream's table printers per API group.
//
// k8s.io/kubernetes/pkg/printers/internalversion registers the printers
// for every kind in one AddHandlers and imports every internal API package,
// which is too large for one Worker Loader dynamic worker. This writes, for
// each served group, a package holding only that group's print functions,
// the column definitions, and the same-package helpers they reach, copied
// verbatim from the pinned upstream source.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	pinned "github.com/k8flare/k8flare/scripts/internal/upstream"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"golang.org/x/tools/imports"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const source = "pkg/printers/internalversion/printers.go"

// served maps a group to the upstream import alias of its internal
// package and the kinds this apiserver serves from it.
var served = []struct {
	group string
	alias string
	kinds []string
}{
	{"core", "api", []string{"Pod", "Node", "Namespace", "Service", "ConfigMap", "Secret", "ServiceAccount", "Event", "ReplicationController", "Endpoints", "PersistentVolumeClaim", "PersistentVolume", "LimitRange", "ResourceQuota", "PodTemplate", "ComponentStatus"}},
	{"apps", "apps", []string{"ReplicaSet", "StatefulSet", "Deployment", "DaemonSet", "ControllerRevision"}},
	{"batch", "batch", []string{"Job", "CronJob"}},
	{"autoscaling", "autoscaling", []string{"HorizontalPodAutoscaler"}},
	{"policy", "policy", []string{"PodDisruptionBudget"}},
	{"resource", "resource", []string{"DeviceClass", "ResourceClaim", "ResourceClaimTemplate", "ResourceSlice"}},
	{"coordination", "coordination", []string{"Lease"}},
	{"discovery", "discovery", []string{"EndpointSlice"}},
	{"node", "nodeapi", []string{"RuntimeClass"}},
	{"storage", "storage", []string{"CSIDriver", "CSINode", "CSIStorageCapacity", "StorageClass", "VolumeAttachment", "VolumeAttributesClass"}},
	{"rbac", "rbac", []string{"Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding"}},
	{"scheduling", "scheduling", []string{"PriorityClass"}},
	{"networking", "networking", []string{"Ingress", "IngressClass", "NetworkPolicy", "IPAddress", "ServiceCIDR"}},
	{"certificates", "certificates", []string{"CertificateSigningRequest"}},
	{"flowcontrol", "flowcontrol", []string{"FlowSchema", "PriorityLevelConfiguration"}},
}

type upstream struct {
	fset    *token.FileSet
	file    *ast.File
	funcs   map[string]*ast.FuncDecl
	methods map[string][]*ast.FuncDecl
	values  map[string]*ast.GenDecl
	types   map[string]*ast.GenDecl
	imports map[string]*ast.ImportSpec
}

func main() {
	writePins := len(os.Args) > 1 && os.Args[1] == "-write-pins"
	root, err := repoRoot()
	check(err)
	dir, err := moduleDir()
	check(err)
	src := filepath.Join(dir, source)
	check(checkPin(src, filepath.Join(root, "scripts/genprinters/upstream-printers.go.sha256"), writePins))
	up, err := load(src)
	check(err)
	for _, s := range served {
		out, err := generate(up, s.alias, s.kinds)
		check(err)
		target := filepath.Join(root, "packages", "printers-"+s.group, "tables", "zz_generated_printers.go")
		check(os.MkdirAll(filepath.Dir(target), 0o755))
		check(os.WriteFile(target, out, 0o644))
		fmt.Println("genprinters: wrote", target)
	}
}

func load(path string) (*upstream, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	up := &upstream{fset: fset, file: file, funcs: map[string]*ast.FuncDecl{}, methods: map[string][]*ast.FuncDecl{}, values: map[string]*ast.GenDecl{}, types: map[string]*ast.GenDecl{}, imports: map[string]*ast.ImportSpec{}}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				up.methods[receiverName(d)] = append(up.methods[receiverName(d)], d)
			} else {
				up.funcs[d.Name.Name] = d
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for _, n := range spec.Names {
						up.values[n.Name] = &ast.GenDecl{Tok: d.Tok, Specs: []ast.Spec{spec}}
					}
				case *ast.TypeSpec:
					up.types[spec.Name.Name] = &ast.GenDecl{Tok: d.Tok, Specs: []ast.Spec{spec}}
				case *ast.ImportSpec:
					up.imports[importName(spec)] = spec
				}
			}
		}
	}
	return up, nil
}

func receiverName(d *ast.FuncDecl) string {
	t := d.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	p, _ := strconv.Unquote(spec.Path.Value)
	return p[strings.LastIndex(p, "/")+1:]
}

// firstParamType returns "alias.Kind" for a print function's object parameter.
func firstParamType(d *ast.FuncDecl) string {
	if d.Type.Params == nil || len(d.Type.Params.List) == 0 {
		return ""
	}
	t := d.Type.Params.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if sel, ok := t.(*ast.SelectorExpr); ok {
		if x, ok := sel.X.(*ast.Ident); ok {
			return x.Name + "." + sel.Sel.Name
		}
	}
	return ""
}

func generate(up *upstream, alias string, kinds []string) ([]byte, error) {
	wanted := map[string]bool{}
	for _, k := range kinds {
		wanted[alias+"."+k] = true
		wanted[alias+"."+k+"List"] = true
	}
	// The handler registrations and the print functions they name.
	addHandlers := up.funcs["AddHandlers"]
	if addHandlers == nil {
		return nil, fmt.Errorf("upstream has no AddHandlers")
	}
	keepFuncs := map[string]bool{}
	var stmts []ast.Stmt
	var pendingDefs []ast.Stmt
	for _, st := range addHandlers.Body.List {
		if as, ok := st.(*ast.AssignStmt); ok && as.Tok == token.DEFINE {
			pendingDefs = []ast.Stmt{st}
			continue
		}
		fn := tableHandlerFunc(st)
		if fn == "" {
			continue
		}
		decl := up.funcs[fn]
		if decl == nil || !wanted[firstParamType(decl)] {
			continue
		}
		stmts = append(stmts, pendingDefs...)
		pendingDefs = nil
		stmts = append(stmts, st)
		keepFuncs[fn] = true
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("no handlers found for %s %v", alias, kinds)
	}
	// Everything the kept code reaches inside the package.
	keepValues, keepTypes := map[string]bool{}, map[string]bool{}
	pending := []ast.Node{&ast.BlockStmt{List: stmts}}
	for name := range keepFuncs {
		pending = append(pending, up.funcs[name])
	}
	for len(pending) > 0 {
		n := pending[0]
		pending = pending[1:]
		ast.Inspect(n, func(x ast.Node) bool {
			id, ok := x.(*ast.Ident)
			if !ok {
				return true
			}
			switch name := id.Name; {
			case up.funcs[name] != nil && !keepFuncs[name]:
				keepFuncs[name] = true
				pending = append(pending, up.funcs[name])
			case up.values[name] != nil && !keepValues[name]:
				keepValues[name] = true
				pending = append(pending, up.values[name])
			case up.types[name] != nil && !keepTypes[name]:
				keepTypes[name] = true
				pending = append(pending, up.types[name])
				for _, m := range up.methods[name] {
					pending = append(pending, m)
				}
			}
			return true
		})
	}
	// Emit in upstream order so diffs stay readable.
	var body bytes.Buffer
	usedPkgs := map[string]bool{}
	emit := func(n ast.Node) {
		ast.Inspect(n, func(x ast.Node) bool {
			if sel, ok := x.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					usedPkgs[id.Name] = true
				}
			}
			return true
		})
		if err := printer.Fprint(&body, up.fset, n); err != nil {
			check(err)
		}
		body.WriteString("\n\n")
	}
	body.WriteString("func AddHandlers(h printers.PrintHandler) {\n")
	for _, st := range stmts {
		var b bytes.Buffer
		check(printer.Fprint(&b, up.fset, st))
		ast.Inspect(st, func(x ast.Node) bool {
			if sel, ok := x.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					usedPkgs[id.Name] = true
				}
			}
			return true
		})
		body.WriteString("\t" + strings.ReplaceAll(b.String(), "\n", "\n\t") + "\n")
	}
	body.WriteString("}\n\n")
	for _, d := range up.file.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				if keepTypes[receiverName(d)] {
					emit(d)
				}
			} else if keepFuncs[d.Name.Name] && d.Name.Name != "AddHandlers" {
				emit(d)
			}
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			var specs []ast.Spec
			for _, spec := range d.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for _, n := range spec.Names {
						if keepValues[n.Name] {
							specs = append(specs, spec)
							break
						}
					}
				case *ast.TypeSpec:
					if keepTypes[spec.Name.Name] {
						specs = append(specs, spec)
					}
				}
			}
			if len(specs) > 0 {
				emit(&ast.GenDecl{Tok: d.Tok, Lparen: d.Lparen, Specs: specs, Rparen: d.Rparen})
			}
		}
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by scripts/genprinters from %s@%s %s. DO NOT EDIT.\n\npackage tables\n\nimport (\n", pinned.Module, pinned.Version, source)
	var names []string
	for name := range up.imports {
		if usedPkgs[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		spec := up.imports[name]
		if spec.Name != nil {
			fmt.Fprintf(&out, "\t%s %s\n", spec.Name.Name, spec.Path.Value)
		} else {
			fmt.Fprintf(&out, "\t%s\n", spec.Path.Value)
		}
		if p, _ := strconv.Unquote(spec.Path.Value); strings.HasPrefix(p, "k8s.io/kubernetes/pkg/apis/") && !strings.HasPrefix(p, "k8s.io/kubernetes/pkg/apis/core") {
			if name != alias {
				fmt.Fprintf(os.Stderr, "genprinters: %s %v reaches %s\n", alias, kinds, p)
			}
		}
	}
	out.WriteString(")\n\n")
	out.Write(body.Bytes())
	return imports.Process("", out.Bytes(), nil)
}

func tableHandlerFunc(st ast.Stmt) string {
	as, ok := st.(*ast.AssignStmt)
	if !ok || len(as.Rhs) != 1 {
		return ""
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "TableHandler" {
		return ""
	}
	if id, ok := call.Args[1].(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func moduleDir() (string, error) {
	cmd := exec.Command("go", "mod", "download", "-json", pinned.Module+"@"+pinned.Version)
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GO111MODULE=on")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var info struct{ Dir string }
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	return info.Dir, nil
}

func checkPin(src, pinFile string, write bool) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if write {
		return os.WriteFile(pinFile, []byte(got+"\n"), 0o644)
	}
	want, err := os.ReadFile(pinFile)
	if err != nil {
		return fmt.Errorf("no pin (run with -write-pins after reviewing the output): %w", err)
	}
	if strings.TrimSpace(string(want)) != got {
		return fmt.Errorf("%s changed upstream (pin %s, got %s): review the generated printers, then refresh the pin", source, strings.TrimSpace(string(want)), got)
	}
	return nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "genprinters:", err)
		os.Exit(1)
	}
}
