package workloads

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/kubernetes/pkg/controller/namespace/deletion"
)

const (
	namespaceRetry    = 2 * time.Second
	namespaceMaxRetry = time.Minute
	namespaceBatch    = 40
	namespaceWorkers  = 8
	namespaceProbes   = 8
)

var namespaceBudget = 60 * time.Second

type deleteOutcome struct {
	terminating bool
	deleted     bool
	remaining   bool
	retry       time.Duration
}

func remainingAfter(wait time.Duration) deleteOutcome {
	if wait <= 0 {
		wait = namespaceRetry
	}
	return deleteOutcome{terminating: true, remaining: true, retry: wait}
}

func namespaceBackoff(ns *v1.Namespace) time.Duration {
	return min(max(time.Since(ns.DeletionTimestamp.Time), namespaceRetry), namespaceMaxRetry)
}

type NamespaceResult struct {
	Terminating int      `json:"terminating"`
	Deleted     int      `json:"deleted"`
	Remaining   int      `json:"remaining"`
	NextMs      int64    `json:"nextMs"`
	Names       []string `json:"names,omitempty"`
}

type NamespaceMessage struct {
	Kind  string   `json:"kind"`
	Key   string   `json:"key"`
	Names []string `json:"names"`
}

func NamespaceNames(msgs []NamespaceMessage) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, msg := range msgs {
		if msg.Kind == "change" && strings.HasPrefix(msg.Key, "/registry/namespaces/") {
			add(strings.TrimPrefix(msg.Key, "/registry/namespaces/"))
		}
		if msg.Kind == "retry" {
			for _, name := range msg.Names {
				add(name)
			}
		}
	}
	return names
}

type Deleter struct {
	client kubernetes.Interface
	meta   metadata.Interface
	mu     sync.Mutex
	busy   map[string]bool
}

func NewDeleter(client kubernetes.Interface, meta metadata.Interface) *Deleter {
	return &Deleter{client: client, meta: meta, busy: map[string]bool{}}
}

func (d *Deleter) claim(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.busy[name] {
		return false
	}
	d.busy[name] = true
	return true
}

func (d *Deleter) release(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.busy, name)
}

func (d *Deleter) deleteContent(ctx context.Context, name string) error {
	resources, discoveryErr := d.client.Discovery().ServerPreferredNamespacedResources()
	sweep := resourcesWithContent(ctx, d.meta, name, resources)
	if len(sweep) == 0 {
		return fmt.Errorf("no namespaced resources discovered: %v", discoveryErr)
	}
	return deletion.NewNamespacedResourcesDeleter(
		ctx,
		d.client.CoreV1().Namespaces(),
		d.meta,
		d.client.CoreV1(),
		func() ([]*metav1.APIResourceList, error) { return sweep, discoveryErr },
		v1.FinalizerKubernetes,
	).Delete(ctx, name)
}

func resourcesWithContent(ctx context.Context, meta metadata.Interface, ns string, lists []*metav1.APIResourceList) []*metav1.APIResourceList {
	keep := make([][]bool, len(lists))
	var wg sync.WaitGroup
	slots := make(chan struct{}, namespaceProbes)
	for i, list := range lists {
		keep[i] = make([]bool, len(list.APIResources))
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		for j, resource := range list.APIResources {
			if err != nil || (gv.Group == "" && resource.Name == "pods") || !slices.Contains(resource.Verbs, "list") {
				keep[i][j] = true
				continue
			}
			slots <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-slots }()
				items, err := meta.Resource(gv.WithResource(resource.Name)).Namespace(ns).List(ctx, metav1.ListOptions{Limit: 1})
				keep[i][j] = err != nil || len(items.Items) > 0
			}()
		}
	}
	wg.Wait()
	var sweep []*metav1.APIResourceList
	for i, list := range lists {
		kept := &metav1.APIResourceList{TypeMeta: list.TypeMeta, GroupVersion: list.GroupVersion}
		for j, resource := range list.APIResources {
			if keep[i][j] {
				kept.APIResources = append(kept.APIResources, resource)
			}
		}
		if len(kept.APIResources) > 0 {
			sweep = append(sweep, kept)
		}
	}
	return sweep
}

func listTerminating(ctx context.Context, client kubernetes.Interface) ([]string, error) {
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	type item struct {
		name string
		at   time.Time
	}
	var items []item
	for i := range list.Items {
		if list.Items[i].DeletionTimestamp == nil {
			continue
		}
		items = append(items, item{name: list.Items[i].Name, at: list.Items[i].DeletionTimestamp.Time})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].at.After(items[j].at)
	})
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].name
	}
	return names, nil
}

func pickTerminating(names []string, limit int) (selected []string, more bool) {
	if limit <= 0 || len(names) <= limit {
		return names, false
	}
	return names[:limit], true
}

func mergeTerminating(hinted, listed []string) []string {
	seen := map[string]bool{}
	var names []string
	for _, name := range hinted {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, name := range listed {
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func (d *Deleter) DeleteTerminating(ctx context.Context, client kubernetes.Interface, names []string) (*NamespaceResult, error) {
	listed, err := listTerminating(ctx, client)
	if err != nil {
		return nil, err
	}
	selected, more := pickTerminating(mergeTerminating(names, listed), namespaceBatch)
	result := &NamespaceResult{}
	done := map[string]bool{}
	budgeted, cancel := context.WithTimeout(ctx, namespaceBudget)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, namespaceWorkers)
	for _, name := range selected {
		if budgeted.Err() != nil {
			mu.Lock()
			result.Remaining++
			result.NextMs = soonest(result.NextMs, namespaceRetry)
			mu.Unlock()
			continue
		}
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			outcome := d.deleteOne(budgeted, client, name)
			mu.Lock()
			defer mu.Unlock()
			if outcome.terminating {
				result.Terminating++
			}
			if outcome.deleted {
				result.Deleted++
				done[name] = true
			}
			if outcome.remaining {
				result.Remaining++
				result.NextMs = soonest(result.NextMs, outcome.retry)
			}
		}()
	}
	wg.Wait()
	if more {
		result.NextMs = soonest(result.NextMs, namespaceRetry)
	}
	if more || result.Remaining > 0 {
		for _, name := range listed {
			if !done[name] {
				result.Names = append(result.Names, name)
			}
		}
	}
	return result, nil
}

func (d *Deleter) deleteOne(ctx context.Context, client kubernetes.Interface, name string) deleteOutcome {
	ns, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return deleteOutcome{}
	}
	if err != nil {
		println("namespaces: get", name, "failed:", err.Error())
		return deleteOutcome{remaining: true, retry: namespaceRetry}
	}
	if ns.DeletionTimestamp == nil {
		return deleteOutcome{}
	}
	if !d.claim(ns.Name) {
		return remainingAfter(namespaceBackoff(ns))
	}
	defer d.release(ns.Name)
	if err := d.deleteContent(ctx, ns.Name); err != nil {
		var remain *deletion.ResourcesRemainingError
		if errors.As(err, &remain) {
			return remainingAfter(max(time.Duration(remain.Estimate)*time.Second, namespaceBackoff(ns)))
		}
		println("namespaces: delete", ns.Name, "failed:", err.Error())
		return remainingAfter(namespaceBackoff(ns))
	}
	return deleteOutcome{terminating: true, deleted: true}
}
