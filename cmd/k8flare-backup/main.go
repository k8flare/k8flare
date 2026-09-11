// Command k8flare-backup exports a cluster's Kubernetes objects to a file and
// restores them into a cluster.
//
// Why it exists: `wrangler deploy` applies Durable Object migrations without
// prompting, and a tag carrying deleted_classes destroys the storage every
// cluster's state lives in. packages/wasm-build/src/check-migrations.ts now
// refuses to deploy a changed migrations block, so that cannot happen by
// accident -- but nothing could undo it deliberately, because there was no
// export at all (docs/adopter-quickstart.md, TODO.md P1-4).
//
// What it covers and what it does not, stated plainly because a backup tool
// that overstates its scope is worse than none:
//
//   - COVERED: every object served by the cluster's own discovery, in every
//     namespace, as the API returns it.
//   - NOT COVERED: the CA keypairs and the per-cluster token vault. They live
//     in Durable Object facets that the Kubernetes API deliberately does not
//     serve. Restoring this dump into a fresh deployment gives you your
//     workloads back, not your cluster's identity: nodes holding certificates
//     signed by the old CA will not rejoin, and issued cluster tokens change.
//   - NOT COVERED: anything a controller would recreate anyway, if you restore
//     with -skip-generated (the default): Endpoints, EndpointSlices and Events.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// generated names the resources a controller owns and will rebuild from the
// objects that do get restored. Restoring them is not merely redundant: an
// Endpoints object restored ahead of its Pods describes addresses that do not
// exist yet, and the real endpoint controller has to correct it.
var generated = map[string]bool{
	"endpoints":      true,
	"endpointslices": true,
	"events":         true,
}

type record struct {
	Group     string                     `json:"group"`
	Version   string                     `json:"version"`
	Resource  string                     `json:"resource"`
	Namespace string                     `json:"namespace,omitempty"`
	Name      string                     `json:"name"`
	Object    map[string]json.RawMessage `json:"object"`
}

func main() {
	fs := flag.NewFlagSet("k8flare-backup", flag.ExitOnError)
	kubeconfig := fs.String("kubeconfig", os.Getenv("KUBECONFIG"), "path to a kubeconfig for the cluster")
	file := fs.String("file", "", "file to write (dump) or read (restore); - for stdin/stdout")
	skipGenerated := fs.Bool("skip-generated", true, "skip Endpoints, EndpointSlices and Events, which controllers rebuild")
	timeout := fs.Duration("timeout", 10*time.Minute, "overall deadline")
	if len(os.Args) < 2 {
		usage(fs)
	}
	mode := os.Args[1]
	if mode != "dump" && mode != "restore" {
		usage(fs)
	}
	_ = fs.Parse(os.Args[2:])
	if *file == "" {
		fmt.Fprintln(os.Stderr, "-file is required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fatal("build config: %v", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		fatal("dynamic client: %v", err)
	}

	switch mode {
	case "dump":
		disco, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			fatal("discovery client: %v", err)
		}
		if err := dump(ctx, disco, dyn, *file, *skipGenerated); err != nil {
			fatal("dump: %v", err)
		}
	case "restore":
		if err := restore(ctx, dyn, *file, *skipGenerated); err != nil {
			fatal("restore: %v", err)
		}
	}
}

func dump(ctx context.Context, disco discovery.DiscoveryInterface, dyn dynamic.Interface, file string, skipGenerated bool) error {
	lists, err := disco.ServerPreferredResources()
	if err != nil && len(lists) == 0 {
		return fmt.Errorf("discover resources: %w", err)
	}

	out, closeOut, err := openWrite(file)
	if err != nil {
		return err
	}
	defer closeOut()
	enc := json.NewEncoder(out)

	var written, skipped int
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return fmt.Errorf("parse %q: %w", list.GroupVersion, err)
		}
		for _, r := range list.APIResources {
			if strings.Contains(r.Name, "/") || !verbSupported(r.Verbs, "list") {
				continue
			}
			if skipGenerated && generated[r.Name] {
				skipped++
				continue
			}
			gvr := gv.WithResource(r.Name)
			items, err := dyn.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
			if err != nil {
				// A resource the cluster advertises but cannot serve is worth
				// reporting, not worth aborting a backup over.
				fmt.Fprintf(os.Stderr, "warning: list %s: %v\n", gvr, err)
				continue
			}
			for i := range items.Items {
				rec, err := toRecord(gvr, &items.Items[i])
				if err != nil {
					return err
				}
				if err := enc.Encode(rec); err != nil {
					return fmt.Errorf("write %s/%s: %w", rec.Namespace, rec.Name, err)
				}
				written++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "dumped %d object(s)", written)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, ", skipped %d controller-owned resource kind(s)", skipped)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func restore(ctx context.Context, dyn dynamic.Interface, file string, skipGenerated bool) error {
	in, closeIn, err := openRead(file)
	if err != nil {
		return err
	}
	defer closeIn()

	dec := json.NewDecoder(in)
	var created, existed, skipped int
	for {
		var rec record
		if err := dec.Decode(&rec); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("read record: %w", err)
		}
		if skipGenerated && generated[rec.Resource] {
			skipped++
			continue
		}
		obj := &unstructured.Unstructured{Object: map[string]any{}}
		raw, err := json.Marshal(rec.Object)
		if err != nil {
			return fmt.Errorf("%s/%s: re-encode: %w", rec.Namespace, rec.Name, err)
		}
		if err := obj.UnmarshalJSON(raw); err != nil {
			return fmt.Errorf("%s/%s: decode: %w", rec.Namespace, rec.Name, err)
		}
		stripServerFields(obj)

		gvr := schema.GroupVersionResource{Group: rec.Group, Version: rec.Version, Resource: rec.Resource}
		_, err = dyn.Resource(gvr).Namespace(rec.Namespace).Create(ctx, obj, metav1.CreateOptions{})
		switch {
		case err == nil:
			created++
		case apierrors.IsAlreadyExists(err):
			existed++
		default:
			return fmt.Errorf("create %s %s/%s: %w", gvr.Resource, rec.Namespace, rec.Name, err)
		}
	}
	fmt.Fprintf(os.Stderr, "restored %d object(s), %d already present", created, existed)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, ", skipped %d controller-owned", skipped)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// stripServerFields removes what the destination assigns itself. A restored
// object keeps its name, labels, annotations and spec; it does not keep the
// identity of the object it was dumped from, because a UID or resourceVersion
// carried across would either be rejected or, worse, accepted and wrong.
func stripServerFields(obj *unstructured.Unstructured) {
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetGeneration(0)
	obj.SetSelfLink("")
	obj.SetManagedFields(nil)
	obj.SetOwnerReferences(nil)
	unstructured.RemoveNestedField(obj.Object, "status")
	unstructured.RemoveNestedField(obj.Object, "spec", "clusterIP")
	unstructured.RemoveNestedField(obj.Object, "spec", "clusterIPs")
	unstructured.RemoveNestedField(obj.Object, "spec", "nodeName")
}

func toRecord(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) (record, error) {
	raw, err := obj.MarshalJSON()
	if err != nil {
		return record{}, fmt.Errorf("encode %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return record{}, err
	}
	return record{
		Group:     gvr.Group,
		Version:   gvr.Version,
		Resource:  gvr.Resource,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Object:    fields,
	}, nil
}

func verbSupported(verbs metav1.Verbs, want string) bool {
	for _, v := range verbs {
		if v == want {
			return true
		}
	}
	return false
}

func openWrite(path string) (*os.File, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create %s: %w", path, err)
	}
	return f, func() { f.Close() }, nil
}

func openRead(path string) (*os.File, func(), error) {
	if path == "-" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	return f, func() { f.Close() }, nil
}

func usage(fs *flag.FlagSet) {
	fmt.Fprintln(os.Stderr, "usage: k8flare-backup dump|restore -file <path> [-kubeconfig <path>]")
	fs.PrintDefaults()
	os.Exit(2)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "k8flare-backup: "+format+"\n", args...)
	os.Exit(1)
}
