//go:build js && wasm && schedwidth

// schedwidth build: the scheduler WASM binary is built with -tags
// schedwidth against go.wasm.mod, whose k8s.io/client-go replace points
// at the width-pruned .build/clientgo-lean-mirror (kubernetes.Interface
// has only the 9 methods pkg/clientgo-lean-overlays/kubernetes/
// clientset_schedwidth.go declares -- see that file's doc comment for
// the full accounting). CoreV1/AppsV1 come from the base Clientset
// (clientset.go, untagged for width) and StorageV1/ResourceV1/PolicyV1
// from SchedulerClientset (scheduler.go, `!leanwidth`-tagged so it
// already applies here too, unchanged) -- both real. The remaining 4
// methods the schedwidth Interface declares but neither of those
// provides are permanent panic stubs here, same reasoning as
// stubs_leanwidth.go's SchedulingV1alpha2 (the job controller's PodGroup
// informer is never started): ResourceV1beta2's only caller
// (scheduler.go's DeviceTaintRules construction) is gated behind the
// DRADeviceTaintRules feature, Beta/Default:false in v1.36.2-k3s1;
// EventsV1's only caller (client-go/tools/events'
// eventBroadcasterAdapterImpl) only invokes it if a preceding
// Discovery().ServerResourcesForGroupVersion check SUCCEEDS -- and this
// build's Discovery() (below) deliberately reports events.k8s.io as
// unavailable, so the adapter falls back to the real CoreV1 events path
// instead. Discovery() itself must NOT panic: the adapter probes it
// unconditionally at RunScheduler startup -- confirmed live 2026-07-10,
// the first sched dynamic-worker boot died on exactly that panic.
package clientset

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	discovery "k8s.io/client-go/discovery"
	eventsv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	resourcev1beta2 "k8s.io/client-go/kubernetes/typed/resource/v1beta2"
	schedulingv1alpha2 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha2"
)

// errDiscovery answers the one discovery probe the scheduler path makes
// (client-go/tools/events' NewEventBroadcasterAdapterWithContext testing
// for events.k8s.io/v1) with an error, which that caller treats as
// "events/v1 unavailable, use the core-events fallback" -- see its
// `if err == nil` in tools/events/event_broadcaster.go. Every OTHER
// DiscoveryInterface method panics via the nil embedded interface: no
// other discovery call site exists on this build's call graph (grep),
// and a new one appearing should fail loudly, not silently.
type errDiscovery struct {
	discovery.DiscoveryInterface // nil: any method not overridden panics
}

func (errDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	return nil, fmt.Errorf("leanclient: discovery pruned in the schedwidth build (probe for %s answered as unavailable)", groupVersion)
}

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	return errDiscovery{}
}

func (c *Clientset) SchedulingV1alpha2() schedulingv1alpha2.SchedulingV1alpha2Interface {
	panic("leanclient: SchedulingV1alpha2 not implemented (PodGroup informer is never started by this repo's scheduler)")
}

func (c *Clientset) ResourceV1beta2() resourcev1beta2.ResourceV1beta2Interface {
	panic("leanclient: ResourceV1beta2 not implemented (DeviceTaintRules gated behind DRADeviceTaintRules, not enabled)")
}

func (c *Clientset) EventsV1() eventsv1.EventsV1Interface {
	panic("leanclient: EventsV1 not implemented (unreachable: Discovery() reports events.k8s.io unavailable, so tools/events uses the CoreV1 fallback)")
}
