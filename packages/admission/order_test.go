package admission

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	kubeoptions "k8s.io/kubernetes/pkg/kubeapiserver/options"
)

func TestChainRunsTheEnabledPluginsInUpstreamOrder(t *testing.T) {
	upstream := kubeoptions.NewAdmissionOptions().GenericAdmission
	registered := map[string]bool{}
	for _, name := range upstream.Plugins.Registered() {
		registered[name] = true
	}
	ours := map[string]bool{}
	var gotAdmit, gotValidate, notUpstream []string
	for _, p := range chain {
		if !registered[p.name] {
			notUpstream = append(notUpstream, p.name)
			continue
		}
		ours[p.name] = true
		if p.admit != nil {
			gotAdmit = append(gotAdmit, p.name)
		}
		if p.validate != nil {
			gotValidate = append(gotValidate, p.name)
		}
	}
	var wantAdmit, wantValidate []string
	for _, name := range upstream.RecommendedPluginOrder {
		if !ours[name] {
			continue
		}
		plugin := upstreamPluginInstance(t, upstream.Plugins, name)
		if _, ok := plugin.(admission.MutationInterface); ok {
			wantAdmit = append(wantAdmit, name)
		}
		if _, ok := plugin.(admission.ValidationInterface); ok {
			wantValidate = append(wantValidate, name)
		}
	}
	if diff := cmp.Diff(wantAdmit, gotAdmit); diff != "" {
		t.Errorf("admit phase differs from upstream (- upstream, + ours):\n%s", diff)
	}
	if diff := cmp.Diff(wantValidate, gotValidate); diff != "" {
		t.Errorf("validate phase differs from upstream (- upstream, + ours):\n%s", diff)
	}
	if diff := cmp.Diff([]string{computeClassPluginName}, notUpstream); diff != "" {
		t.Errorf("plugins upstream does not have (- want, + got):\n%s", diff)
	}
}

type pluginCapture struct{ plugin *admission.Interface }

func (c pluginCapture) Initialize(p admission.Interface) { *c.plugin = p }

func upstreamPluginInstance(t *testing.T, plugins *admission.Plugins, name string) admission.Interface {
	t.Helper()
	var plugin admission.Interface
	_, _ = plugins.InitPlugin(name, nil, pluginCapture{&plugin})
	if plugin == nil {
		t.Fatalf("upstream plugin %s could not be constructed", name)
	}
	return plugin
}

func TestBuiltInValidatorsRunBeforeValidatingWebhooks(t *testing.T) {
	h, calls := newWebhookHandler(t, map[string][]byte{
		"/registry/limitranges/default/lr":                    mustJSON(t, containerCPULimitRange("50m", "500m")),
		"/registry/validatingwebhookconfigurations/deny-pods": mustJSON(t, validatingWebhookFor("deny-pods", "pods")),
	}, hookDeny)
	req := podCreateReq("validate", map[string]any{
		"serviceAccountName": "default",
		"containers": []any{map[string]any{
			"name": "c", "image": "img",
			"resources": map[string]any{"limits": map[string]any{"cpu": "2"}},
		}},
	})
	out := postAdmit(t, h, req)
	if out.Allowed {
		t.Fatal("expected a denial")
	}
	if !strings.Contains(out.Message, "maximum cpu usage per Container is 500m, but limit is 2") {
		t.Fatalf("message = %q, want LimitRanger's denial", out.Message)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("validating webhook called %d times for a request LimitRanger denies", n)
	}
}

func TestValidatingAdmissionPolicyRunsBeforeValidatingWebhooks(t *testing.T) {
	h, calls := newWebhookHandler(t, map[string][]byte{
		"/registry/namespaces/default": mustJSON(t, corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}),
		"/registry/validatingadmissionpolicies/deny-all": mustJSON(t, defaulted(&admissionregv1.ValidatingAdmissionPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "deny-all"},
			Spec: admissionregv1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregv1.RuleWithOperations{
						Operations: []admissionregv1.OperationType{admissionregv1.Create},
						Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
					},
				}}},
				Validations: []admissionregv1.Validation{{Expression: "false", Message: "denied by policy"}},
			},
		})),
		"/registry/validatingadmissionpolicybindings/deny-all": mustJSON(t, defaulted(&admissionregv1.ValidatingAdmissionPolicyBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "deny-all"},
			Spec: admissionregv1.ValidatingAdmissionPolicyBindingSpec{
				PolicyName:        "deny-all",
				ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
			},
		})),
		"/registry/validatingwebhookconfigurations/deny-configmaps": mustJSON(t, validatingWebhookFor("deny-configmaps", "configmaps")),
	}, hookDeny)
	out := postAdmit(t, h, admit.Request{
		Phase:     "validate",
		Name:      "cm",
		Namespace: "default",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
		Operation: "CREATE",
		Object:    map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "cm", "namespace": "default"}},
	})
	if out.Allowed {
		t.Fatal("expected a denial")
	}
	if out.Message != `configmaps "cm" is forbidden: ValidatingAdmissionPolicy 'deny-all' with binding 'deny-all' denied request: denied by policy` {
		t.Fatalf("message = %q, want the policy's denial", out.Message)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("validating webhook called %d times for a request the policy denies", n)
	}
}

func TestNodeRestrictionRunsBeforeMutatingWebhooks(t *testing.T) {
	fail := admissionregv1.Fail
	url := workerURLPrefix + "label-nodes"
	h, calls := newWebhookHandler(t, map[string][]byte{
		"/registry/mutatingwebhookconfigurations/label-nodes": mustJSON(t, admissionregv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "label-nodes"},
			Webhooks: []admissionregv1.MutatingWebhook{{
				Name:          "label-nodes.k8flare.dev",
				FailurePolicy: &fail,
				ClientConfig:  admissionregv1.WebhookClientConfig{URL: &url},
				Rules: []admissionregv1.RuleWithOperations{{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"nodes"}},
				}},
			}},
		}),
	}, hookPatch(`[{"op":"add","path":"/metadata/labels/node-restriction.kubernetes.io~1pool","value":"a"}]`))
	req := admit.Request{
		Phase:     "admit",
		Name:      "n1",
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Node"},
		Operation: "CREATE",
		User:      admit.User{Username: "system:node:n1", Groups: []string{user.NodesGroup, user.AllAuthenticated}},
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Node",
			"metadata": map[string]any{"name": "n1", "labels": map[string]any{"kubernetes.io/hostname": "n1"}},
		},
	}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("admit: %+v", out)
	}
	if calls.Load() != 1 {
		t.Fatalf("mutating webhook calls = %d, want 1", calls.Load())
	}
	labels := out.Object["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels["node-restriction.kubernetes.io/pool"] != "a" {
		t.Fatalf("webhook label not applied: %v", labels)
	}
	for _, phase := range []string{"check", "validate"} {
		next := req
		next.Phase = phase
		next.Object = out.Object
		if got := postAdmit(t, h, next); !got.Allowed {
			t.Fatalf("%s: NodeRestriction rechecked what a mutating webhook set: %+v", phase, got)
		}
	}
}

func TestComputeClassRunsBeforeServiceAccount(t *testing.T) {
	kineSrv := httptest.NewServer(&memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	req := podCreateReq("admit", map[string]any{"containers": []any{map[string]any{"name": "c", "image": "img"}}})
	req.Object["metadata"].(map[string]any)["annotations"] = map[string]any{computeClassAnnotation: computeClassContainers}
	out := postAdmit(t, h, req)
	if !out.Allowed {
		t.Fatalf("admit: %+v", out)
	}
	spec := out.Object["spec"].(map[string]any)
	if spec["automountServiceAccountToken"] != false || hasProjectedSAVolume(spec) {
		t.Fatalf("ServiceAccount mounted a token into a containers pod: %+v", spec)
	}
}

type hookResponse func(uid string) map[string]any

func hookDeny(uid string) map[string]any {
	return map[string]any{"uid": uid, "allowed": false, "status": map[string]any{"message": "denied by webhook"}}
}

func hookPatch(patch string) hookResponse {
	return func(uid string) map[string]any {
		return map[string]any{"uid": uid, "allowed": true, "patchType": "JSONPatch", "patch": base64.StdEncoding.EncodeToString([]byte(patch))}
	}
}

func newWebhookHandler(t *testing.T, data map[string][]byte, respond hookResponse) (http.Handler, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	kineSrv := httptest.NewServer(&memStore{data: data})
	t.Cleanup(kineSrv.Close)
	hooks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var review struct {
			Request struct {
				UID string `json:"uid"`
			} `json:"request"`
		}
		_ = json.NewDecoder(r.Body).Decode(&review)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "admission.k8s.io/v1",
			"kind":       "AdmissionReview",
			"response":   respond(review.Request.UID),
		})
	}))
	t.Cleanup(hooks.Close)
	return NewHandler(Config{Kine: rewriteClient(kineSrv), Hooks: rewriteClient(hooks)}), calls
}

func validatingWebhookFor(name, resource string) admissionregv1.ValidatingWebhookConfiguration {
	fail := admissionregv1.Fail
	url := workerURLPrefix + name
	return admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Webhooks: []admissionregv1.ValidatingWebhook{{
			Name:          name + ".k8flare.dev",
			FailurePolicy: &fail,
			ClientConfig:  admissionregv1.WebhookClientConfig{URL: &url},
			Rules: []admissionregv1.RuleWithOperations{{
				Operations: []admissionregv1.OperationType{admissionregv1.Create},
				Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{resource}},
			}},
		}},
	}
}
