package core

import (
	"strings"
	"testing"

	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
)

func TestPodAllowsPrivilegedContainer(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	pod := validPod()
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptr.To(true)}
	if err := registrytest.Create(store, pod); err != nil {
		t.Fatal(err)
	}
}

func coreStore(t *testing.T, name, kind string, namespaced bool) *registry.Store {
	t.Helper()
	store := registrytest.Store(t, schema.GroupVersion{Version: "v1"}, metav1.APIResource{Name: name, Kind: kind, Namespaced: namespaced})
	if customize, ok := registry.Customizers[name]; ok {
		customize(store, registry.Deps{})
	}
	return store
}

func validPod() *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:      "c",
			Image:     "nginx",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")}},
		}}},
	}
	scheme.Scheme.Default(pod)
	return pod
}

func TestConfigMapRejectsEmptyKey(t *testing.T) {
	store := coreStore(t, "configmaps", "ConfigMap", true)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"}, Data: map[string]string{"": "x"}}
	registrytest.RequireFieldError(t, registrytest.Create(store, cm), "data[]", "")
}

func TestSecretRejectsEmptyStringDataKey(t *testing.T) {
	store := coreStore(t, "secrets", "Secret", true)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"}, StringData: map[string]string{"": "x"}}
	registrytest.RequireFieldError(t, registrytest.Create(store, secret), "data[]", "")
}

func TestImmutableConfigMapRejectsDataChange(t *testing.T) {
	store := coreStore(t, "configmaps", "ConfigMap", true)
	immutable := true
	old := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default", ResourceVersion: "1", UID: "uid"},
		Immutable:  &immutable,
		Data:       map[string]string{"k": "v"},
	}
	next := old.DeepCopy()
	next.Data["k2"] = "v2"
	registrytest.RequireFieldError(t, registrytest.Update(store, next, old), "data", "field is immutable when `immutable` is set")
}

func TestPodCreateRejectsDuplicateContainerNames(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	pod := validPod()
	pod.Spec.Containers = append(pod.Spec.Containers, pod.Spec.Containers[0])
	registrytest.RequireFieldError(t, registrytest.Create(store, pod), "spec.containers[1].name", "Duplicate value")
}

func TestPodCreateRejectsInvalidSysctlName(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	pod := validPod()
	pod.Spec.SecurityContext = &corev1.PodSecurityContext{Sysctls: []corev1.Sysctl{{Name: "Not Valid", Value: "1"}}}
	registrytest.RequireFieldError(t, registrytest.Create(store, pod), "spec.securityContext.sysctls[0].name", "")
}

func TestPodCreateAcceptsValid(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	pod := validPod()
	if err := registrytest.Create(store, pod); err != nil {
		t.Fatal(err)
	}
	if pod.Status.Phase != corev1.PodPending || pod.Status.QOSClass == "" {
		t.Fatalf("status=%+v", pod.Status)
	}
}

func TestPodCreateMergesMatchLabelKeysIntoTopologySpreadSelector(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	pod := validPod()
	pod.Labels = map[string]string{"app": "web"}
	pod.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		MaxSkew:           1,
		TopologyKey:       "zone",
		WhenUnsatisfiable: corev1.DoNotSchedule,
		LabelSelector:     &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "front"}},
		MatchLabelKeys:    []string{"app"},
	}}
	if err := registrytest.Create(store, pod); err != nil {
		t.Fatal(err)
	}
	selector := pod.Spec.TopologySpreadConstraints[0].LabelSelector
	if len(selector.MatchExpressions) != 1 || selector.MatchExpressions[0].Key != "app" {
		t.Fatalf("selector=%+v", selector)
	}
}

func TestNodeCreateWarnsOnNonCanonicalPodCIDR(t *testing.T) {
	store := coreStore(t, "nodes", "Node", false)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}, Spec: corev1.NodeSpec{PodCIDR: "010.0.0.0/24", PodCIDRs: []string{"010.0.0.0/24"}}}
	warnings := store.CreateStrategy.WarningsOnCreate(registrytest.Context(store), node)
	if len(warnings) == 0 {
		t.Fatal("no warnings")
	}
}

func TestPodUpdateRejectsResourceChange(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	old := validPod()
	old.ResourceVersion = "1"
	old.UID = "uid"
	next := old.DeepCopy()
	next.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse("100m")
	registrytest.RequireFieldError(t, registrytest.Update(store, next, old), "spec", "pod updates may not change fields other than")
}

func TestPodUpdateAcceptsImageChange(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	old := validPod()
	old.ResourceVersion = "1"
	old.UID = "uid"
	next := old.DeepCopy()
	next.Spec.Containers[0].Image = "nginx:2"
	if err := registrytest.Update(store, next, old); err != nil {
		t.Fatal(err)
	}
}

func TestPodUpdateRejectsNodeNameChange(t *testing.T) {
	store := coreStore(t, "pods", "Pod", true)
	old := validPod()
	old.ResourceVersion = "1"
	old.UID = "uid"
	next := old.DeepCopy()
	next.Spec.NodeName = "n1"
	registrytest.RequireFieldError(t, registrytest.Update(store, next, old), "spec", "pod updates may not change fields other than")
}

func TestServiceCreateRejectsInvalidPort(t *testing.T) {
	store := coreStore(t, "services", "Service", true)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:   map[string]string{"app": "a"},
			ClusterIP:  "10.0.0.1",
			ClusterIPs: []string{"10.0.0.1"},
			Ports:      []corev1.ServicePort{{Name: "http", Port: 0, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
		},
	}
	scheme.Scheme.Default(service)
	registrytest.RequireFieldError(t, registrytest.Create(store, service), "spec.ports[0].port", "must be between 1 and 65535")
}

func TestServiceCreateAcceptsValid(t *testing.T) {
	store := coreStore(t, "services", "Service", true)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Selector:   map[string]string{"app": "a"},
			ClusterIP:  "10.0.0.1",
			ClusterIPs: []string{"10.0.0.1"},
			Ports:      []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
		},
	}
	scheme.Scheme.Default(service)
	if err := registrytest.Create(store, service); err != nil {
		t.Fatal(err)
	}
}

func TestTypeConverterKnowsPodContainersAreKeyedByName(t *testing.T) {
	converter, err := registry.TypeConverter()
	if err != nil {
		t.Fatal(err)
	}
	pod := validPod()
	pod.APIVersion, pod.Kind = "v1", "Pod"
	typed, err := converter.ObjectToTyped(pod)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := typed.ToFieldSet()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fields.String(), `.spec.containers[name="c"]`) {
		t.Fatalf("containers are not keyed by name: %s", fields.String())
	}
}
