package all

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/k8flare/k8flare/packages/workloads"
)

func TestSyncPublishesTheRootCAOnlyIntoActiveNamespaces(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alive"}, Status: v1.NamespaceStatus{Phase: v1.NamespaceActive}},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "going", DeletionTimestamp: &now}, Status: v1.NamespaceStatus{Phase: v1.NamespaceTerminating}},
	)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"namespaces"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps("alive").Get(context.Background(), "kube-root-ca.crt", metav1.GetOptions{}); err != nil {
		t.Fatalf("active namespace has no root CA: %v", err)
	}
	if _, err := client.CoreV1().ConfigMaps("going").Get(context.Background(), "kube-root-ca.crt", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("terminating namespace got a create: %v", err)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "create" && action.GetResource().Resource == "configmaps" && action.GetNamespace() == "going" {
			t.Fatalf("create attempted in the terminating namespace: %v", action)
		}
	}
}

func TestAConfigMapChangeDuringALongPassWithoutThePublisherIsPublishedBeforeThatPassDrains(t *testing.T) {
	previous := workloads.WatchDialer
	defer func() { workloads.WatchDialer = previous }()
	workloads.WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		return make(chan []byte), func() {}, nil
	}
	client := fake.NewSimpleClientset(
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "spec", UID: "s1"},
			Spec:       v1.ServiceSpec{Selector: map[string]string{"app": "web"}, Ports: []v1.ServicePort{{Port: 80}}},
		},
		&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "spec"}, Status: v1.NamespaceStatus{Phase: v1.NamespaceActive}},
		&v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "spec"}, Data: map[string]string{"ca.crt": ""}},
	)
	blocked := make(chan struct{}, 1)
	release := make(chan struct{})
	slow := slowEndpointsClientset{Interface: client, blocked: blocked, release: release, creates: &atomic.Int32{}}
	ctx := context.Background()
	first := make(chan error, 1)
	go func() {
		_, err := workloads.Sync(ctx, slow, []byte("ca"), nil, nil, []string{"services"})
		first <- err
	}()
	select {
	case <-blocked:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("the first pass never reached its endpoints write")
	}
	second := make(chan error, 1)
	go func() {
		_, err := workloads.Sync(ctx, slow, []byte("ca"), nil, nil, []string{"services", "configmaps"})
		second <- err
	}()
	published := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		cm, err := client.CoreV1().ConfigMaps("spec").Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
		if err == nil && cm.Data["ca.crt"] == "ca" {
			published = true
			break
		}
	}
	attempts := slow.creates.Load()
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Fatal("the root CA was not published while the pass without the publisher was still draining")
	}
	if attempts != 1 {
		t.Fatalf("%d endpoints creates while the first was in flight, want 1: a controller the pass ran was started again", attempts)
	}
}

type slowEndpointsClientset struct {
	kubernetes.Interface
	blocked chan struct{}
	release chan struct{}
	creates *atomic.Int32
}

func (s slowEndpointsClientset) CoreV1() corev1client.CoreV1Interface {
	return slowEndpointsCore{CoreV1Interface: s.Interface.CoreV1(), clientset: s}
}

type slowEndpointsCore struct {
	corev1client.CoreV1Interface
	clientset slowEndpointsClientset
}

func (c slowEndpointsCore) Endpoints(namespace string) corev1client.EndpointsInterface {
	return slowEndpoints{EndpointsInterface: c.CoreV1Interface.Endpoints(namespace), clientset: c.clientset}
}

type slowEndpoints struct {
	corev1client.EndpointsInterface
	clientset slowEndpointsClientset
}

func (e slowEndpoints) Create(ctx context.Context, endpoints *v1.Endpoints, opts metav1.CreateOptions) (*v1.Endpoints, error) {
	e.clientset.creates.Add(1)
	select {
	case e.clientset.blocked <- struct{}{}:
	default:
	}
	<-e.clientset.release
	return e.EndpointsInterface.Create(ctx, endpoints, opts)
}
