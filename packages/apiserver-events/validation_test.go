package events

import (
	"testing"

	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpstreamRejectsInvalid(t *testing.T) {
	store := registrytest.Store(t, schema.GroupVersion{Group: "events.k8s.io", Version: "v1"}, metav1.APIResource{Name: "events", Kind: "Event", Namespaced: true})
	obj := &eventsv1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: "default"}}
	err := registrytest.Create(store, obj)
	registrytest.RequireFieldError(t, err, "eventTime", "Required value")
}
