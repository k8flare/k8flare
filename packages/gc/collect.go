package gc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/utils/ptr"
)

const (
	orphanFinalizer     = metav1.FinalizerOrphanDependents
	foregroundFinalizer = metav1.FinalizerDeleteDependents
	eventTTL            = time.Hour
	ConcurrentGCSyncs   = 20
)

type Result struct {
	Items   int `json:"items"`
	Deleted int `json:"deleted"`
	Patched int `json:"patched"`
	Pending int `json:"pending"`
}

type collector struct {
	dynamic dynamic.Interface
	mapper  meta.RESTMapper
	graph   *graph
	result  *Result
	mu      sync.Mutex
}

func Collect(ctx context.Context, client kubernetes.Interface, dyn dynamic.Interface, store *kine.Client) (*Result, error) {
	groups, err := restmapper.GetAPIGroupResources(client.Discovery())
	if err != nil {
		return nil, err
	}
	g, err := loadGraph(ctx, store)
	if err != nil {
		return nil, err
	}
	c := &collector{dynamic: dyn, mapper: restmapper.NewDiscoveryRESTMapper(groups), graph: g, result: &Result{Items: len(g.items)}}
	var wg sync.WaitGroup
	slots := make(chan struct{}, ConcurrentGCSyncs)
	for _, it := range g.items {
		if it.DeletionTimestamp != nil && (it.hasFinalizer(foregroundFinalizer) || it.hasFinalizer(orphanFinalizer)) {
			c.result.Pending++
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(target *item) {
			defer wg.Done()
			defer func() { <-slots }()
			if err := c.sync(ctx, target); err != nil {
				println("gc:", target.key, "failed:", err.Error())
			}
		}(it)
	}
	wg.Wait()
	for _, it := range absentNamespace(g.items) {
		if err := c.deleteItem(ctx, it, metav1.DeletePropagationBackground); err != nil {
			println("gc:", it.key, "namespace gone:", err.Error())
			c.result.Pending++
		}
	}
	if err := c.deleteOrphanKeys(ctx, store, append(append(orphanEventKeys(g), orphanLeaseKeys(g)...), expiredEventKeys(g, time.Now())...)); err != nil {
		return nil, err
	}
	return c.result, nil
}

const orphanEventBatch = 200

func (c *collector) deleteOrphanKeys(ctx context.Context, store *kine.Client, keys []string) error {
	if len(keys) > orphanEventBatch {
		c.result.Pending += len(keys) - orphanEventBatch
		keys = keys[:orphanEventBatch]
	}
	for _, key := range keys {
		if _, err := store.Delete(ctx, key, 0); err != nil && err != kine.ErrNotFound {
			return err
		}
		c.result.Deleted++
	}
	return nil
}

func orphanEventKeys(g *graph) []string {
	return orphanNamespacedKeys(g, g.eventKeys, eventNamespace)
}

func expiredEventKeys(g *graph, now time.Time) []string {
	var out []string
	for _, key := range g.eventKeys {
		at := g.eventAt[key]
		if at.IsZero() || now.Sub(at) < eventTTL {
			continue
		}
		out = append(out, key)
	}
	return out
}

func orphanLeaseKeys(g *graph) []string {
	return orphanNamespacedKeys(g, g.leaseKeys, leaseNamespace)
}

func orphanNamespacedKeys(g *graph, keys []string, namespace func(string) (string, bool)) []string {
	present := map[string]bool{}
	for _, it := range g.items {
		if it.Kind == "Namespace" {
			present[it.Name] = true
		}
	}
	var gone []string
	for _, key := range keys {
		ns, ok := namespace(key)
		if !ok || present[ns] {
			continue
		}
		gone = append(gone, key)
	}
	return gone
}

func absentNamespace(items []*item) []*item {
	present := map[string]bool{}
	for _, it := range items {
		if it.Kind == "Namespace" {
			present[it.Name] = true
		}
	}
	var gone []*item
	for _, it := range items {
		if it.Namespace == "" || present[it.Namespace] {
			continue
		}
		gone = append(gone, it)
	}
	return gone
}

func (c *collector) sync(ctx context.Context, it *item) error {
	switch {
	case it.DeletionTimestamp != nil && it.hasFinalizer(foregroundFinalizer):
		return c.finishForeground(ctx, it)
	case it.DeletionTimestamp != nil && it.hasFinalizer(orphanFinalizer):
		return c.orphanDependents(ctx, it)
	case it.DeletionTimestamp != nil:
		return nil
	}
	return c.checkOwners(ctx, it)
}

func (c *collector) finishForeground(ctx context.Context, it *item) error {
	live, deleting := splitBlockers(c.graph, it)
	for _, dep := range deleting {
		if err := c.unblockOwners(ctx, dep); err != nil {
			return err
		}
	}
	if len(live) > 0 {
		return nil
	}
	return c.removeFinalizer(ctx, it, foregroundFinalizer)
}

func splitBlockers(g *graph, it *item) (live, deleting []*item) {
	for _, dep := range g.dependents[it.UID] {
		if !blocks(dep, it.UID) {
			continue
		}
		if dep.DeletionTimestamp != nil {
			deleting = append(deleting, dep)
		} else {
			live = append(live, dep)
		}
	}
	return
}

func (c *collector) orphanDependents(ctx context.Context, it *item) error {
	for _, dep := range c.graph.dependents[it.UID] {
		if err := c.dropOwners(ctx, dep, map[types.UID]bool{it.UID: true}); err != nil {
			return err
		}
	}
	return c.removeFinalizer(ctx, it, orphanFinalizer)
}

func classify(g *graph, it *item) (solid, waiting int, stale map[types.UID]bool) {
	stale = map[types.UID]bool{}
	for _, ref := range it.owners() {
		owner, ok := g.byUID[ref.UID]
		switch {
		case !ok:
			stale[ref.UID] = true
		case owner.DeletionTimestamp != nil && owner.hasFinalizer(foregroundFinalizer):
			waiting++
			stale[ref.UID] = true
		default:
			solid++
		}
	}
	return solid, waiting, stale
}

func (c *collector) checkOwners(ctx context.Context, it *item) error {
	if len(it.owners()) == 0 {
		return nil
	}
	solid, waiting, stale := classify(c.graph, it)
	if solid > 0 {
		if len(stale) == 0 {
			return nil
		}
		return c.dropOwners(ctx, it, stale)
	}
	if waiting > 0 {
		if c.hasForegroundDependents(it) {
			if err := c.unblockOwners(ctx, it); err != nil {
				return err
			}
		}
		return c.deleteItem(ctx, it, metav1.DeletePropagationForeground)
	}
	return c.deleteItem(ctx, it, c.propagationFor(it))
}

func (c *collector) hasForegroundDependents(it *item) bool {
	for _, dep := range c.graph.dependents[it.UID] {
		if dep.DeletionTimestamp != nil && dep.hasFinalizer(foregroundFinalizer) {
			return true
		}
	}
	return false
}

func (c *collector) unblockOwners(ctx context.Context, it *item) error {
	owners := it.owners()
	refs := make([]metav1.OwnerReference, len(owners))
	copy(refs, owners)
	changed := false
	for i := range refs {
		if refs[i].BlockOwnerDeletion != nil && *refs[i].BlockOwnerDeletion {
			refs[i].BlockOwnerDeletion = ptr.To(false)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := c.patchMeta(ctx, it, map[string]any{"ownerReferences": refs}); err != nil {
		return err
	}
	it.setOwners(refs)
	return nil
}

func (c *collector) propagationFor(it *item) metav1.DeletionPropagation {
	switch {
	case it.hasFinalizer(orphanFinalizer):
		return metav1.DeletePropagationOrphan
	case it.hasFinalizer(foregroundFinalizer):
		return metav1.DeletePropagationForeground
	}
	return metav1.DeletePropagationBackground
}

func blocks(dep *item, owner types.UID) bool {
	for _, ref := range dep.owners() {
		if ref.UID == owner && ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion {
			return true
		}
	}
	return false
}

func (c *collector) resource(it *item) (dynamic.ResourceInterface, error) {
	gv, err := schema.ParseGroupVersion(it.APIVersion)
	if err != nil {
		return nil, err
	}
	mapping, err := c.mapper.RESTMapping(gv.WithKind(it.Kind).GroupKind(), gv.Version)
	if err != nil {
		return nil, err
	}
	if it.Namespace == "" {
		return c.dynamic.Resource(mapping.Resource), nil
	}
	return c.dynamic.Resource(mapping.Resource).Namespace(it.Namespace), nil
}

func (c *collector) deleteItem(ctx context.Context, it *item, policy metav1.DeletionPropagation) error {
	res, err := c.resource(it)
	if err != nil {
		return err
	}
	uid := it.UID
	err = res.Delete(ctx, it.Name, metav1.DeleteOptions{
		PropagationPolicy: &policy,
		Preconditions:     &metav1.Preconditions{UID: &uid},
	})
	if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
		return nil
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.result.Deleted++
	c.mu.Unlock()
	return nil
}

func (c *collector) dropOwners(ctx context.Context, it *item, remove map[types.UID]bool) error {
	owners := it.owners()
	kept := make([]metav1.OwnerReference, 0, len(owners))
	for _, ref := range owners {
		if !remove[ref.UID] {
			kept = append(kept, ref)
		}
	}
	if len(kept) == len(owners) {
		return nil
	}
	return c.patchMeta(ctx, it, map[string]any{"ownerReferences": kept})
}

func (c *collector) removeFinalizer(ctx context.Context, it *item, name string) error {
	kept := make([]string, 0, len(it.Finalizers))
	for _, f := range it.Finalizers {
		if f != name {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(it.Finalizers) {
		return nil
	}
	return c.patchMeta(ctx, it, map[string]any{"finalizers": kept})
}

func (c *collector) patchMeta(ctx context.Context, it *item, fields map[string]any) error {
	res, err := c.resource(it)
	if err != nil {
		return err
	}
	fields["uid"] = it.UID
	fields["resourceVersion"] = it.ResourceVersion
	patch, err := json.Marshal(map[string]any{"metadata": fields})
	if err != nil {
		return err
	}
	_, err = res.Patch(ctx, it.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("patch %s: %w", it.key, err)
	}
	c.mu.Lock()
	c.result.Patched++
	c.mu.Unlock()
	return nil
}
