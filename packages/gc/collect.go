package gc

import (
	"context"
	"encoding/json"
	"fmt"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
)

const (
	orphanFinalizer     = metav1.FinalizerOrphanDependents
	foregroundFinalizer = metav1.FinalizerDeleteDependents
)

type Result struct {
	Items   int `json:"items"`
	Deleted int `json:"deleted"`
	Patched int `json:"patched"`
}

type collector struct {
	dynamic dynamic.Interface
	mapper  meta.RESTMapper
	graph   *graph
	result  *Result
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
	for _, it := range g.items {
		if err := c.sync(ctx, it); err != nil {
			println("gc:", it.key, "failed:", err.Error())
		}
	}
	return c.result, nil
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
	for _, dep := range c.graph.dependents[it.UID] {
		if blocks(dep, it.UID) {
			return nil
		}
	}
	return c.removeFinalizer(ctx, it, foregroundFinalizer)
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
	for _, ref := range it.OwnerReferences {
		owner, ok := g.byUID[ref.UID]
		switch {
		case !ok:
			stale[ref.UID] = true
		case owner.DeletionTimestamp != nil && owner.hasFinalizer(foregroundFinalizer):
			waiting++
		default:
			solid++
		}
	}
	return solid, waiting, stale
}

func (c *collector) checkOwners(ctx context.Context, it *item) error {
	if len(it.OwnerReferences) == 0 {
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
		return c.deleteItem(ctx, it, metav1.DeletePropagationForeground)
	}
	return c.deleteItem(ctx, it, c.propagationFor(it))
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
	for _, ref := range dep.OwnerReferences {
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
	c.result.Deleted++
	return nil
}

func (c *collector) dropOwners(ctx context.Context, it *item, remove map[types.UID]bool) error {
	kept := make([]metav1.OwnerReference, 0, len(it.OwnerReferences))
	for _, ref := range it.OwnerReferences {
		if !remove[ref.UID] {
			kept = append(kept, ref)
		}
	}
	if len(kept) == len(it.OwnerReferences) {
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
	if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("patch %s: %w", it.key, err)
	}
	c.result.Patched++
	return nil
}
