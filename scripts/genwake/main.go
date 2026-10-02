package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/k8flare/k8flare/scripts/internal/upstream"
	"golang.org/x/tools/go/packages"
)

const (
	needsTarget        = "packages/workloads/zz_generated_wake.go"
	prefixesTestTarget = "packages/workloads/zz_generated_wake_test.go"
	prefixesTSTarget   = "packages/cluster-store/src/zz_generated_wake.ts"
)

func main() {
	root, err := upstream.RepoRoot()
	check(err)
	kinds, err := loadKinds(filepath.Join(root, servedRegistryFile))
	check(err)
	pkgs, err := load(root)
	check(err)
	derived, err := extract(pkgs, kinds)
	check(err)
	for _, source := range dynamicInformers {
		resources, err := quotaResources(filepath.Join(root, source.file), source.function)
		check(err)
		for _, r := range resources {
			derived.add(source.controller, r)
		}
	}
	check(applyOutsideConstructors(derived, outsideConstructors))
	prefixes, err := workloadPrefixes(derived, outsideConstructors, routedElsewhere, append(append([]string{}, listedWithoutEventHandler...), wakeWithheld...), extraWorkloadPrefixes)
	check(err)
	goNeeds, err := renderNeeds(derived)
	check(err)
	goPrefixes, err := renderPrefixesTest(prefixes)
	check(err)
	write(root, needsTarget, goNeeds)
	write(root, prefixesTestTarget, goPrefixes)
	write(root, prefixesTSTarget, renderPrefixesTS(prefixes))
}

func load(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:  root,
		Env:  append(os.Environ(), "CGO_ENABLED=0"),
	}
	pkgs, err := packages.Load(cfg, shardPatterns...)
	if err != nil {
		return nil, err
	}
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			return nil, fmt.Errorf("%s: %v", pkg.PkgPath, e)
		}
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no package matches %v", shardPatterns)
	}
	return pkgs, nil
}

func write(root, target string, content []byte) {
	path := filepath.Join(root, target)
	check(os.WriteFile(path, content, 0o644))
	fmt.Println("genwake: wrote", path)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "genwake:", err)
		os.Exit(1)
	}
}
