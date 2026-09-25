package crdreconcile

import (
	"context"
	"sync"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/apiextensions-apiserver/pkg/controller/apiapproval"
	"k8s.io/apiextensions-apiserver/pkg/controller/establish"
	"k8s.io/apiextensions-apiserver/pkg/controller/nonstructuralschema"
	"k8s.io/apiextensions-apiserver/pkg/controller/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	drainPoll = 50 * time.Millisecond
	listPage  = 500
	workers   = 2
)

var QueueNames = []string{
	"crd_naming_condition_controller",
	"crdEstablishing",
	"non_structural_schema_condition_controller",
	"kubernetes_api_approval_conformant_condition_controller",
}

type Deps struct {
	Client  clientset.Interface
	Kine    *kine.Client
	Drained func() bool
}

func NeedsConditions(crd *apiextensionsv1.CustomResourceDefinition) bool {
	return crd.DeletionTimestamp == nil && !apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established)
}

func NeedsFinalize(crd *apiextensionsv1.CustomResourceDefinition) bool {
	return crd.DeletionTimestamp != nil && apihelpers.CRDHasFinalizer(crd, apiextensionsv1.CustomResourceCleanupFinalizer)
}

func markEstablished(ctx context.Context, d Deps, crds []*apiextensionsv1.CustomResourceDefinition) {
	for _, crd := range crds {
		if !NeedsConditions(crd) || !apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.NamesAccepted) {
			continue
		}
		next := crd.DeepCopy()
		apihelpers.SetCRDCondition(next, apiextensionsv1.CustomResourceDefinitionCondition{
			Type:    apiextensionsv1.Established,
			Status:  apiextensionsv1.ConditionTrue,
			Reason:  "InitialNamesAccepted",
			Message: "the initial names have been accepted",
		})
		if _, err := d.Client.ApiextensionsV1().CustomResourceDefinitions().UpdateStatus(ctx, next, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
			println("crdreconcile: establish", next.Name, "failed:", err.Error())
		}
	}
}

func Conditions(ctx context.Context, d Deps, crds []*apiextensionsv1.CustomResourceDefinition, budget time.Duration) {
	markEstablished(ctx, d, crds)
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	snap := &snapshotInformer{SharedIndexInformer: cache.NewSharedIndexInformer(nil, &apiextensionsv1.CustomResourceDefinition{}, 0, cache.Indexers{})}
	factory := informers.NewSharedInformerFactory(d.Client, 0)
	factory.InformerFor(&apiextensionsv1.CustomResourceDefinition{}, func(clientset.Interface, time.Duration) cache.SharedIndexInformer { return snap })
	lister := factory.Apiextensions().V1().CustomResourceDefinitions()
	naming := status.NewNamingConditionController(klog.FromContext(ctx), lister, d.Client.ApiextensionsV1())
	establishing := establish.NewEstablishingController(lister, d.Client.ApiextensionsV1())
	nonStructural := nonstructuralschema.NewConditionController(lister, d.Client.ApiextensionsV1())
	approval := apiapproval.NewKubernetesAPIApprovalPolicyConformantConditionController(lister, d.Client.ApiextensionsV1())
	for _, crd := range crds {
		snap.GetIndexer().Add(crd)
	}
	snap.replay(crds)
	runs := []func(context.Context){
		naming.RunWithContext,
		establishing.RunWithContext,
		func(ctx context.Context) { nonStructural.RunWithContext(workers, ctx) },
		func(ctx context.Context) { approval.RunWithContext(workers, ctx) },
	}
	var wg sync.WaitGroup
	for _, run := range runs {
		wg.Add(1)
		go func() { defer wg.Done(); run(ctx) }()
	}
	for {
		select {
		case <-ctx.Done():
			cancel()
			wg.Wait()
			return
		case <-time.After(drainPoll):
		}
		if establishFromClient(ctx, d, snap, establishing, crds) && d.Drained() {
			break
		}
	}
	cancel()
	wg.Wait()
}

func establishFromClient(ctx context.Context, d Deps, snap *snapshotInformer, establishing *establish.EstablishingController, crds []*apiextensionsv1.CustomResourceDefinition) bool {
	done := true
	for _, crd := range crds {
		latest, err := d.Client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if err != nil {
			done = false
			continue
		}
		_ = snap.GetIndexer().Update(latest)
		if !NeedsConditions(latest) {
			continue
		}
		done = false
		if !apihelpers.IsCRDConditionTrue(latest, apiextensionsv1.NamesAccepted) {
			continue
		}
		establishing.QueueCRD(latest.Name, 0)
		apihelpers.SetCRDCondition(latest, apiextensionsv1.CustomResourceDefinitionCondition{
			Type:    apiextensionsv1.Established,
			Status:  apiextensionsv1.ConditionTrue,
			Reason:  "InitialNamesAccepted",
			Message: "the initial names have been accepted",
		})
		if _, err := d.Client.ApiextensionsV1().CustomResourceDefinitions().UpdateStatus(ctx, latest, metav1.UpdateOptions{}); err != nil {
			continue
		}
	}
	return done
}

func Finalize(ctx context.Context, d Deps, crd *apiextensionsv1.CustomResourceDefinition) error {
	crd = crd.DeepCopy()
	apihelpers.SetCRDCondition(crd, apiextensionsv1.CustomResourceDefinitionCondition{
		Type:    apiextensionsv1.Terminating,
		Status:  apiextensionsv1.ConditionTrue,
		Reason:  "InstanceDeletionInProgress",
		Message: "CustomResource deletion is in progress",
	})
	updated, err := d.Client.ApiextensionsV1().CustomResourceDefinitions().UpdateStatus(ctx, crd, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := "/registry/" + crd.Spec.Group + "/" + crd.Spec.Names.Plural + "/"
	for {
		kvs, _, _, err := d.Kine.List(ctx, prefix, "", listPage)
		if err != nil {
			return err
		}
		if len(kvs) == 0 {
			break
		}
		for _, kv := range kvs {
			if _, err := d.Kine.Delete(ctx, kv.Key, kv.ModRevision); err != nil {
				return err
			}
		}
	}
	apihelpers.CRDRemoveFinalizer(updated, apiextensionsv1.CustomResourceCleanupFinalizer)
	_, err = d.Client.ApiextensionsV1().CustomResourceDefinitions().UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
		return nil
	}
	return err
}
