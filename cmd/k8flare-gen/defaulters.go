package main

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// defaulterPackage describes one API group's real upstream versioned
// defaulting package. Go import paths can't be derived mechanically from a
// schema.GroupVersion (the core group's package lives at .../apis/core/v1,
// not .../apis/v1; some groups have no versioned defaulters package at
// all), so this table is hand-maintained here -- the smallest possible
// piece of hand-written mapping, versus hand-writing the registration calls
// themselves in pkg/apiserver/scheme.go as before this generator existed.
//
// A GroupVersion present in apidef.Table but absent here (e.g. node.k8s.io/v1,
// which has no SetDefaults_* functions upstream -- RuntimeClass has nothing
// to default) is silently skipped: it contributes no import and no
// registration call.
type defaulterPackage struct {
	gv         schema.GroupVersion
	alias      string
	importPath string
}

var defaulterPackages = []defaulterPackage{
	{schema.GroupVersion{Group: "", Version: "v1"}, "corev1defaults", "k8s.io/kubernetes/pkg/apis/core/v1"},
	{schema.GroupVersion{Group: "coordination.k8s.io", Version: "v1"}, "coordinationv1defaults", "k8s.io/kubernetes/pkg/apis/coordination/v1"},
	{schema.GroupVersion{Group: "storage.k8s.io", Version: "v1"}, "storagev1defaults", "k8s.io/kubernetes/pkg/apis/storage/v1"},
	{schema.GroupVersion{Group: "resource.k8s.io", Version: "v1"}, "resourcev1defaults", "k8s.io/kubernetes/pkg/apis/resource/v1"},
	{schema.GroupVersion{Group: "apps", Version: "v1"}, "appsv1defaults", "k8s.io/kubernetes/pkg/apis/apps/v1"},
	{schema.GroupVersion{Group: "discovery.k8s.io", Version: "v1"}, "discoveryv1defaults", "k8s.io/kubernetes/pkg/apis/discovery/v1"},
	{schema.GroupVersion{Group: "networking.k8s.io", Version: "v1"}, "networkingv1defaults", "k8s.io/kubernetes/pkg/apis/networking/v1"},
	{schema.GroupVersion{Group: "batch", Version: "v1"}, "batchv1defaults", "k8s.io/kubernetes/pkg/apis/batch/v1"},
	// policy/v1 (PodDisruptionBudget) and node.k8s.io/v1 (RuntimeClass)
	// have no versioned defaulting package upstream -- both are
	// omitted deliberately, not by oversight.
}

// genDefaulters writes pkg/apiserver/zz_generated_defaulters.go: one import
// + one RegisterDefaults(Scheme) call per apidef.Table GroupVersion that has
// a known defaulterPackage entry.
func genDefaulters(root string) error {
	known := make(map[schema.GroupVersion]defaulterPackage, len(defaulterPackages))
	for _, p := range defaulterPackages {
		known[p.gv] = p
	}

	var used []defaulterPackage
	for _, gv := range apidef.GroupVersions() {
		if p, ok := known[gv]; ok {
			used = append(used, p)
		}
	}
	sort.Slice(used, func(i, j int) bool { return used[i].importPath < used[j].importPath })

	var b strings.Builder
	fmt.Fprintf(&b, "// %s\npackage apiserver\n\nimport (\n\t\"fmt\"\n\n", generatedHeader)
	for _, p := range used {
		fmt.Fprintf(&b, "\t%s %q\n", p.alias, p.importPath)
	}
	b.WriteString(")\n\n")

	b.WriteString("// registerVersionedDefaults registers every API group's real upstream\n")
	b.WriteString("// versioned defaulting functions (k8s.io/kubernetes/pkg/apis/<group>/<version>)\n")
	b.WriteString("// onto Scheme, so Scheme.Default(obj) (called from ApplyDefaults, defaults.go)\n")
	b.WriteString("// applies them -- generated from apidef.Table's GroupVersions crossed with\n")
	b.WriteString("// cmd/k8flare-gen/defaulters.go's defaulterPackage list; a GroupVersion with no\n")
	b.WriteString("// known defaulters package (e.g. node.k8s.io/v1) is skipped.\n")
	b.WriteString("func registerVersionedDefaults() error {\n")
	for _, p := range used {
		fmt.Fprintf(&b, "\tif err := %s.RegisterDefaults(Scheme); err != nil {\n", p.alias)
		fmt.Fprintf(&b, "\t\treturn fmt.Errorf(%q, err)\n", "register "+p.gv.String()+" defaults: %w")
		b.WriteString("\t}\n")
	}
	b.WriteString("\treturn nil\n}\n")

	return writeGoFile(root+"/pkg/apiserver/zz_generated_defaulters.go", []byte(b.String()))
}
