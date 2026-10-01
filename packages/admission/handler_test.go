package admission

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func TestHandlerLimitRangeAndVAPAndWebhook(t *testing.T) {
	cpu := "100m"
	workerURL := workerURLPrefix + "admission-echo"
	fail := admissionregv1.Fail
	store := &memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRange"},
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type:           corev1.LimitTypeContainer,
				DefaultRequest: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)},
			}}},
		}),
		"/registry/validatingadmissionpolicies/deny-label": mustJSON(t, admissionregv1.ValidatingAdmissionPolicy{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingAdmissionPolicy"},
			ObjectMeta: metav1.ObjectMeta{Name: "deny-label"},
			Spec: admissionregv1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregv1.RuleWithOperations{
						Operations: []admissionregv1.OperationType{admissionregv1.Create},
						Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
					},
				}}},
				Validations: []admissionregv1.Validation{{Expression: `!(has(object.metadata.labels) && object.metadata.labels["k8flare.io/deny"] == "true")`}},
			},
		}),
		"/registry/validatingadmissionpolicybindings/deny-label": mustJSON(t, admissionregv1.ValidatingAdmissionPolicyBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingAdmissionPolicyBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: "deny-label"},
			Spec: admissionregv1.ValidatingAdmissionPolicyBindingSpec{
				PolicyName:        "deny-label",
				ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
			},
		}),
		"/registry/validatingwebhookconfigurations/echo": mustJSON(t, admissionregv1.ValidatingWebhookConfiguration{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration"},
			ObjectMeta: metav1.ObjectMeta{Name: "echo"},
			Webhooks: []admissionregv1.ValidatingWebhook{{
				Name:          "echo.k8flare.dev",
				FailurePolicy: &fail,
				ClientConfig:  admissionregv1.WebhookClientConfig{URL: &workerURL},
				Rules: []admissionregv1.RuleWithOperations{{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"secrets"}},
				}},
			}},
		}),
	}}
	kineSrv := httptest.NewServer(store)
	defer kineSrv.Close()
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var review struct {
			Request struct {
				Object map[string]any `json:"object"`
				UID    string         `json:"uid"`
			} `json:"request"`
		}
		_ = json.NewDecoder(r.Body).Decode(&review)
		allowed := true
		msg := ""
		if meta, _ := review.Request.Object["metadata"].(map[string]any); meta != nil {
			if ann, _ := meta["annotations"].(map[string]any); ann["k8flare.io/admit"] == "deny" {
				allowed, msg = false, "denied by echo"
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "admission.k8s.io/v1",
			"kind":       "AdmissionReview",
			"response":   map[string]any{"uid": review.Request.UID, "allowed": allowed, "status": map[string]any{"message": msg}},
		})
	}))
	defer hooks.Close()

	h := NewHandler(Config{
		Kine:  rewriteClient(kineSrv),
		Hooks: rewriteClient(hooks),
	})

	podReq := admit.Request{
		Phase:     "admit",
		Name:      "p",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": "default"},
			"spec":     map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}},
		},
	}
	out := postAdmit(t, h, podReq)
	if !out.Allowed {
		t.Fatalf("pod admit: %+v", out)
	}
	spec := out.Object["spec"].(map[string]any)
	if spec["serviceAccountName"] != "default" {
		t.Fatalf("service account: %+v", spec)
	}
	if !hasProjectedSAVolume(spec) {
		t.Fatalf("projected sa volume: %+v", spec["volumes"])
	}
	containers := spec["containers"].([]any)
	res := containers[0].(map[string]any)["resources"].(map[string]any)
	reqCPU := res["requests"].(map[string]any)["cpu"]
	if reqCPU != cpu {
		t.Fatalf("limitrange default request: got %v", reqCPU)
	}

	minStore := &memStore{data: map[string][]byte{
		"/registry/limitranges/default/lr": mustJSON(t, corev1.LimitRange{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRange"},
			ObjectMeta: metav1.ObjectMeta{Name: "lr", Namespace: "default"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Min:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
				Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			}}},
		}),
	}}
	minKine := httptest.NewServer(minStore)
	defer minKine.Close()
	minH := NewHandler(Config{Kine: rewriteClient(minKine)})
	belowMin := podReq
	belowMin.Object = map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": "p", "namespace": "default"},
		"spec": map[string]any{"containers": []any{map[string]any{
			"name": "c", "image": "img",
			"resources": map[string]any{"requests": map[string]any{"cpu": "10m"}},
		}}},
	}
	out = postAdmit(t, minH, belowMin)
	if out.Allowed {
		t.Fatal("expected LimitRange min deny")
	}

	denyCM := admit.Request{
		Phase:     "validate",
		Name:      "cm",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Operation: "CREATE",
		Object: map[string]any{
			"metadata": map[string]any{"name": "cm", "labels": map[string]any{"k8flare.io/deny": "true"}},
		},
	}
	out = postAdmit(t, h, denyCM)
	if out.Allowed {
		t.Fatal("expected VAP deny")
	}

	denySecret := admit.Request{
		Phase:     "validate",
		Name:      "s",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "secrets"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
		Operation: "CREATE",
		Object: map[string]any{
			"metadata": map[string]any{"name": "s", "annotations": map[string]any{"k8flare.io/admit": "deny"}},
		},
	}
	out = postAdmit(t, h, denySecret)
	if out.Allowed {
		t.Fatal("expected webhook deny")
	}
	if !strings.Contains(out.Message, "denied the request") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestHandlerDeniesConnectPodAttach(t *testing.T) {
	fail := admissionregv1.Fail
	hookURL := ""
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var review struct {
			Request struct {
				UID  string `json:"uid"`
				Name string `json:"name"`
				Kind struct {
					Kind string `json:"kind"`
				} `json:"kind"`
			} `json:"request"`
		}
		_ = json.NewDecoder(r.Body).Decode(&review)
		msg := ""
		allowed := true
		if review.Request.Kind.Kind == "PodAttachOptions" {
			allowed, msg = false, "attaching to pod '"+review.Request.Name+"' is not allowed"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "admission.k8s.io/v1",
			"kind":       "AdmissionReview",
			"response":   map[string]any{"uid": review.Request.UID, "allowed": allowed, "status": map[string]any{"message": msg}},
		})
	}))
	defer hooks.Close()
	hookURL = hooks.URL + "/pods/attach"
	store := &memStore{data: map[string][]byte{
		"/registry/namespaces/webhook": mustJSON(t, corev1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: "webhook", Labels: map[string]string{"unique": "true"}},
		}),
		"/registry/validatingwebhookconfigurations/deny-attach": mustJSON(t, admissionregv1.ValidatingWebhookConfiguration{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "ValidatingWebhookConfiguration"},
			ObjectMeta: metav1.ObjectMeta{Name: "deny-attach"},
			Webhooks: []admissionregv1.ValidatingWebhook{{
				Name:          "deny-attaching-pod.k8s.io",
				FailurePolicy: &fail,
				ClientConfig:  admissionregv1.WebhookClientConfig{URL: &hookURL},
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"unique": "true"},
				},
				Rules: []admissionregv1.RuleWithOperations{{
					Operations: []admissionregv1.OperationType{admissionregv1.Connect},
					Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods/attach"}},
				}},
			}},
		}),
	}}
	kineSrv := httptest.NewServer(store)
	defer kineSrv.Close()
	h := NewHandler(Config{
		Kine:     rewriteClient(kineSrv),
		Outbound: rewriteClient(hooks),
	})
	out := postAdmit(t, h, admit.Request{
		Phase:       "validate",
		Name:        "to-be-attached-pod",
		Namespace:   "webhook",
		Resource:    schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Subresource: "attach",
		Kind:        schema.GroupVersionKind{Version: "v1", Kind: "PodAttachOptions"},
		Operation:   "CONNECT",
		Object:      map[string]any{"apiVersion": "v1", "kind": "PodAttachOptions", "container": "container1"},
	})
	if out.Allowed {
		t.Fatal("expected CONNECT pods/attach deny")
	}
	if !strings.Contains(out.Message, "attaching to pod 'to-be-attached-pod' is not allowed") {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestHandlerSkipsWebhooksForAdmissionConfigs(t *testing.T) {
	workerURL := workerURLPrefix + "admission-echo"
	fail := admissionregv1.Fail
	store := &memStore{data: map[string][]byte{
		"/registry/mutatingwebhookconfigurations/mutate-configs": mustJSON(t, admissionregv1.MutatingWebhookConfiguration{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingWebhookConfiguration"},
			ObjectMeta: metav1.ObjectMeta{Name: "mutate-configs"},
			Webhooks: []admissionregv1.MutatingWebhook{{
				Name:          "add-label-to-webhook-configurations.k8s.io",
				FailurePolicy: &fail,
				ClientConfig:  admissionregv1.WebhookClientConfig{URL: &workerURL},
				Rules: []admissionregv1.RuleWithOperations{{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule: admissionregv1.Rule{
						APIGroups:   []string{"admissionregistration.k8s.io"},
						APIVersions: []string{"*"},
						Resources:   []string{"validatingwebhookconfigurations", "mutatingwebhookconfigurations"},
					},
				}},
			}},
		}),
	}}
	kineSrv := httptest.NewServer(store)
	defer kineSrv.Close()
	called := false
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer hooks.Close()
	h := NewHandler(Config{
		Kine:  rewriteClient(kineSrv),
		Hooks: rewriteClient(hooks),
	})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "dummy",
		Resource:  schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations"},
		Kind:      schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "ValidatingWebhookConfiguration"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingWebhookConfiguration",
			"metadata":   map[string]any{"name": "dummy"},
		},
	})
	if !out.Allowed {
		t.Fatalf("expected exempt admit, got %+v", out)
	}
	if called {
		t.Fatal("webhook should not run for admission configuration objects")
	}
}

func TestMAPApplyConfiguration(t *testing.T) {
	fail := admissionregv1.Fail
	never := admissionregv1.NeverReinvocationPolicy
	store := &memStore{data: map[string][]byte{
		"/registry/mutatingadmissionpolicies/label": mustJSON(t, admissionregv1.MutatingAdmissionPolicy{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingAdmissionPolicy"},
			ObjectMeta: metav1.ObjectMeta{Name: "label"},
			Spec: admissionregv1.MutatingAdmissionPolicySpec{
				FailurePolicy: &fail,
				MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregv1.RuleWithOperations{
						Operations: []admissionregv1.OperationType{admissionregv1.Create},
						Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
					},
				}}},
				Mutations: []admissionregv1.Mutation{{
					PatchType: admissionregv1.PatchTypeApplyConfiguration,
					ApplyConfiguration: &admissionregv1.ApplyConfiguration{
						Expression: `Object{metadata: Object.metadata{labels: {"k8flare.io/mutated": "true"}}}`,
					},
				}},
				ReinvocationPolicy: never,
			},
		}),
		"/registry/mutatingadmissionpolicybindings/label": mustJSON(t, admissionregv1.MutatingAdmissionPolicyBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "admissionregistration.k8s.io/v1", Kind: "MutatingAdmissionPolicyBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: "label"},
			Spec:       admissionregv1.MutatingAdmissionPolicyBindingSpec{PolicyName: "label"},
		}),
	}}
	kineSrv := httptest.NewServer(store)
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "cm",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"name": "cm", "namespace": "default"},
			"data":     map[string]any{"k": "v"},
		},
	})
	if !out.Allowed {
		t.Fatalf("map admit: %+v", out)
	}
	labels, _ := out.Object["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["k8flare.io/mutated"] != "true" {
		t.Fatalf("mutated labels: %+v", out.Object["metadata"])
	}
}

func postAdmit(t *testing.T, h http.Handler, req admit.Request) admit.Response {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/admit", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("admit HTTP %d: %s", w.Code, w.Body.String())
	}
	var out admit.Response
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

type memStore struct {
	data map[string][]byte
	mu   sync.Mutex
	revs map[string]int64
}

func (m *memStore) revision(key string) int64 {
	if rev, ok := m.revs[key]; ok {
		return rev
	}
	return 1
}

func (m *memStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.revs == nil {
		m.revs = map[string]int64{}
	}
	switch r.URL.Path {
	case "/list":
		prefix := r.URL.Query().Get("prefix")
		var kvs []map[string]any
		for k, v := range m.data {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": m.revision(k)})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kvs": kvs})
	case "/kv":
		switch r.Method {
		case http.MethodPut, http.MethodDelete:
			var body struct {
				Key      string `json:"key"`
				Value    string `json:"value"`
				Revision int64  `json:"revision"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_, exists := m.data[body.Key]
			if body.Revision != 0 && (!exists || m.revision(body.Key) != body.Revision) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "error": "conflict"})
				return
			}
			if r.Method == http.MethodDelete {
				delete(m.data, body.Key)
			} else {
				value, err := base64.StdEncoding.DecodeString(body.Value)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				m.data[body.Key] = value
			}
			m.revs[body.Key] = m.revision(body.Key) + 1
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": m.revs[body.Key]})
			return
		}
		k := r.URL.Query().Get("key")
		if v, ok := m.data[k]; ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": m.revision(k)}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
	default:
		http.NotFound(w, r)
	}
}

func rewriteClient(srv *httptest.Server) *http.Client {
	return &http.Client{Transport: hostRewrite{base: srv.URL, next: srv.Client().Transport}}
}

type hostRewrite struct {
	base string
	next http.RoundTripper
}

func (h hostRewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	next := req.Clone(req.Context())
	u, err := http.NewRequest(req.Method, h.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.URL = u.URL
	next.Host = u.Host
	if h.next == nil {
		return http.DefaultTransport.RoundTrip(next)
	}
	return h.next.RoundTrip(next)
}

func TestApplyServiceAccountHonorsSAAutomount(t *testing.T) {
	store := &memStore{data: map[string][]byte{
		"/registry/serviceaccounts/default/nomount": mustJSON(t, corev1.ServiceAccount{
			ObjectMeta:                   metav1.ObjectMeta{Name: "nomount", Namespace: "default"},
			AutomountServiceAccountToken: ptr.To(false),
		}),
	}}
	kineSrv := httptest.NewServer(store)
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	pod := func(sa string, automount any) admit.Request {
		spec := map[string]any{
			"serviceAccountName": sa,
			"containers":         []any{map[string]any{"name": "c", "image": "img"}},
		}
		if automount != nil {
			spec["automountServiceAccountToken"] = automount
		}
		return admit.Request{
			Phase:     "admit",
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
	off := postAdmit(t, h, pod("nomount", nil))
	if !off.Allowed {
		t.Fatalf("nomount admit: %+v", off)
	}
	if hasProjectedSAVolume(off.Object["spec"].(map[string]any)) {
		t.Fatalf("SA automount=false still mounted: %+v", off.Object["spec"])
	}
	on := postAdmit(t, h, pod("nomount", true))
	if !on.Allowed || !hasProjectedSAVolume(on.Object["spec"].(map[string]any)) {
		t.Fatalf("pod automount=true should mount: %+v", on)
	}
}

func TestApplyServiceAccountDeniesMissing(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, admit.Request{
		Phase:     "admit",
		Name:      "miss",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "miss", "namespace": "default"},
			"spec": map[string]any{
				"serviceAccountName": "no-such-sa",
				"containers":         []any{map[string]any{"name": "c", "image": "img"}},
			},
		},
	})
	if out.Allowed {
		t.Fatal("expected missing service account deny")
	}
	if !strings.Contains(out.Message, `error looking up service account default/no-such-sa`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func hasProjectedSAVolume(spec map[string]any) bool {
	volumes, _ := spec["volumes"].([]any)
	for _, raw := range volumes {
		v, _ := raw.(map[string]any)
		if v != nil && v["name"] == "kube-api-access" {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

