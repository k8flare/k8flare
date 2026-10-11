package admission

import (
	"context"
	"fmt"
	"slices"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/policy/generic"
	"k8s.io/apiserver/pkg/admission/plugin/policy/matching"
	"k8s.io/apiserver/pkg/warning"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/kubeapiserver/admission/exclusion"
)

var policyObjectInterfaces = admission.NewObjectInterfacesFromScheme(runtime.NewScheme())

func policyExcluded(req admit.Request) bool {
	gr := req.Resource.GroupResource()
	return slices.Contains(generic.AdmissionResources(), gr) || slices.Contains(exclusion.Excluded(), gr)
}

func policyAttributes(req admit.Request) admission.Attributes {
	return requestAttributes(&req, policyObject(req.Object, req.Kind), policyObject(req.OldObject, req.Kind))
}

func policyObject(obj map[string]any, kind schema.GroupVersionKind) runtime.Object {
	if obj == nil {
		return nil
	}
	u := &unstructured.Unstructured{Object: asCEL(obj).(map[string]any)}
	u.SetGroupVersionKind(kind)
	return u
}

type admissionRecord struct {
	warnings    []string
	annotations map[string]string
}

type admissionRecordKey struct{}

func withAdmissionRecord(ctx context.Context) (context.Context, *admissionRecord) {
	rec := &admissionRecord{annotations: map[string]string{}}
	return warning.WithWarningRecorder(context.WithValue(ctx, admissionRecordKey{}, rec), rec), rec
}

func (r *admissionRecord) AddWarning(_, text string) {
	r.warnings = append(r.warnings, text)
}

func recordedAttributes(ctx context.Context, a admission.Attributes) admission.Attributes {
	rec, _ := ctx.Value(admissionRecordKey{}).(*admissionRecord)
	if rec == nil {
		return a
	}
	return &recordingAttributes{Attributes: a, record: rec}
}

type recordingAttributes struct {
	admission.Attributes
	record *admissionRecord
}

func (a *recordingAttributes) AddAnnotation(key, value string) error {
	if err := a.Attributes.AddAnnotation(key, value); err != nil {
		return err
	}
	a.record.annotations[key] = value
	return nil
}

func objectMap(obj runtime.Object) map[string]any {
	if obj == nil {
		return nil
	}
	return obj.(*unstructured.Unstructured).Object
}

func (h *Handler) policyMatcher(f *storeInformerFactory) *matching.Matcher {
	lister := f.Core().V1().Namespaces().Lister()
	return matching.NewMatcher(lister, namespaceClient{lister: lister})
}

type namespaceClient struct {
	kubernetes.Interface
	lister listersv1.NamespaceLister
}

func (c namespaceClient) CoreV1() corev1client.CoreV1Interface {
	return namespaceCoreV1{lister: c.lister}
}

type namespaceCoreV1 struct {
	corev1client.CoreV1Interface
	lister listersv1.NamespaceLister
}

func (c namespaceCoreV1) Namespaces() corev1client.NamespaceInterface {
	return namespaceGetter{lister: c.lister}
}

type namespaceGetter struct {
	corev1client.NamespaceInterface
	lister listersv1.NamespaceLister
}

func (g namespaceGetter) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.Namespace, error) {
	return g.lister.Get(name)
}

type policyHookSet[P, B runtime.Object, E generic.Evaluator] struct {
	newPolicyAccessor  func(P) generic.PolicyAccessor
	newBindingAccessor func(B) generic.BindingAccessor
	compile            func(P) E
}

func (s policyHookSet[P, B, E]) hooks(ctx context.Context, h *Handler, a admission.Attributes, m generic.PolicyMatcher, policies []P, bindings []B) []generic.PolicyHook[P, B, E] {
	byPolicy := map[string][]B{}
	for _, b := range bindings {
		name := s.newBindingAccessor(b).GetPolicyName().Name
		byPolicy[name] = append(byPolicy[name], b)
	}
	var params *paramSource
	var out []generic.PolicyHook[P, B, E]
	for _, p := range policies {
		accessor := s.newPolicyAccessor(p)
		bound := byPolicy[accessor.GetName()]
		if len(bound) == 0 {
			continue
		}
		if matches, _, _, err := m.DefinitionMatches(a, policyObjectInterfaces, accessor); err == nil && !matches {
			continue
		}
		hook := generic.PolicyHook[P, B, E]{Policy: p, Bindings: bound}
		if kind := accessor.GetParamKind(); kind != nil {
			gv, err := schema.ParseGroupVersion(kind.APIVersion)
			if err != nil {
				continue
			}
			if params == nil {
				params = h.newParamSource(ctx)
			}
			hook.ParamInformer, hook.ParamScope, hook.ConfigurationError = params.informerFor(gv.WithKind(kind.Kind))
		}
		hook.Evaluator = s.compile(p)
		out = append(out, hook)
	}
	return out
}

type paramSource struct {
	ctx     context.Context
	mapper  meta.RESTMapper
	dynamic dynamic.Interface
	err     error
}

func (h *Handler) newParamSource(ctx context.Context) *paramSource {
	if h.api == nil {
		return &paramSource{err: fmt.Errorf("no API client")}
	}
	disco, err := discovery.NewDiscoveryClientForConfig(h.api)
	if err != nil {
		return &paramSource{err: err}
	}
	dyn, err := dynamic.NewForConfig(h.api)
	if err != nil {
		return &paramSource{err: err}
	}
	return &paramSource{ctx: ctx, mapper: restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco)), dynamic: dyn}
}

func (s *paramSource) informerFor(gvk schema.GroupVersionKind) (informers.GenericInformer, meta.RESTScope, error) {
	if s.err != nil {
		return nil, nil, fmt.Errorf("failed to find resource referenced by paramKind: '%v'", gvk)
	}
	mapping, err := s.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find resource referenced by paramKind: '%v'", gvk)
	}
	return paramInformer{lister: paramLister{ctx: s.ctx, resource: s.dynamic.Resource(mapping.Resource)}}, mapping.Scope, nil
}

type paramInformer struct {
	lister cache.GenericLister
}

func (i paramInformer) Informer() cache.SharedIndexInformer { return storeInformer{} }
func (i paramInformer) Lister() cache.GenericLister         { return i.lister }

type paramLister struct {
	ctx       context.Context
	resource  dynamic.NamespaceableResourceInterface
	namespace string
}

func (l paramLister) client() dynamic.ResourceInterface {
	if l.namespace == "" {
		return l.resource
	}
	return l.resource.Namespace(l.namespace)
}

func (l paramLister) List(selector labels.Selector) ([]runtime.Object, error) {
	list, err := l.client().List(l.ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	out := make([]runtime.Object, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	return out, nil
}

func (l paramLister) Get(name string) (runtime.Object, error) {
	return l.client().Get(l.ctx, name, metav1.GetOptions{})
}

func (l paramLister) ByNamespace(namespace string) cache.GenericNamespaceLister {
	l.namespace = namespace
	return l
}

func pointersTo[T any](items []T) []*T {
	out := make([]*T, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}
