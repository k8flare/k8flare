package admission

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var (
	pvcResource = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	pvcKind     = schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"}
	pvResource  = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}
	pvKind      = schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolume"}
)

func diffClaim(mutate func(*corev1.PersistentVolumeClaim)) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "ns"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: quantity("1Gi")}},
		},
	}
	if mutate != nil {
		mutate(pvc)
	}
	return pvc
}

func claimCase(plugin, name string, pvc *corev1.PersistentVolumeClaim, cluster ...runtime.Object) diffCase {
	return diffCase{plugin: plugin, name: name, resource: pvcResource, kind: pvcKind, namespace: "ns", objName: "pvc", object: pvc, cluster: cluster}
}

func storageClass(name string, created time.Time, annotations map[string]string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: name, Annotations: annotations, CreationTimestamp: metav1.NewTime(created)},
		Provisioner: "example.com/p",
	}
}

func TestDiffDefaultStorageClass(t *testing.T) {
	const plugin = "DefaultStorageClass"
	const stable = "storageclass.kubernetes.io/is-default-class"
	const beta = "storageclass.beta.kubernetes.io/is-default-class"
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	def := map[string]string{stable: "true"}
	notDef := map[string]string{stable: "false"}
	withClass := func(name *string) *corev1.PersistentVolumeClaim {
		return diffClaim(func(p *corev1.PersistentVolumeClaim) { p.Spec.StorageClassName = name })
	}
	betaAnnotated := diffClaim(func(p *corev1.PersistentVolumeClaim) {
		p.Annotations = map[string]string{"volume.beta.kubernetes.io/storage-class": "x"}
	})
	cases := []diffCase{
		claimCase(plugin, "no classes", withClass(nil)),
		claimCase(plugin, "one default", withClass(nil), storageClass("a", base, def)),
		claimCase(plugin, "default annotated false", withClass(nil), storageClass("a", base, notDef)),
		claimCase(plugin, "beta default annotation", withClass(nil), storageClass("a", base, map[string]string{beta: "true"})),
		claimCase(plugin, "non-default classes only", withClass(nil), storageClass("a", base, nil), storageClass("b", base, nil)),
		claimCase(plugin, "newest of two defaults wins", withClass(nil), storageClass("old", base, def), storageClass("new", base.Add(time.Hour), def)),
		claimCase(plugin, "same timestamp tie goes to the lower name", withClass(nil), storageClass("b", base, def), storageClass("a", base, def)),
		claimCase(plugin, "stable and beta defaults compete", withClass(nil), storageClass("stable", base, def), storageClass("beta", base.Add(time.Hour), map[string]string{beta: "true"})),
		claimCase(plugin, "explicit class name", withClass(ptr.To("mine")), storageClass("a", base, def)),
		claimCase(plugin, "explicit empty class name", withClass(ptr.To("")), storageClass("a", base, def)),
		claimCase(plugin, "beta storage class annotation", betaAnnotated, storageClass("a", base, def)),
	}
	update := claimCase(plugin, "update is ignored", withClass(nil), storageClass("a", base, def))
	update.operation = "UPDATE"
	update.oldObject = withClass(nil)
	status := claimCase(plugin, "status subresource is ignored", withClass(nil), storageClass("a", base, def))
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = withClass(nil)
	other := claimCase(plugin, "other resource is ignored", withClass(nil), storageClass("a", base, def))
	other.resource.Resource = "persistentvolumes"
	cases = append(cases, update, status, other)
	runDiffCases(t, cases)
}

func TestDiffDefaultIngressClass(t *testing.T) {
	const plugin = "DefaultIngressClass"
	const defaultAnnotation = "ingressclass.kubernetes.io/is-default-class"
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	def := map[string]string{defaultAnnotation: "true"}
	ingressClass := func(name string, created time.Time, annotations map[string]string) *networkingv1.IngressClass {
		return &networkingv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations, CreationTimestamp: metav1.NewTime(created)},
			Spec:       networkingv1.IngressClassSpec{Controller: "example.com/c"},
		}
	}
	ingress := func(mutate func(*networkingv1.Ingress)) *networkingv1.Ingress {
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "ing", Namespace: "ns"},
			Spec: networkingv1.IngressSpec{
				DefaultBackend: &networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "svc", Port: networkingv1.ServiceBackendPort{Number: 80}}},
			},
		}
		if mutate != nil {
			mutate(ing)
		}
		return ing
	}
	ingCase := func(name string, ing *networkingv1.Ingress, cluster ...runtime.Object) diffCase {
		return diffCase{
			plugin: plugin, name: name,
			resource:  schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
			kind:      schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
			namespace: "ns", objName: "ing", object: ing, cluster: cluster,
		}
	}
	cases := []diffCase{
		ingCase("no classes", ingress(nil)),
		ingCase("one default", ingress(nil), ingressClass("a", base, def)),
		ingCase("default annotated false", ingress(nil), ingressClass("a", base, map[string]string{defaultAnnotation: "false"})),
		ingCase("non-default only", ingress(nil), ingressClass("a", base, nil)),
		ingCase("newest of two defaults wins", ingress(nil), ingressClass("old", base, def), ingressClass("new", base.Add(time.Hour), def)),
		ingCase("same timestamp tie goes to the lower name", ingress(nil), ingressClass("b", base, def), ingressClass("a", base, def)),
		ingCase("explicit class name", ingress(func(i *networkingv1.Ingress) { i.Spec.IngressClassName = ptr.To("mine") }), ingressClass("a", base, def)),
		ingCase("explicit empty class name", ingress(func(i *networkingv1.Ingress) { i.Spec.IngressClassName = ptr.To("") }), ingressClass("a", base, def)),
		ingCase("class annotation", ingress(func(i *networkingv1.Ingress) {
			i.Annotations = map[string]string{"kubernetes.io/ingress.class": "nginx"}
		}), ingressClass("a", base, def)),
	}
	update := ingCase("update is ignored", ingress(nil), ingressClass("a", base, def))
	update.operation = "UPDATE"
	update.oldObject = ingress(nil)
	status := ingCase("status subresource is ignored", ingress(nil), ingressClass("a", base, def))
	status.operation = "UPDATE"
	status.subresource = "status"
	status.oldObject = ingress(nil)
	cases = append(cases, update, status)
	runDiffCases(t, cases)
}

func TestDiffStorageObjectInUseProtection(t *testing.T) {
	const plugin = "StorageObjectInUseProtection"
	pv := func(finalizers ...string) *corev1.PersistentVolume {
		return &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv", Finalizers: finalizers},
			Spec: corev1.PersistentVolumeSpec{
				Capacity:               corev1.ResourceList{corev1.ResourceStorage: quantity("1Gi")},
				AccessModes:            []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				PersistentVolumeSource: corev1.PersistentVolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/tmp"}},
			},
		}
	}
	claim := func(finalizers ...string) *corev1.PersistentVolumeClaim {
		return diffClaim(func(p *corev1.PersistentVolumeClaim) { p.Finalizers = finalizers })
	}
	vac := func(finalizers ...string) *storagev1.VolumeAttributesClass {
		return &storagev1.VolumeAttributesClass{ObjectMeta: metav1.ObjectMeta{Name: "vac", Finalizers: finalizers}, DriverName: "example.com/d"}
	}
	pvcCase := func(name, op string, pvc *corev1.PersistentVolumeClaim) diffCase {
		c := claimCase(plugin, name, pvc)
		c.operation = admissionOperation(op)
		return c
	}
	pvCase := func(name, op string, obj *corev1.PersistentVolume) diffCase {
		return diffCase{plugin: plugin, name: name, resource: pvResource, kind: pvKind, objName: "pv", object: obj, operation: admissionOperation(op)}
	}
	vacCase := func(name, op string, obj *storagev1.VolumeAttributesClass) diffCase {
		return diffCase{
			plugin: plugin, name: name,
			resource: schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "volumeattributesclasses"},
			kind:     schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "VolumeAttributesClass"},
			objName:  "vac", object: obj, operation: admissionOperation(op),
		}
	}
	cases := []diffCase{
		pvcCase("claim without finalizers", "CREATE", claim()),
		pvcCase("claim with the protection finalizer", "CREATE", claim("kubernetes.io/pvc-protection")),
		pvcCase("claim with another finalizer", "CREATE", claim("example.com/f")),
		pvcCase("claim with another finalizer and the protection one", "CREATE", claim("example.com/f", "kubernetes.io/pvc-protection")),
		pvcCase("claim update", "UPDATE", claim()),
		pvcCase("claim delete", "DELETE", claim()),
		pvCase("volume without finalizers", "CREATE", pv()),
		pvCase("volume with the protection finalizer", "CREATE", pv("kubernetes.io/pv-protection")),
		pvCase("volume with another finalizer", "CREATE", pv("example.com/f")),
		pvCase("volume update", "UPDATE", pv()),
		pvCase("volume carrying the claim finalizer", "CREATE", pv("kubernetes.io/pvc-protection")),
		vacCase("attributes class without finalizers", "CREATE", vac()),
		vacCase("attributes class with the protection finalizer", "CREATE", vac("kubernetes.io/vac-protection")),
		vacCase("attributes class update", "UPDATE", vac()),
	}
	claimStatus := pvcCase("claim status subresource", "UPDATE", claim())
	claimStatus.subresource = "status"
	pvStatus := pvCase("volume status subresource", "UPDATE", pv())
	pvStatus.subresource = "status"
	pod := podDiffCase(plugin, "other resource is ignored", diffPod(nil))
	cases = append(cases, claimStatus, pvStatus, pod)
	runDiffCases(t, cases)
}
