package admission

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	"github.com/k8flare/k8flare/packages/openapi"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/policy/mutating"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/apiserver/pkg/warning"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	_ "k8s.io/kubernetes/pkg/apis/admissionregistration/install"
)

var policyNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team", Labels: map[string]string{"policy": "on"}}}

func defaulted[T runtime.Object](obj T) T {
	legacyscheme.Scheme.Default(obj)
	return obj
}

func paramsConfigMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team", Labels: map[string]string{"params": "yes"}}, Data: data}
}

func configMapRule() admissionregv1.NamedRuleWithOperations {
	return admissionregv1.NamedRuleWithOperations{RuleWithOperations: admissionregv1.RuleWithOperations{
		Operations: []admissionregv1.OperationType{admissionregv1.Create},
		Rule:       admissionregv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
	}}
}

func deploymentRule() admissionregv1.NamedRuleWithOperations {
	return admissionregv1.NamedRuleWithOperations{RuleWithOperations: admissionregv1.RuleWithOperations{
		Operations: []admissionregv1.OperationType{admissionregv1.Create},
		Rule:       admissionregv1.Rule{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
	}}
}

type vapCase struct {
	name     string
	spec     admissionregv1.ValidatingAdmissionPolicySpec
	binding  admissionregv1.ValidatingAdmissionPolicyBindingSpec
	params   []*corev1.ConfigMap
	resource schema.GroupVersionResource
	kind     schema.GroupVersionKind
	object   runtime.Object
}

func configMapRequest(name string, data map[string]string) (schema.GroupVersionResource, schema.GroupVersionKind, runtime.Object) {
	return corev1.SchemeGroupVersion.WithResource("configmaps"), corev1.SchemeGroupVersion.WithKind("ConfigMap"),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team"}, Data: data}
}

func vapCases() []vapCase {
	ignore := admissionregv1.Ignore
	deny := admissionregv1.DenyAction
	paramKind := &admissionregv1.ParamKind{APIVersion: "v1", Kind: "ConfigMap"}
	match := &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{configMapRule()}}
	withCM := func(c vapCase, name string, data map[string]string) vapCase {
		c.resource, c.kind, c.object = configMapRequest(name, data)
		return c
	}
	return []vapCase{
		withCM(vapCase{
			name: "message expression denies",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match, Validations: []admissionregv1.Validation{{
				Expression: "object.data.size() < 2", MessageExpression: "'too many keys: ' + string(object.data.size())", Reason: ptrTo(metav1.StatusReasonForbidden),
			}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
		}, "cm", map[string]string{"a": "1", "b": "2"}),
		withCM(vapCase{
			name: "match condition skips the policy",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match,
				MatchConditions: []admissionregv1.MatchCondition{{Name: "only-protected", Expression: "object.metadata.name.startsWith('protected-')"}},
				Validations:     []admissionregv1.Validation{{Expression: "false"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
		}, "cm", nil),
		withCM(vapCase{
			name: "match condition error with failurePolicy Ignore admits",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match, FailurePolicy: &ignore,
				MatchConditions: []admissionregv1.MatchCondition{{Name: "broken", Expression: "object.data.missing == 'x'"}},
				Validations:     []admissionregv1.Validation{{Expression: "false"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
		}, "cm", map[string]string{"a": "1"}),
		withCM(vapCase{
			name: "expression error with failurePolicy Ignore admits",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match, FailurePolicy: &ignore,
				Validations: []admissionregv1.Validation{{Expression: "object.data.missing == 'x'"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
		}, "cm", map[string]string{"a": "1"}),
		withCM(vapCase{
			name: "expression error with failurePolicy Fail denies",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match,
				Validations: []admissionregv1.Validation{{Expression: "object.data.missing == 'x'"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
		}, "cm", map[string]string{"a": "1"}),
		withCM(vapCase{
			name: "param by name is bound",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match, ParamKind: paramKind,
				Validations: []admissionregv1.Validation{{Expression: "object.data.size() <= int(params.data.maxKeys)", Message: "over the limit"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
				ParamRef: &admissionregv1.ParamRef{Name: "limits", ParameterNotFoundAction: &deny}},
			params: []*corev1.ConfigMap{paramsConfigMap("limits", map[string]string{"maxKeys": "1"})},
		}, "cm", map[string]string{"a": "1", "b": "2"}),
		withCM(vapCase{
			name: "params by selector are each evaluated",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match, ParamKind: paramKind,
				Validations: []admissionregv1.Validation{{Expression: "!(params.data.forbidden in object.data)", MessageExpression: "'key ' + params.data.forbidden + ' is forbidden'"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
				ParamRef: &admissionregv1.ParamRef{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"params": "yes"}}, ParameterNotFoundAction: &deny}},
			params: []*corev1.ConfigMap{paramsConfigMap("first", map[string]string{"forbidden": "x"}), paramsConfigMap("second", map[string]string{"forbidden": "b"})},
		}, "cm", map[string]string{"a": "1", "b": "2"}),
		withCM(vapCase{
			name: "missing param with parameterNotFoundAction Deny denies",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match, ParamKind: paramKind,
				Validations: []admissionregv1.Validation{{Expression: "true"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
				ParamRef: &admissionregv1.ParamRef{Name: "absent", ParameterNotFoundAction: &deny}},
		}, "cm", nil),
		withCM(vapCase{
			name: "Warn action admits with a warning",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match,
				Validations: []admissionregv1.Validation{{Expression: "false", Message: "this is only a warning"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Warn}},
		}, "cm", nil),
		withCM(vapCase{
			name: "Audit action and auditAnnotations annotate",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match,
				Validations:      []admissionregv1.Validation{{Expression: "false", Message: "audited"}},
				AuditAnnotations: []admissionregv1.AuditAnnotation{{Key: "keys", ValueExpression: "string(object.data.size())"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Audit}},
		}, "cm", map[string]string{"a": "1"}),
		withCM(vapCase{
			name: "binding namespaceSelector narrows the match",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{MatchConstraints: match,
				Validations: []admissionregv1.Validation{{Expression: "false"}}},
			binding: admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny},
				MatchResources: &admissionregv1.MatchResources{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"policy": "off"}}}},
		}, "cm", nil),
		{
			name: "policies do not apply to policies",
			spec: admissionregv1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{{RuleWithOperations: admissionregv1.RuleWithOperations{
					Operations: []admissionregv1.OperationType{admissionregv1.Create},
					Rule:       admissionregv1.Rule{APIGroups: []string{"*"}, APIVersions: []string{"*"}, Resources: []string{"*"}},
				}}}},
				Validations: []admissionregv1.Validation{{Expression: "false"}}},
			binding:  admissionregv1.ValidatingAdmissionPolicyBindingSpec{ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}},
			resource: admissionregv1.SchemeGroupVersion.WithResource("validatingadmissionpolicybindings"),
			kind:     admissionregv1.SchemeGroupVersion.WithKind("ValidatingAdmissionPolicyBinding"),
			object:   &admissionregv1.ValidatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: "other"}, Spec: admissionregv1.ValidatingAdmissionPolicyBindingSpec{PolicyName: "other", ValidationActions: []admissionregv1.ValidationAction{admissionregv1.Deny}}},
		},
	}
}

func ptrTo[T any](v T) *T { return &v }

var semanticObjects = cmp.Comparer(func(a, b runtime.Object) bool { return apiequality.Semantic.DeepEqual(a, b) })

type policyOutcome struct {
	Allowed     bool
	Reason      metav1.StatusReason
	Code        int32
	Message     string
	Warnings    []string
	Annotations map[string]string
	Object      runtime.Object
}

func TestValidatingAdmissionPolicyMatchesUpstream(t *testing.T) {
	for _, c := range vapCases() {
		t.Run(c.name, func(t *testing.T) {
			policy := defaulted(&admissionregv1.ValidatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy"}, Spec: c.spec})
			c.binding.PolicyName = "policy"
			binding := defaulted(&admissionregv1.ValidatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: "binding"}, Spec: c.binding})
			cluster := []runtime.Object{policyNamespace, policy, binding}
			for _, p := range c.params {
				cluster = append(cluster, p)
			}
			upstream := runUpstreamPolicy(t, "validate", cluster, c.params, c.resource, c.kind, c.object)
			ours := runOurPolicy(t, "validate", cluster, c.params, c.resource, c.kind, c.object)
			if diff := cmp.Diff(upstream, ours, semanticObjects); diff != "" {
				t.Fatalf("outcome differs from upstream (- upstream, + ours):\n%s", diff)
			}
		})
	}
}

type mapCase struct {
	name    string
	spec    admissionregv1.MutatingAdmissionPolicySpec
	params  []*corev1.ConfigMap
	binding admissionregv1.MutatingAdmissionPolicyBindingSpec
}

func TestMutatingAdmissionPolicyMatchesUpstream(t *testing.T) {
	deny := admissionregv1.DenyAction
	match := &admissionregv1.MatchResources{ResourceRules: []admissionregv1.NamedRuleWithOperations{deploymentRule()}}
	cases := []mapCase{
		{
			name: "apply configuration merges containers by name",
			spec: admissionregv1.MutatingAdmissionPolicySpec{MatchConstraints: match, Mutations: []admissionregv1.Mutation{{
				PatchType: admissionregv1.PatchTypeApplyConfiguration,
				ApplyConfiguration: &admissionregv1.ApplyConfiguration{Expression: `Object{spec: Object.spec{template: Object.spec.template{spec: Object.spec.template.spec{
					containers: [Object.spec.template.spec.containers{name: "app", imagePullPolicy: "Always"}]}}}}`},
			}}},
		},
		{
			name: "param by name is bound",
			spec: admissionregv1.MutatingAdmissionPolicySpec{MatchConstraints: match, ParamKind: &admissionregv1.ParamKind{APIVersion: "v1", Kind: "ConfigMap"}, Mutations: []admissionregv1.Mutation{{
				PatchType: admissionregv1.PatchTypeJSONPatch,
				JSONPatch: &admissionregv1.JSONPatch{Expression: `[JSONPatch{op: "replace", path: "/spec/replicas", value: int(params.data.replicas)}]`},
			}}},
			params:  []*corev1.ConfigMap{paramsConfigMap("replicas", map[string]string{"replicas": "3"})},
			binding: admissionregv1.MutatingAdmissionPolicyBindingSpec{ParamRef: &admissionregv1.ParamRef{Name: "replicas", ParameterNotFoundAction: &deny}},
		},
		{
			name: "match condition skips the policy",
			spec: admissionregv1.MutatingAdmissionPolicySpec{MatchConstraints: match,
				MatchConditions: []admissionregv1.MatchCondition{{Name: "only-big", Expression: "object.spec.replicas > 5"}},
				Mutations: []admissionregv1.Mutation{{
					PatchType: admissionregv1.PatchTypeJSONPatch,
					JSONPatch: &admissionregv1.JSONPatch{Expression: `[JSONPatch{op: "replace", path: "/spec/replicas", value: 5}]`},
				}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.spec.ReinvocationPolicy = admissionregv1.NeverReinvocationPolicy
			policy := defaulted(&admissionregv1.MutatingAdmissionPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy"}, Spec: c.spec})
			c.binding.PolicyName = "policy"
			binding := defaulted(&admissionregv1.MutatingAdmissionPolicyBinding{ObjectMeta: metav1.ObjectMeta{Name: "binding"}, Spec: c.binding})
			cluster := []runtime.Object{policyNamespace, policy, binding}
			for _, p := range c.params {
				cluster = append(cluster, p)
			}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team"},
				Spec: appsv1.DeploymentSpec{Replicas: ptrTo(int32(1)), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx"}, {Name: "sidecar", Image: "envoy"}},
				}}},
			}
			resource, kind := appsv1.SchemeGroupVersion.WithResource("deployments"), appsv1.SchemeGroupVersion.WithKind("Deployment")
			upstream := runUpstreamPolicy(t, "admit", cluster, c.params, resource, kind, deployment)
			ours := runOurPolicy(t, "admit", cluster, c.params, resource, kind, deployment)
			if diff := cmp.Diff(upstream, ours, semanticObjects); diff != "" {
				t.Fatalf("outcome differs from upstream (- upstream, + ours):\n%s", diff)
			}
		})
	}
}

type annotationRecorder struct {
	admission.Attributes
	annotations map[string]string
}

func (a *annotationRecorder) AddAnnotation(key, value string) error {
	a.annotations[key] = value
	return a.Attributes.AddAnnotation(key, value)
}

type warningList struct{ warnings []string }

func (w *warningList) AddWarning(_, text string) { w.warnings = append(w.warnings, text) }

type openAPIClientset struct {
	*fake.Clientset
	disco discovery.DiscoveryInterface
}

func (c openAPIClientset) Discovery() discovery.DiscoveryInterface { return c.disco }

func runUpstreamPolicy(t *testing.T, phase string, cluster []runtime.Object, params []*corev1.ConfigMap, resource schema.GroupVersionResource, kind schema.GroupVersionKind, object runtime.Object) policyOutcome {
	t.Helper()
	api := newPolicyAPI(t, params)
	client := fake.NewSimpleClientset(cluster...)
	factory := informers.NewSharedInformerFactory(client, 0)
	var paramObjects []runtime.Object
	for _, p := range params {
		paramObjects = append(paramObjects, p)
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, paramObjects...)
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
	stop := make(chan struct{})
	defer close(stop)

	var plugin admission.Interface
	var setup interface {
		SetExternalKubeInformerFactory(informers.SharedInformerFactory)
		SetExternalKubeClientSet(kubernetes.Interface)
		SetRESTMapper(meta.RESTMapper)
		SetDynamicClient(dynamic.Interface)
		SetDrainedNotification(<-chan struct{})
		SetAuthorizer(authorizer.Authorizer)
		ValidateInitialization() error
	}
	if phase == "admit" {
		p, err := mutating.NewPlugin(nil)
		if err != nil {
			t.Fatal(err)
		}
		p.InspectFeatureGates(utilfeature.DefaultFeatureGate)
		plugin, setup = p, p
		disco, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: api.URL})
		if err != nil {
			t.Fatal(err)
		}
		setup.SetExternalKubeClientSet(openAPIClientset{Clientset: client, disco: disco})
	} else {
		p, err := validating.NewPlugin(nil)
		if err != nil {
			t.Fatal(err)
		}
		plugin, setup = p, p
		setup.SetExternalKubeClientSet(client)
	}
	setup.SetExternalKubeInformerFactory(factory)
	setup.SetRESTMapper(mapper)
	setup.SetDynamicClient(dynamicClient)
	setup.SetDrainedNotification(stop)
	setup.SetAuthorizer(noOpinion{})
	if err := setup.ValidateInitialization(); err != nil {
		t.Fatal(err)
	}
	factory.Start(stop)
	factory.WaitForCacheSync(stop)

	versioned := versionedCopy(t, object)
	deadline := time.Now().Add(10 * time.Second)
	for {
		internal := toInternal(t, versioned)
		req := admit.Request{Name: objectName(object), Namespace: objectNamespace(object), Resource: resource, Kind: kind, Operation: string(admission.Create), User: admitUser(diffCase{}.userInfo())}
		attrs := &annotationRecorder{Attributes: requestAttributes(&req, internal, nil), annotations: map[string]string{}}
		warnings := &warningList{}
		ctx := warning.WithWarningRecorder(context.Background(), warnings)
		var err error
		if phase == "admit" {
			err = plugin.(admission.MutationInterface).Admit(ctx, attrs, legacyObjectInterfaces)
		} else {
			err = plugin.(admission.ValidationInterface).Validate(ctx, attrs, legacyObjectInterfaces)
		}
		if apierrors.IsServiceUnavailable(err) && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		out := policyOutcomeFrom(err, warnings.warnings, attrs.annotations)
		if err == nil && phase == "admit" {
			out.Object = toVersioned(t, attrs.GetObject(), kind.GroupVersion())
		}
		return out
	}
}

func runOurPolicy(t *testing.T, phase string, cluster []runtime.Object, params []*corev1.ConfigMap, resource schema.GroupVersionResource, kind schema.GroupVersionKind, object runtime.Object) policyOutcome {
	t.Helper()
	data := map[string][]byte{}
	for _, obj := range cluster {
		data[registryKey(obj)] = mustJSON(t, obj)
	}
	kineSrv := httptest.NewServer(&memStore{data: data})
	defer kineSrv.Close()
	api := newPolicyAPI(t, params)
	h := NewHandler(Config{Kine: rewriteClient(kineSrv), API: rewriteClient(api)})
	versioned := versionedCopy(t, object)
	resp := postAdmit(t, h, admit.Request{
		Phase: phase, Name: objectName(object), Namespace: objectNamespace(object), Resource: resource, Kind: kind,
		Operation: string(admission.Create), Object: toMapObject(t, withKind(versioned, kind)), User: admitUser(diffCase{}.userInfo()),
	})
	var err error
	if !resp.Allowed && resp.Status != nil {
		err = &apierrors.StatusError{ErrStatus: *resp.Status}
	} else if !resp.Allowed {
		err = errors.New(resp.Reason + ": " + resp.Message)
	}
	out := policyOutcomeFrom(err, resp.Warnings, resp.AuditAnnotations)
	if err == nil && phase == "admit" {
		mutated := fromMapObject(t, resp.Object, versioned)
		legacyscheme.Scheme.Default(mutated)
		out.Object = mutated
	}
	return out
}

func policyOutcomeFrom(err error, warnings []string, annotations map[string]string) policyOutcome {
	out := policyOutcome{Allowed: err == nil, Warnings: warnings}
	if len(annotations) > 0 {
		out.Annotations = annotations
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		out.Reason, out.Code, out.Message = status.Status().Reason, status.Status().Code, err.Error()
	} else if err != nil {
		out.Message = err.Error()
	}
	return out
}

func objectName(obj runtime.Object) string { return obj.(metav1.Object).GetName() }

func objectNamespace(obj runtime.Object) string { return obj.(metav1.Object).GetNamespace() }

var (
	specHandlerOnce sync.Once
	specHandler     http.Handler
	specHandlerErr  error
)

func newPolicyAPI(t *testing.T, params []*corev1.ConfigMap) *httptest.Server {
	t.Helper()
	specHandlerOnce.Do(func() { specHandler, specHandlerErr = openapi.SpecHandler() })
	if specHandlerErr != nil {
		t.Fatal(specHandlerErr)
	}
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.Handle("/openapi/", specHandler)
	mux.HandleFunc("GET /api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, metav1.APIVersions{TypeMeta: metav1.TypeMeta{Kind: "APIVersions"}, Versions: []string{"v1"}})
	})
	mux.HandleFunc("GET /apis", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"}})
	})
	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, metav1.APIResourceList{TypeMeta: metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"}, GroupVersion: "v1", APIResources: []metav1.APIResource{
			{Name: "configmaps", Namespaced: true, Kind: "ConfigMap", Verbs: metav1.Verbs{"get", "list"}},
		}})
	})
	mux.HandleFunc("GET /api/v1/namespaces/{ns}/configmaps/{name}", func(w http.ResponseWriter, r *http.Request) {
		for _, p := range params {
			if p.Namespace == r.PathValue("ns") && p.Name == r.PathValue("name") {
				writeJSON(w, withKind(p, corev1.SchemeGroupVersion.WithKind("ConfigMap")))
				return
			}
		}
		status := apierrors.NewNotFound(corev1.Resource("configmaps"), r.PathValue("name")).Status()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(withKind(&status, metav1.SchemeGroupVersion.WithKind("Status")))
	})
	mux.HandleFunc("GET /api/v1/namespaces/{ns}/configmaps", func(w http.ResponseWriter, r *http.Request) {
		selector, err := labels.Parse(r.URL.Query().Get("labelSelector"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		list := corev1.ConfigMapList{TypeMeta: metav1.TypeMeta{Kind: "ConfigMapList", APIVersion: "v1"}}
		for _, p := range params {
			if p.Namespace == r.PathValue("ns") && selector.Matches(labels.Set(p.Labels)) {
				list.Items = append(list.Items, *p)
			}
		}
		sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Name < list.Items[j].Name })
		writeJSON(w, list)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
