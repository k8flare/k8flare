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

func Conditions(ctx context.Context, d Deps, crds []*apiextensionsv1.CustomResourceDefinition, budget time.Duration) {
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
	for _, crd := range crds {
		if NeedsConditions(crd) && apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.NamesAccepted) {
			establishing.QueueCRD(crd.Name, 0)
		}
	}
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
	quiet := 0
	for quiet < 2 {
		select {
		case <-ctx.Done():
			quiet = 2
			continue
		case <-time.After(drainPoll):
		}
		if d.Drained() {
			quiet++
		} else {
			quiet = 0
		}
	}
	cancel()
	wg.Wait()
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
