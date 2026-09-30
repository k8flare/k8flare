package events

import (
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/apis/events"
	"k8s.io/kubernetes/pkg/registry/core/event"
)

func init() {
	utilruntime.Must(events.AddToScheme(registry.InternalScheme))
	utilruntime.Must(eventsv1.AddToScheme(registry.InternalScheme))
	registry.Upstreams[schema.GroupResource{Group: "events.k8s.io", Resource: "events"}] = registry.Upstream{Strategy: event.Strategy}
}
