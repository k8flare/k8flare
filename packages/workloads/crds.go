package workloads

import (
	"context"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/apiextensions-apiserver/pkg/controller/apiapproval"
	"k8s.io/apiextensions-apiserver/pkg/controller/establish"
	"k8s.io/apiextensions-apiserver/pkg/controller/nonstructuralschema"
	"k8s.io/apiextensions-apiserver/pkg/controller/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type CRDResult struct {
	CRDs    int  `json:"crds"`
	Drained bool `json:"drained"`
}

func SyncCRDs(ctx context.Context, client clientset.Interface) (*CRDResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	objs, err := list(ctx, func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
		return client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, o)
	})
	if err != nil {
		return nil, err
	}
	factory := informers.NewSharedInformerFactory(client, 0)
	snap := newSnapshotInformer(&apiextensionsv1.CustomResourceDefinition{})
	factory.InformerFor(&apiextensionsv1.CustomResourceDefinition{}, func(clientset.Interface, time.Duration) cache.SharedIndexInformer { return snap })
	crds := factory.Apiextensions().V1().CustomResourceDefinitions()
	naming := status.NewNamingConditionController(klog.FromContext(ctx), crds, client.ApiextensionsV1())
	establishing := establish.NewEstablishingController(crds, client.ApiextensionsV1())
	nonStructural := nonstructuralschema.NewConditionController(crds, client.ApiextensionsV1())
	apiApproval := apiapproval.NewKubernetesAPIApprovalPolicyConformantConditionController(crds, client.ApiextensionsV1())
	snap.fill(objs)
	snap.replay(objs)
	for _, o := range objs {
		crd := o.(*apiextensionsv1.CustomResourceDefinition)
		if apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.NamesAccepted) && !apihelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established) {
			establishing.QueueCRD(crd.Name, 0)
		}
	}
	runs := []func(context.Context){
		naming.RunWithContext,
		establishing.RunWithContext,
		func(ctx context.Context) { nonStructural.RunWithContext(workers, ctx) },
		func(ctx context.Context) { apiApproval.RunWithContext(workers, ctx) },
	}
	done := make(chan struct{}, len(runs))
	for _, run := range runs {
		go func() { run(ctx); done <- struct{}{} }()
	}
	result := &CRDResult{CRDs: len(objs), Drained: drain()}
	cancel()
	for range runs {
		<-done
	}
	return result, nil
}
