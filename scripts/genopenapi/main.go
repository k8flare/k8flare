// Command genopenapi writes packages/openapi/definitions from upstream's
// generated OpenAPI definitions, keeping only the models reachable from
// the served kinds. The full set covers every Kubernetes API and imports
// every API package, which does not fit one Worker Loader dynamic worker
// next to the route installer.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/k8flare/k8flare/scripts/internal/upstream"
)

const source = "pkg/generated/openapi/zz_generated.openapi.go"

type definitions struct {
	fset    *token.FileSet
	file    *ast.File
	imports map[string]*ast.ImportSpec
	aliases map[string]string
	funcs   map[string]*ast.FuncDecl
	entries map[string]*ast.KeyValueExpr
}

func main() {
	writePins := len(os.Args) > 1 && os.Args[1] == "-write-pins"
	root, err := upstream.RepoRoot()
	check(err)
	dir, err := upstream.ModuleDir()
	check(err)
	src := filepath.Join(dir, source)
	check(checkPin(src, filepath.Join(root, "scripts/genopenapi/upstream-openapi.go.sha256"), writePins))
	defs, err := load(src)
	check(err)
	roots, err := rootModels(defs, dir)
	check(err)
	out, err := generate(defs, roots)
	check(err)
	target := filepath.Join(root, "packages/openapi/definitions/zz_generated_openapi.go")
	check(os.MkdirAll(filepath.Dir(target), 0o755))
	check(os.WriteFile(target, out, 0o644))
	fmt.Println("genopenapi: wrote", target)
}

func load(path string) (*definitions, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	d := &definitions{fset: fset, file: file, imports: map[string]*ast.ImportSpec{}, aliases: map[string]string{}, funcs: map[string]*ast.FuncDecl{}, entries: map[string]*ast.KeyValueExpr{}}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		d.imports[importName(spec)] = spec
		d.aliases[path] = importName(spec)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		d.funcs[fn.Name.Name] = fn
	}
	get, ok := d.funcs["GetOpenAPIDefinitions"]
	if !ok {
		return nil, fmt.Errorf("%s: no GetOpenAPIDefinitions", path)
	}
	ret := get.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.CompositeLit)
	for _, elt := range ret.Elts {
		kv := elt.(*ast.KeyValueExpr)
		name, ok := modelName(kv.Key)
		if !ok {
			return nil, fmt.Errorf("unexpected key %s", render(fset, kv.Key))
		}
		d.entries[name] = kv
	}
	return d, nil
}

func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	path, _ := strconv.Unquote(spec.Path.Value)
	return path[strings.LastIndex(path, "/")+1:]
}

func modelName(expr ast.Expr) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "OpenAPIModelName" {
		return "", false
	}
	lit, ok := sel.X.(*ast.CompositeLit)
	if !ok {
		return "", false
	}
	typ, ok := lit.Type.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := typ.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	return pkg.Name + "." + typ.Sel.Name, true
}

func rootModels(d *definitions, dir string) ([]string, error) {
	var roots []string
	for name := range d.entries {
		if strings.HasPrefix(name, d.aliases["k8s.io/apimachinery/pkg/apis/meta/v1"]+".") {
			roots = append(roots, name)
		}
	}
	for _, s := range upstream.Served {
		group, version, _ := strings.Cut(s.GV, "/")
		if version == "" {
			group, version = "core", group
		}
		apiGroup, _, _ := strings.Cut(group, ".")
		alias, ok := d.aliases["k8s.io/api/"+apiGroup+"/"+version]
		if !ok {
			return nil, fmt.Errorf("%s: no import for its API package", s.GV)
		}
		list, err := upstream.LoadDiscovery(dir, s.GV)
		if err != nil {
			return nil, err
		}
		for _, res := range list.Resources {
			if !contains(s.Resources, res.Name) {
				continue
			}
			suffixes := []string{""}
			if !strings.Contains(res.Name, "/") {
				suffixes = append(suffixes, "List")
			}
			for _, suffix := range suffixes {
				name := alias + "." + res.Kind + suffix
				if _, ok := d.entries[name]; ok {
					roots = append(roots, name)
				}
			}
		}
	}
	sort.Strings(roots)
	return roots, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func generate(d *definitions, roots []string) ([]byte, error) {
	keep := map[string]bool{}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if keep[name] {
			continue
		}
		keep[name] = true
		fn := d.funcs[schemaFunc(d.entries[name])]
		ast.Inspect(fn, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			if ref, ok := modelName(expr); ok {
				if _, exists := d.entries[ref]; exists && !keep[ref] {
					queue = append(queue, ref)
				}
			}
			return true
		})
	}
	names := make([]string, 0, len(keep))
	for name := range keep {
		names = append(names, name)
	}
	sort.Strings(names)

	var body bytes.Buffer
	used := map[string]bool{}
	body.WriteString("func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {\n\treturn map[string]common.OpenAPIDefinition{\n")
	for _, name := range names {
		kv := d.entries[name]
		body.WriteString("\t\t" + render(d.fset, kv.Key) + ": " + render(d.fset, kv.Value) + ",\n")
	}
	body.WriteString("\t}\n}\n\n")
	for _, name := range names {
		fn := d.funcs[schemaFunc(d.entries[name])]
		ast.Inspect(fn, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					used[id.Name] = true
				}
			}
			return true
		})
		body.WriteString(render(d.fset, fn) + "\n\n")
	}

	var out bytes.Buffer
	out.WriteString("// Code generated by scripts/genopenapi from " + upstream.Module + "@" + upstream.Version + " " + source + ". DO NOT EDIT.\n\n")
	out.WriteString("package definitions\n\nimport (\n")
	aliases := make([]string, 0, len(d.imports))
	for alias := range d.imports {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		if !used[alias] && alias != "common" {
			continue
		}
		spec := d.imports[alias]
		if spec.Name != nil {
			out.WriteString("\t" + spec.Name.Name + " " + spec.Path.Value + "\n")
		} else {
			out.WriteString("\t" + spec.Path.Value + "\n")
		}
	}
	out.WriteString(")\n\n")
	out.Write(body.Bytes())
	return format.Source(out.Bytes())
}

func schemaFunc(kv *ast.KeyValueExpr) string {
	return kv.Value.(*ast.CallExpr).Fun.(*ast.Ident).Name
}

func render(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, node); err != nil {
		panic(err)
	}
	return buf.String()
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
		return fmt.Errorf("%s changed upstream (pin %s, got %s): review the generated definitions, then refresh the pin", source, strings.TrimSpace(string(want)), got)
	}
	return nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "genopenapi:", err)
		os.Exit(1)
	}
}
