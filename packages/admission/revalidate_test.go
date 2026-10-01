package admission

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLimitRangerValidatesWhatAMutatingWebhookChanged(t *testing.T) {
	h := newPodPatchingHandler(t, map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, containerCPULimitRange("50m", "500m")),
	}, `[{"op":"replace","path":"/spec/containers/0/resources/limits/cpu","value":"2"}]`)
	out := postAdmit(t, h, podCreateReq("admit", map[string]any{"containers": []any{map[string]any{
		"name": "c", "image": "img",
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "100m"},
			"limits":   map[string]any{"cpu": "100m"},
		},
	}}}))
	if !out.Allowed {
		t.Fatalf("admit: %+v", out)
	}
	if got := containerResource(out.Object, "limits", "cpu"); got != "2" {
		t.Fatalf("webhook patch not applied: cpu limit = %v", got)
	}
	for _, phase := range []string{"check", "validate"} {
		req := podCreateReq(phase, nil)
		req.Object = out.Object
		denied := postAdmit(t, h, req)
		if denied.Allowed {
			t.Fatalf("%s: expected the LimitRange max to reject the mutated pod", phase)
		}
		if !strings.Contains(denied.Message, "maximum cpu usage per Container is 500m, but limit is 2") {
			t.Fatalf("%s: message = %q", phase, denied.Message)
		}
	}
}

func TestLimitRangerAdmitsWhatAMutatingWebhookBroughtIntoRange(t *testing.T) {
	h := newPodPatchingHandler(t, map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, containerCPULimitRange("50m", "500m")),
	}, `[{"op":"replace","path":"/spec/containers/0/resources/requests/cpu","value":"100m"}]`)
	out := postAdmit(t, h, podCreateReq("admit", map[string]any{"containers": []any{map[string]any{
		"name": "c", "image": "img",
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "10m"},
			"limits":   map[string]any{"cpu": "200m"},
		},
	}}}))
	if !out.Allowed {
		t.Fatalf("admit: %+v", out)
	}
	req := podCreateReq("validate", nil)
	req.Object = out.Object
	if got := postAdmit(t, h, req); !got.Allowed {
		t.Fatalf("validate: %+v", got)
	}
}

func TestLimitRangerIgnoresPodUpdate(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type:    corev1.LimitTypeContainer,
				Default: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi")},
				Max:     corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			}}},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	spec := func() map[string]any {
		return map[string]any{
			"serviceAccountName": "default",
			"containers":         []any{map[string]any{"name": "c", "image": "img"}},
		}
	}
	for _, phase := range []string{"admit", "validate"} {
		req := podCreateReq(phase, spec())
		req.Operation = "UPDATE"
		req.OldObject = map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec":     spec(),
		}
		out := postAdmit(t, h, req)
		if !out.Allowed {
			t.Fatalf("%s: %+v", phase, out)
		}
		if phase == "admit" && containerResource(out.Object, "limits", "memory") != nil {
			t.Fatalf("defaults applied on update: %+v", out.Object["spec"])
		}
	}
}

func TestServiceAccountValidatesWhatAMutatingWebhookChanged(t *testing.T) {
	h := newPodPatchingHandler(t, map[string][]byte{
		"/registry/serviceaccounts/default/default": mustJSON(t, corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
		}),
	}, `[{"op":"replace","path":"/spec/serviceAccountName","value":"ghost"}]`)
	out := postAdmit(t, h, podCreateReq("admit", map[string]any{
		"containers": []any{map[string]any{"name": "c", "image": "img"}},
	}))
	if !out.Allowed {
		t.Fatalf("admit: %+v", out)
	}
	if got := out.Object["spec"].(map[string]any)["serviceAccountName"]; got != "ghost" {
		t.Fatalf("webhook patch not applied: serviceAccountName = %v", got)
	}
	req := podCreateReq("validate", nil)
	req.Object = out.Object
	denied := postAdmit(t, h, req)
	if denied.Allowed {
		t.Fatal("expected the missing service account to reject the mutated pod")
	}
	if !strings.Contains(denied.Message, "error looking up service account default/ghost") {
		t.Fatalf("message = %q", denied.Message)
	}
}

func TestServiceAccountValidateRequiresAName(t *testing.T) {
	h := newPodPatchingHandler(t, map[string][]byte{}, `[{"op":"remove","path":"/spec/serviceAccountName"}]`)
	out := postAdmit(t, h, podCreateReq("admit", map[string]any{
		"containers": []any{map[string]any{"name": "c", "image": "img"}},
	}))
	if !out.Allowed {
		t.Fatalf("admit: %+v", out)
	}
	req := podCreateReq("validate", nil)
	req.Object = out.Object
	denied := postAdmit(t, h, req)
	if denied.Allowed {
		t.Fatal("expected a pod without a service account to be rejected")
	}
	if !strings.Contains(denied.Message, "no service account specified for pod default/p") {
		t.Fatalf("message = %q", denied.Message)
	}
}

func TestPriorityClassRejectsSecondGlobalDefault(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/priorityclasses/first": mustJSON(t, schedulingv1.PriorityClass{
			ObjectMeta:    metav1.ObjectMeta{Name: "first"},
			Value:         10,
			GlobalDefault: true,
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	class := func(op, name string, globalDefault bool) admit.Request {
		return admit.Request{
			Phase:     "validate",
			Name:      name,
			Resource:  schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"},
			Kind:      schema.GroupVersionKind{Group: "scheduling.k8s.io", Version: "v1", Kind: "PriorityClass"},
			Operation: op,
			Object: map[string]any{
				"apiVersion": "scheduling.k8s.io/v1", "kind": "PriorityClass",
				"metadata":      map[string]any{"name": name},
				"value":         int64(20),
				"globalDefault": globalDefault,
			},
		}
	}
	for _, req := range []admit.Request{class("CREATE", "second", true), class("UPDATE", "second", true)} {
		out := postAdmit(t, h, req)
		if out.Allowed {
			t.Fatalf("%s: expected a second global default to be rejected", req.Operation)
		}
		if !strings.Contains(out.Message, "PriorityClass first is already marked as default. Only one default can exist") {
			t.Fatalf("%s: message = %q", req.Operation, out.Message)
		}
	}
	for _, req := range []admit.Request{class("UPDATE", "first", true), class("CREATE", "second", false)} {
		if out := postAdmit(t, h, req); !out.Allowed {
			t.Fatalf("%s %s: %+v", req.Operation, req.Name, out)
		}
	}
}

func newPodPatchingHandler(t *testing.T, data map[string][]byte, patch string) http.Handler {
	t.Helper()
	workerURL := workerURLPrefix + "patch-pods"
	fail := admissionregv1.Fail
	data["/registry/mutatingwebhookconfigurations/patch-pods"] = mustJSON(t, admissionregv1.MutatingWebhookConfiguration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingWebhookConfiguration"},
		ObjectMeta: metav1.ObjectMeta{Name: "patch-pods"},
		Webhooks: []admissionregv1.MutatingWebhook{{
			Name:          "patch-pods.k8flare.dev",
			FailurePolicy: &fail,
			ClientConfig:  admissionregv1.WebhookClientConfig{URL: &workerURL},
			Rules: []admissionregv1.RuleWithOperations{{
				Operations: []admissionregv1.OperationType{admissionregv1.Create},
				Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			}},
		}},
	})
	kineSrv := httptest.NewServer(&memStore{data: data})
	t.Cleanup(kineSrv.Close)
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var review struct {
			Request struct {
				UID string `json:"uid"`
			} `json:"request"`
		}
		_ = json.NewDecoder(r.Body).Decode(&review)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "admission.k8s.io/v1",
			"kind":       "AdmissionReview",
			"response": map[string]any{
				"uid":       review.Request.UID,
				"allowed":   true,
				"patchType": "JSONPatch",
				"patch":     base64.StdEncoding.EncodeToString([]byte(patch)),
			},
		})
	}))
	t.Cleanup(hooks.Close)
	return NewHandler(Config{Kine: rewriteClient(kineSrv), Hooks: rewriteClient(hooks)})
}

func podCreateReq(phase string, spec map[string]any) admit.Request {
	return admit.Request{
		Phase:     phase,
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec":     spec,
		},
	}
}

func containerCPULimitRange(min, max string) corev1.LimitRange {
	return corev1.LimitRange{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRange"},
		ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Min:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(min)},
			Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(max)},
		}}},
	}
}

func containerResource(obj map[string]any, bucket, name string) any {
	spec, _ := obj["spec"].(map[string]any)
	containers, _ := spec["containers"].([]any)
	if len(containers) == 0 {
		return nil
	}
	c, _ := containers[0].(map[string]any)
	resources, _ := c["resources"].(map[string]any)
	m, _ := resources[bucket].(map[string]any)
	return m[name]
}
