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
	{schema.GroupVersion{Group: "apps", Version: "v1"}, "appsv1defaults", "k8s.io/kubernetes/pkg/apis/apps/v1"},
	{schema.GroupVersion{Group: "batch", Version: "v1"}, "batchv1defaults", "k8s.io/kubernetes/pkg/apis/batch/v1"},
	// scheduling.k8s.io/v1 (PriorityClass): upstream's SetDefaults_PriorityClass
	// fills in spec.preemptionPolicy (defaults to PreemptLowerPriority) when a
	// created PriorityClass omits it -- needed so pkg/apiserver/priority.go's
	// resolution copies a real, non-nil PreemptionPolicy onto a Pod exactly
	// like a real cluster would, not just the numeric Value.
	{schema.GroupVersion{Group: "scheduling.k8s.io", Version: "v1"}, "schedulingv1defaults", "k8s.io/kubernetes/pkg/apis/scheduling/v1"},
	// policy/v1 (PodDisruptionBudget) and node.k8s.io/v1 (RuntimeClass)
	// have no versioned defaulting package upstream -- both are
	// omitted deliberately, not by oversight.
	//
	// coordination.k8s.io/v1 (Lease), storage.k8s.io/v1 (StorageClass/
	// CSIDriver/CSINode), resource.k8s.io/v1 (DRA, stub types),
	// discovery.k8s.io/v1 (EndpointSlice), and networking.k8s.io/v1
	// (Ingress/IngressClass/NetworkPolicy/ServiceCIDR) are deliberately
	// NOT in this list, even though upstream has a versioned defaulters
	// package for each: every write path that creates one of these types
	// in this apiserver already produces a fully-formed object without
	// relying on Scheme.Default() to fill anything in (Leases are written
	// by the k3s agent/kubelet with explicit HolderIdentity/RenewTime;
	// StorageClass is bootstrapped as a complete literal,
	// pkg/apiserver/pvcbind.go's BootstrapStorageClasses; EndpointSlice is
	// written by the real endpointslice controller, which sets its own
	// fields explicitly; the Ingress/NetworkPolicy/ServiceCIDR/DRA types
	// have no controller in this project acting on them at all yet -- see
	// apidef.Table's comments). Measured
	// (docs/platform-verification.md, apiserver Loader-cap headroom
	// investigation): importing all 8 upstream defaulters packages costs
	// ~4.4MiB of the Loader's 64MiB cap; dropping just these 5 unused
	// ones recovers ~1.84MiB of that with `go test ./pkg/apiserver/...`
	// still green, at effectively zero behavioral cost. If a future real
	// controller starts relying on one of these five groups' server-side
	// defaulting, add it back here.
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
