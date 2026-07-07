//go:build js && wasm && !leanwidth

// SchedulerClientset extends Clientset with a real StorageV1/ResourceV1/
// PolicyV1, for the real, unmodified upstream kube-scheduler's WASM fork
// (see docs/platform-verification.md's S8 kube-scheduler-wasm-fork
// entry). This lives in its own !leanwidth-tagged file -- not added to
// Clientset/clientset.go directly -- so the `-tags leanwidth` KCM build
// (which never needs any of the three, see clientset_leanwidth.go's own
// doc comment) stays byte-for-byte unaffected: a build-tag exclusion, not
// just an unreferenced symbol, is what keeps their typed-client packages
// out of that binary entirely (this repo's own README already documents
// that merely referencing a type -- even one never called -- links its
// package). (A `-tags leanwidth,schedwidth` attempt to also narrow
// kubernetes.Interface for the scheduler was tried and reverted -- see
// clientset_leanwidth.go's doc comment for why.)
//
// All three groups are needed for real, not as panic stubs, confirmed by
// reading pkg/scheduler's actual call sites (not assumed):
//   - StorageV1: pkg/scheduler.New unconditionally builds
//     nodevolumelimits.NewCSIManager(informerFactory.Storage().V1().
//     CSINodes().Lister()), and framework/plugins/volumebinding.New (its
//     registry entry is still linked -- only DynamicResources is dropped
//     from the js registry, see third_party/k8s-js-overlays/
//     scheduler-registry_js.go) references CSIDrivers/
//     CSIStorageCapacities/StorageClasses too, even though sched.go's
//     profile config disables the VolumeBinding *plugin* -- the registry
//     map literal still needs volumebinding's New function body to
//     compile.
//   - ResourceV1: DynamicResourceAllocation is GA and LockToDefault:true
//     in v1.36.2-k3s1 (verified by reading pkg/features/kube_features.go,
//     not assumed), so scheduler.go's DRA construction block
//     (ResourceClaims/ResourceSlices/DeviceClasses informers) always
//     runs -- it cannot be gated off, unlike an earlier draft of this
//     change assumed.
//   - PolicyV1: framework/preemption/{preemption,podgrouppreemption,
//     executor}.go unconditionally build a PodDisruptionBudget lister
//     (DefaultPreemption is not in sched.go's disabled-plugins list).
package clientset

import (
	kubernetes "k8s.io/client-go/kubernetes"
	policyv1client "k8s.io/client-go/kubernetes/typed/policy/v1"
	resourcev1client "k8s.io/client-go/kubernetes/typed/resource/v1"
	storagev1client "k8s.io/client-go/kubernetes/typed/storage/v1"
	restclient "k8s.io/client-go/rest"

	"github.com/k8flare/k8flare/pkg/leanclient/gen/policyv1"
	"github.com/k8flare/k8flare/pkg/leanclient/gen/resourcev1"
	"github.com/k8flare/k8flare/pkg/leanclient/gen/storagev1"
)

// SchedulerClientset embeds *Clientset (promoting its 5 real groups plus
// stubs.go's panic stubs for everything else) and shadows StorageV1/
// ResourceV1/PolicyV1 with real implementations.
type SchedulerClientset struct {
	*Clientset
	storage  *storagev1.Client
	resource *resourcev1.Client
	policy   *policyv1.Client
}

var _ storagev1client.StorageV1Interface = (*storagev1.Client)(nil)
var _ resourcev1client.ResourceV1Interface = (*resourcev1.Client)(nil)
var _ policyv1client.PolicyV1Interface = (*policyv1.Client)(nil)
var _ kubernetes.Interface = (*SchedulerClientset)(nil)

// NewSchedulerClientsetForConfig mirrors NewForConfig's shape, adding the
// three extra groups pkg/scheduler needs for real.
func NewSchedulerClientsetForConfig(cfg *restclient.Config) (*SchedulerClientset, error) {
	base, err := NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	storage, err := storagev1.New(cfg)
	if err != nil {
		return nil, err
	}
	resource, err := resourcev1.New(cfg)
	if err != nil {
		return nil, err
	}
	policy, err := policyv1.New(cfg)
	if err != nil {
		return nil, err
	}
	return &SchedulerClientset{Clientset: base, storage: storage, resource: resource, policy: policy}, nil
}

func (c *SchedulerClientset) StorageV1() storagev1client.StorageV1Interface { return c.storage }

func (c *SchedulerClientset) ResourceV1() resourcev1client.ResourceV1Interface { return c.resource }

func (c *SchedulerClientset) PolicyV1() policyv1client.PolicyV1Interface { return c.policy }
