package registry

import (
	"bytes"
	"testing"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	_ "github.com/k8flare/k8flare/packages/apiserver-events"
	"github.com/k8flare/k8flare/packages/apiserver-events/storageconv"
)

func init() { storageconv.Install() }

func TestEventStorageCodecRoundTrip(t *testing.T) {
	in := &eventsv1.Event{
		ObjectMeta:          metav1.ObjectMeta{Name: "web.1", Namespace: "default"},
		Regarding:           corev1.ObjectReference{Kind: "Pod", Name: "web", Namespace: "default"},
		Note:                "started",
		Reason:              "Started",
		Type:                "Normal",
		ReportingController: "test-controller",
		ReportingInstance:   "test-node",
	}
	var buf bytes.Buffer
	codec := eventStorageCodec{}
	if err := codec.Encode(in, &buf); err != nil {
		t.Fatal(err)
	}
	stored, _, err := scheme.Codecs.UniversalDeserializer().Decode(buf.Bytes(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	core, ok := stored.(*corev1.Event)
	if !ok {
		t.Fatalf("stored %T", stored)
	}
	if core.Message != "started" || core.InvolvedObject.Name != "web" || core.Source.Component != "test-controller" || core.ReportingController != "test-controller" {
		t.Fatalf("core %+v", core)
	}
	out := &eventsv1.Event{}
	got, _, err := codec.Decode(buf.Bytes(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	ev := got.(*eventsv1.Event)
	if ev.Note != "started" || ev.Regarding.Name != "web" || ev.Name != "web.1" || ev.ReportingController != "test-controller" {
		t.Fatalf("events %+v", ev)
	}
}

func TestEventStorageCodecReadsCoreEvent(t *testing.T) {
	core := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "svc.2", Namespace: "kube-system"},
		InvolvedObject: corev1.ObjectReference{Kind: "Service", Name: "kubernetes"},
		Message:        "allocated",
		Reason:         "Allocated",
		Type:           "Normal",
	}
	data, err := runtime.Encode(scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion), core)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := eventStorageCodec{}.Decode(data, nil, &eventsv1.Event{})

	if err != nil {
		t.Fatal(err)
	}
	ev := got.(*eventsv1.Event)
	if ev.Note != "allocated" || ev.Regarding.Name != "kubernetes" {
		t.Fatalf("events %+v", ev)
	}
}
