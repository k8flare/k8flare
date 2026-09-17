package customresources

import (
	"context"
	"sync"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"github.com/k8flare/k8flare/packages/crdreconcile"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

const conditionsBudget = 5 * time.Second

type reconciler struct {
	deps    crdreconcile.Deps
	mu      sync.Mutex
	running chan struct{}
}

func pendingWork(crds []*apiextensionsv1.CustomResourceDefinition) (conditions bool, finalize []*apiextensionsv1.CustomResourceDefinition) {
	for _, crd := range crds {
		if crdreconcile.NeedsConditions(crd) {
			conditions = true
		}
		if crdreconcile.NeedsFinalize(crd) {
			finalize = append(finalize, crd)
		}
	}
	return conditions, finalize
}

func (r *reconciler) run(ctx context.Context, crds []*apiextensionsv1.CustomResourceDefinition) bool {
	conditions, finalize := pendingWork(crds)
	if !conditions && len(finalize) == 0 {
		return false
	}
	r.mu.Lock()
	if r.running != nil {
		wait := r.running
		r.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
		}
		return true
	}
	done := make(chan struct{})
	r.running = done
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = nil
		r.mu.Unlock()
		close(done)
	}()
	if conditions {
		queueWork.reset(crdreconcile.QueueNames...)
		crdreconcile.Conditions(ctx, r.deps, crds, conditionsBudget)
	}
	for _, crd := range finalize {
		if err := crdreconcile.Finalize(ctx, r.deps, crd); err != nil {
			println("customresources: finalize", crd.Name, "failed:", err.Error())
		}
	}
	return true
}

func decodeCRDs(kvs []kine.KV) []*apiextensionsv1.CustomResourceDefinition {
	crds := make([]*apiextensionsv1.CustomResourceDefinition, 0, len(kvs))
	for _, crd := range decodeAll(kvs) {
		crds = append(crds, crd)
	}
	return crds
}
