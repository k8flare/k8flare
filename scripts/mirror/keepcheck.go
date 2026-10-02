package main

import (
	"fmt"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	clientGoPath    = "k8s.io/client-go/"
	clientsetPath   = clientGoPath + "kubernetes"
	informersPath   = clientGoPath + "informers"
	jsBuildTags     = "grpcnotrace"
	versionDirShape = `^v[0-9]+((alpha|beta)[0-9]+)?$`
)

var keptWithoutReference = map[string]string{
	"coordination/InformerFactory": "kube-controller-manager's controller wiring (cmd/kube-controller-manager/app/core.go) calls InformerFactory.Coordination().V1().Leases(); that package is not linked into the js build, but the accessor stays so the mirrored kube-controller-manager sources keep type-checking",
}

type keepRefs struct {
	clients          map[string]string
	informerVersions map[string]string
	factoryGroups    map[string]string
}

var missingClientPattern = regexp.MustCompile(`has no field or method ([A-Z][a-z]+)(V[0-9][A-Za-z0-9]*)?\)`)

func missingClientHints(errs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range errs {
		m := missingClientPattern.FindStringSubmatch(e)
		if m == nil {
			continue
		}
		group := strings.ToLower(m[1])
		hint := fmt.Sprintf("add group %q to keptAPIs in scripts/mirror/keep.go with InformerFactory: true (and NarrowInformer when upstream has more versions)", group)
		if m[2] != "" {
			hint = fmt.Sprintf("add version %q to the Versions of group %q in keptAPIs in scripts/mirror/keep.go", strings.ToLower(m[2]), group)
		}
		if !seen[hint] {
			seen[hint] = true
			out = append(out, hint)
		}
	}
	return out
}

func diffKeep(table []keptGroup, r keepRefs, upstreamVersions map[string][]string, allow map[string]string) []string {
	var problems []string
	used := map[string]bool{}
	byName := map[string]keptGroup{}
	for _, g := range table {
		byName[g.Name] = g
		for _, v := range g.Versions {
			gv := g.Name + "/" + v
			_, viaClient := r.clients[gv]
			_, viaInformer := r.informerVersions[gv]
			if !viaClient && !viaInformer {
				problems = append(problems, fmt.Sprintf("keptAPIs lists %s but no code in the js build references it (no Clientset method or informer version accessor); remove it from scripts/mirror/keep.go", gv))
			}
		}
		if g.InformerFactory {
			if _, ok := r.factoryGroups[g.Name]; !ok {
				key := g.Name + "/InformerFactory"
				if _, allowed := allow[key]; allowed {
					used[key] = true
				} else {
					problems = append(problems, fmt.Sprintf("keptAPIs lists %s with InformerFactory but nothing calls the %s group accessor of the informer factory; set InformerFactory to false, or name %q in keptWithoutReference with the reason", g.Name, g.Name, key))
				}
			}
		}
		wantNarrow := g.InformerFactory && hasVersionOutside(upstreamVersions[g.Name], g.Versions)
		if g.NarrowInformer != wantNarrow {
			problems = append(problems, fmt.Sprintf("keptAPIs lists %s with NarrowInformer %t but upstream's informers/%s has versions %v against kept %v, which needs NarrowInformer %t", g.Name, g.NarrowInformer, g.Name, upstreamVersions[g.Name], g.Versions, wantNarrow))
		}
	}
	for _, key := range sortedKeys(allow) {
		if !used[key] {
			problems = append(problems, fmt.Sprintf("keptWithoutReference names %s but keptAPIs no longer needs it; remove it", key))
		}
	}
	for _, gv := range sortedKeys(r.clients) {
		problems = append(problems, missingVersion(byName, gv, r.clients[gv])...)
	}
	for _, gv := range sortedKeys(r.informerVersions) {
		problems = append(problems, missingVersion(byName, gv, r.informerVersions[gv])...)
	}
	for _, group := range sortedKeys(r.factoryGroups) {
		if g, ok := byName[group]; !ok || !g.InformerFactory {
			problems = append(problems, fmt.Sprintf("%s uses the %s group accessor of the informer factory but keptAPIs does not set InformerFactory for %s", r.factoryGroups[group], group, group))
		}
	}
	return problems
}

func missingVersion(byName map[string]keptGroup, gv, where string) []string {
	group, version, _ := strings.Cut(gv, "/")
	for _, v := range byName[group].Versions {
		if v == version {
			return nil
		}
	}
	return []string{fmt.Sprintf("%s uses %s but keptAPIs does not keep it; add %q to the Versions of group %q in scripts/mirror/keep.go", where, gv, version, group)}
}

func hasVersionOutside(upstream, kept []string) bool {
	for _, u := range upstream {
		found := false
		for _, k := range kept {
			found = found || u == k
		}
		if !found {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func upstreamInformerVersions(root string, table []keptGroup) (map[string][]string, error) {
	shape := regexp.MustCompile(versionDirShape)
	out := map[string][]string{}
	for _, g := range table {
		if !g.InformerFactory {
			continue
		}
		dir := filepath.Join(root, ".build", "client-go-mirror", "informers", g.Name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read %s (run the mirror first): %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() && shape.MatchString(e.Name()) {
				out[g.Name] = append(out[g.Name], e.Name())
			}
		}
	}
	return out, nil
}

func collectKeepRefs(root string) (keepRefs, []string, error) {
	cfg := &packages.Config{
		Mode:       packages.NeedName | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedFiles,
		Dir:        root,
		Env:        append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"),
		BuildFlags: []string{"-tags", jsBuildTags},
	}
	roots, err := packages.Load(cfg, "./packages/...")
	if err != nil {
		return keepRefs{}, nil, err
	}
	all := map[string]*packages.Package{}
	packages.Visit(roots, nil, func(p *packages.Package) { all[p.PkgPath] = p })
	var loadErrors []string
	for _, p := range all {
		for _, e := range p.Errors {
			loadErrors = append(loadErrors, e.Error())
		}
	}
	sort.Strings(loadErrors)
	r := keepRefs{clients: map[string]string{}, informerVersions: map[string]string{}, factoryGroups: map[string]string{}}
	record := func(into map[string]string, key string, fset *token.FileSet, pos token.Pos) {
		if _, ok := into[key]; ok {
			return
		}
		position := fset.Position(pos)
		rel, err := filepath.Rel(root, position.Filename)
		if err != nil {
			rel = position.Filename
		}
		into[key] = fmt.Sprintf("%s:%d", rel, position.Line)
	}
	for _, path := range sortedKeys(all) {
		if path == clientsetPath || strings.HasPrefix(path, clientsetPath+"/") || strings.HasPrefix(path, informersPath) {
			continue
		}
		p := all[path]
		if p.TypesInfo == nil {
			continue
		}
		for id, obj := range p.TypesInfo.Uses {
			fn, ok := obj.(*types.Func)
			if !ok || fn.Pkg() == nil {
				continue
			}
			sig := fn.Type().(*types.Signature)
			if sig.Recv() == nil || sig.Results().Len() != 1 {
				continue
			}
			result, ok := sig.Results().At(0).Type().(*types.Named)
			if !ok || result.Obj().Pkg() == nil {
				continue
			}
			from, to := fn.Pkg().Path(), result.Obj().Pkg().Path()
			switch {
			case from == clientsetPath && strings.HasPrefix(to, clientsetPath+"/typed/"):
				if gv, ok := groupVersionOf(strings.TrimPrefix(to, clientsetPath+"/typed/")); ok {
					record(r.clients, gv, p.Fset, id.Pos())
				}
			case from == informersPath && strings.HasPrefix(to, informersPath+"/"):
				if group := strings.TrimPrefix(to, informersPath+"/"); !strings.Contains(group, "/") {
					record(r.factoryGroups, group, p.Fset, id.Pos())
				}
			case strings.HasPrefix(from, informersPath+"/") && strings.HasPrefix(to, from+"/"):
				if gv, ok := groupVersionOf(strings.TrimPrefix(to, informersPath+"/")); ok {
					record(r.informerVersions, gv, p.Fset, id.Pos())
				}
			}
		}
	}
	return r, loadErrors, nil
}

func groupVersionOf(rel string) (string, bool) {
	return rel, len(strings.Split(rel, "/")) == 2
}

func checkKeep(root string) error {
	refs, loadErrors, err := collectKeepRefs(root)
	if err != nil {
		return err
	}
	if len(loadErrors) > 0 {
		msg := "the js build of ./packages/... does not type-check against the mirrors, so the references cannot be read:\n  " + strings.Join(loadErrors, "\n  ")
		if hints := missingClientHints(loadErrors); len(hints) > 0 {
			msg += "\nthe js build needs a client that the narrowed client-go lacks:\n  " + strings.Join(hints, "\n  ")
		}
		return fmt.Errorf("%s", msg)
	}
	upstream, err := upstreamInformerVersions(root, keptAPIs)
	if err != nil {
		return err
	}
	if problems := diffKeep(keptAPIs, refs, upstream, keptWithoutReference); len(problems) > 0 {
		return fmt.Errorf("keptAPIs in scripts/mirror/keep.go disagrees with what the js build references:\n  %s", strings.Join(problems, "\n  "))
	}
	fmt.Println("mirror: keptAPIs matches the js build's references")
	return nil
}
