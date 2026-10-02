package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/apiserver/pkg/admission/plugin/resourcequota"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/util/compatibility"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	_ "k8s.io/kubernetes/pkg/apis/apps/install"
	_ "k8s.io/kubernetes/pkg/apis/authentication/install"
	_ "k8s.io/kubernetes/pkg/apis/certificates/install"
	_ "k8s.io/kubernetes/pkg/apis/coordination/install"
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	_ "k8s.io/kubernetes/pkg/apis/networking/install"
	_ "k8s.io/kubernetes/pkg/apis/node/install"
	_ "k8s.io/kubernetes/pkg/apis/policy/install"
	_ "k8s.io/kubernetes/pkg/apis/resource/install"
	_ "k8s.io/kubernetes/pkg/apis/scheduling/install"
	_ "k8s.io/kubernetes/pkg/apis/storage/install"
	quotainstall "k8s.io/kubernetes/pkg/quota/v1/install"
	"k8s.io/kubernetes/plugin/pkg/admission/certificates/approval"
	"k8s.io/kubernetes/plugin/pkg/admission/certificates/signing"
	"k8s.io/kubernetes/plugin/pkg/admission/certificates/subjectrestriction"
	"k8s.io/kubernetes/plugin/pkg/admission/defaulttolerationseconds"
	"k8s.io/kubernetes/plugin/pkg/admission/limitranger"
	"k8s.io/kubernetes/plugin/pkg/admission/network/defaultingressclass"
	"k8s.io/kubernetes/plugin/pkg/admission/nodedeclaredfeatures"
	"k8s.io/kubernetes/plugin/pkg/admission/noderestriction"
	"k8s.io/kubernetes/plugin/pkg/admission/nodetaint"
	"k8s.io/kubernetes/plugin/pkg/admission/podresize"
	"k8s.io/kubernetes/plugin/pkg/admission/podtopologylabels"
	"k8s.io/kubernetes/plugin/pkg/admission/priority"
	"k8s.io/kubernetes/plugin/pkg/admission/runtimeclass"
	"k8s.io/kubernetes/plugin/pkg/admission/security/podsecurity"
	"k8s.io/kubernetes/plugin/pkg/admission/serviceaccount"
	"k8s.io/kubernetes/plugin/pkg/admission/storage/persistentvolume/resize"
	"k8s.io/kubernetes/plugin/pkg/admission/storage/storageclass/setdefault"
	"k8s.io/kubernetes/plugin/pkg/admission/storage/storageobjectinuseprotection"
)

type oursFunc func(context.Context, *store, authorizer.Authorizer, *admit.Request) error

type pluginPair struct {
	upstream string
	swapped  bool
	admit    oursFunc
	validate oursFunc
}

func serviceAccountAdmit(ctx context.Context, s *store, _ authorizer.Authorizer, req *admit.Request) error {
	if err := applyServiceAccount(ctx, s, req); err != nil {
		return err
	}
	return validateServiceAccount(ctx, s, req)
}

func storeOnly(fn func(context.Context, *store, *admit.Request) error) oursFunc {
	return func(ctx context.Context, s *store, _ authorizer.Authorizer, req *admit.Request) error {
		return fn(ctx, s, req)
	}
}

func authorizerOnly(fn func(context.Context, authorizer.Authorizer, *admit.Request) error) oursFunc {
	return func(ctx context.Context, _ *store, authz authorizer.Authorizer, req *admit.Request) error {
		return fn(ctx, authz, req)
	}
}

var pluginPairs = map[string]pluginPair{
	"DefaultTolerationSeconds":      {upstream: defaulttolerationseconds.PluginName, swapped: true, admit: storeOnly(applyDefaultTolerationSeconds)},
	"LimitRanger":                   {upstream: limitranger.PluginName, swapped: true, admit: storeOnly(applyLimitRanger), validate: storeOnly(validateLimitRanger)},
	"DefaultStorageClass":           {upstream: setdefault.PluginName, swapped: true, admit: storeOnly(applyDefaultStorageClass)},
	"DefaultIngressClass":           {upstream: defaultingressclass.PluginName, swapped: true, admit: storeOnly(applyDefaultIngressClass)},
	"StorageObjectInUseProtection":  {upstream: storageobjectinuseprotection.PluginName, swapped: true, admit: storeOnly(applyStorageObjectInUseProtection)},
	"RuntimeClass":                  {upstream: runtimeclass.PluginName, swapped: true, admit: storeOnly(applyRuntimeClass), validate: storeOnly(validateRuntimeClass)},
	"TaintNodesByCondition":         {upstream: nodetaint.PluginName, swapped: true, admit: storeOnly(applyTaintNodesByCondition)},
	"PodTopologyLabels":             {upstream: podtopologylabels.PluginName, swapped: true, admit: storeOnly(applyPodTopologyLabels)},
	"PersistentVolumeClaimResize":   {upstream: resize.PluginName, validate: storeOnly(applyPersistentVolumeClaimResize)},
	"CertificateSubjectRestriction": {upstream: subjectrestriction.PluginName, swapped: true, validate: storeOnly(applyCertificateSubjectRestriction)},
	"CertificateApproval":           {upstream: approval.PluginName, validate: authorizerOnly(applyCertificateApproval)},
	"CertificateSigning":            {upstream: signing.PluginName, validate: authorizerOnly(applyCertificateSigning)},
	"ServiceAccount":                {upstream: serviceaccount.PluginName, admit: serviceAccountAdmit, validate: storeOnly(validateServiceAccount)},
	"NodeRestriction":               {upstream: noderestriction.PluginName, admit: storeOnly(applyNodeRestriction)},
	"PodSecurity":                   {upstream: podsecurity.PluginName, validate: storeOnly(applyPodSecurity)},
	"ResourceQuota":                 {upstream: resourcequota.PluginName, validate: storeOnly(applyResourceQuota)},
	"PodResize":                     {upstream: podresize.PluginName, validate: storeOnly(validatePodResize)},
	"NodeDeclaredFeatures":          {upstream: nodedeclaredfeatures.PluginName, validate: storeOnly(validateNodeDeclaredFeatures)},
	"Priority":                      {upstream: priority.PluginName, swapped: true, admit: storeOnly(applyPriority), validate: storeOnly(validatePriorityClass)},
}

var upstreamPlugins = func() *admission.Plugins {
	plugins := admission.NewPlugins()
	defaulttolerationseconds.Register(plugins)
	priority.Register(plugins)
	limitranger.Register(plugins)
	podresize.Register(plugins)
	nodedeclaredfeatures.Register(plugins)
	resourcequota.Register(plugins)
	podsecurity.Register(plugins)
	noderestriction.Register(plugins)
	serviceaccount.Register(plugins)
	subjectrestriction.Register(plugins)
	approval.Register(plugins)
	signing.Register(plugins)
	runtimeclass.Register(plugins)
	nodetaint.Register(plugins)
	podtopologylabels.Register(plugins)
	resize.Register(plugins)
	setdefault.Register(plugins)
	defaultingressclass.Register(plugins)
	storageobjectinuseprotection.Register(plugins)
	return plugins
}()

type objectRewrite func(runtime.Object)

type knownDifference struct {
	reason      string
	signature   string
	messageOnly bool
	rewrites    []objectRewrite
}

type diffCase struct {
	name        string
	plugin      string
	phase       string
	operation   admission.Operation
	resource    schema.GroupVersionResource
	kind        schema.GroupVersionKind
	subresource string
	namespace   string
	objName     string
	user        user.Info
	object      runtime.Object
	oldObject   runtime.Object
	cluster     []runtime.Object
	dryRun      bool
	authorizer  authorizer.Authorizer
	known       *knownDifference
}

type outcome struct {
	allowed bool
	reason  metav1.StatusReason
	code    int32
	message string
	details *metav1.StatusDetails
	object  runtime.Object

	quotaUsed map[string]corev1.ResourceList
}

func (c diffCase) phaseName() string {
	if c.phase == "" {
		return "admit"
	}
	return c.phase
}

func (c diffCase) op() admission.Operation {
	if c.operation == "" {
		return admission.Create
	}
	return c.operation
}

func (c diffCase) userInfo() user.Info {
	if c.user == nil {
		return &user.DefaultInfo{Name: "alice", Groups: []string{"system:authenticated"}}
	}
	return c.user
}

func (c diffCase) attributes(object, oldObject runtime.Object) admission.Attributes {
	req := admit.Request{
		Name:        c.objName,
		Namespace:   c.namespace,
		Resource:    c.resource,
		Subresource: c.subresource,
		Operation:   string(c.op()),
		DryRun:      c.dryRun,
		Kind:        c.kind,
		User:        admitUser(c.userInfo()),
	}
	return requestAttributes(&req, object, oldObject)
}

func versionedCopy(t *testing.T, obj runtime.Object) runtime.Object {
	t.Helper()
	if obj == nil {
		return nil
	}
	out := obj.DeepCopyObject()
	legacyscheme.Scheme.Default(out)
	return out
}

func toInternal(t *testing.T, obj runtime.Object) runtime.Object {
	t.Helper()
	if obj == nil {
		return nil
	}
	out, err := toInternalObject(obj)
	if err != nil {
		t.Fatalf("convert to internal: %v", err)
	}
	return out
}

func toVersioned(t *testing.T, obj runtime.Object, gv schema.GroupVersion) runtime.Object {
	t.Helper()
	if obj == nil {
		return nil
	}
	out, err := toVersionedObject(obj, gv)
	if err != nil {
		t.Fatalf("convert to versioned: %v", err)
	}
	return out
}

func withKind(obj runtime.Object, kind schema.GroupVersionKind) runtime.Object {
	if obj == nil {
		return nil
	}
	out := obj.DeepCopyObject()
	out.GetObjectKind().SetGroupVersionKind(kind)
	return out
}

func toMapObject(t *testing.T, obj runtime.Object) map[string]any {
	t.Helper()
	if obj == nil {
		return nil
	}
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("to unstructured: %v", err)
	}
	return m
}

func fromMapObject(t *testing.T, m map[string]any, like runtime.Object) runtime.Object {
	t.Helper()
	out := like.DeepCopyObject()
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(m, out); err != nil {
		t.Fatalf("from unstructured: %v", err)
	}
	out.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{})
	return out
}

func registryKey(obj runtime.Object) string {
	accessor := obj.(metav1.Object)
	gvks, _, _ := legacyscheme.Scheme.ObjectKinds(obj)
	plural, _ := meta.UnsafeGuessKindToResource(gvks[0])
	resource := plural.Resource
	if accessor.GetNamespace() == "" {
		return "/registry/" + resource + "/" + accessor.GetName()
	}
	return "/registry/" + resource + "/" + accessor.GetNamespace() + "/" + accessor.GetName()
}

type diffInitializer struct {
	stop    <-chan struct{}
	client  kubernetes.Interface
	factory informers.SharedInformerFactory
	authz   authorizer.Authorizer
}

func (i diffInitializer) Initialize(plugin admission.Interface) {
	if w, ok := plugin.(initializer.WantsDrainedNotification); ok {
		w.SetDrainedNotification(i.stop)
	}
	if w, ok := plugin.(initializer.WantsQuotaConfiguration); ok {
		config, err := quotainstall.NewQuotaConfigurationForAdmission(i.factory, nil)
		if err != nil {
			panic(err)
		}
		w.SetQuotaConfiguration(config)
	}
	if w, ok := plugin.(initializer.WantsEffectiveVersion); ok {
		w.InspectEffectiveVersion(compatibility.DefaultKubeEffectiveVersionForTest())
	}
	if w, ok := plugin.(initializer.WantsFeatures); ok {
		w.InspectFeatureGates(utilfeature.DefaultFeatureGate)
	}
	if w, ok := plugin.(initializer.WantsExternalKubeClientSet); ok {
		w.SetExternalKubeClientSet(i.client)
	}
	if w, ok := plugin.(initializer.WantsExternalKubeInformerFactory); ok {
		w.SetExternalKubeInformerFactory(i.factory)
	}
	if w, ok := plugin.(initializer.WantsAuthorizer); ok {
		w.SetAuthorizer(i.authz)
	}
}

type noOpinion struct{}

func (noOpinion) Authorize(context.Context, authorizer.Attributes) (authorizer.Decision, string, error) {
	return authorizer.DecisionNoOpinion, "", nil
}

func runUpstream(t *testing.T, c diffCase, pair pluginPair) outcome {
	t.Helper()
	if c.authorizer == nil {
		c.authorizer = noOpinion{}
	}
	versioned := versionedCopy(t, c.object)
	versionedOld := versionedCopy(t, c.oldObject)
	internal := toInternal(t, versioned)
	internalOld := toInternal(t, versionedOld)

	client := fake.NewSimpleClientset(c.cluster...)
	factory := informers.NewSharedInformerFactory(client, 0)
	stop := make(chan struct{})
	defer close(stop)
	plugin, err := upstreamPlugins.InitPlugin(pair.upstream, nil, diffInitializer{stop: stop, client: client, factory: factory, authz: c.authorizer})
	if err != nil {
		t.Fatalf("init upstream %s: %v", pair.upstream, err)
	}
	if v, ok := plugin.(admission.InitializationValidator); ok {
		if err := v.ValidateInitialization(); err != nil {
			t.Fatalf("validate upstream initialization: %v", err)
		}
	}
	factory.Start(stop)
	factory.WaitForCacheSync(stop)

	attrs := c.attributes(internal, internalOld)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	objectInterfaces := admission.NewObjectInterfacesFromScheme(legacyscheme.Scheme)

	var runErr error
	if plugin.Handles(c.op()) {
		switch c.phaseName() {
		case "admit":
			m, ok := plugin.(admission.MutationInterface)
			if !ok {
				t.Fatalf("upstream %s has no Admit", pair.upstream)
			}
			runErr = m.Admit(ctx, attrs, objectInterfaces)
		case "validate":
			v, ok := plugin.(admission.ValidationInterface)
			if !ok {
				t.Fatalf("upstream %s has no Validate", pair.upstream)
			}
			runErr = v.Validate(ctx, attrs, objectInterfaces)
		}
	}
	if runErr != nil {
		return upstreamDenial(runErr)
	}
	out := outcome{allowed: true}
	if c.object != nil {
		out.object = toVersioned(t, attrs.GetObject(), c.kind.GroupVersion())
	}
	out.quotaUsed = upstreamQuotaUsed(t, client, c)
	return out
}

func upstreamQuotaUsed(t *testing.T, client kubernetes.Interface, c diffCase) map[string]corev1.ResourceList {
	t.Helper()
	if !c.hasQuotas() {
		return nil
	}
	list, err := client.CoreV1().ResourceQuotas(c.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]corev1.ResourceList{}
	for _, rq := range list.Items {
		used[rq.Name] = rq.Status.Used
	}
	return used
}

func (c diffCase) hasQuotas() bool {
	for _, obj := range c.cluster {
		if _, ok := obj.(*corev1.ResourceQuota); ok {
			return true
		}
	}
	return false
}

func upstreamDenial(err error) outcome {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		s := status.Status()
		return outcome{reason: s.Reason, code: s.Code, message: err.Error(), details: s.Details}
	}
	return outcome{reason: metav1.StatusReasonInternalError, code: 500, message: err.Error()}
}

func runOurs(t *testing.T, c diffCase, pair pluginPair) outcome {
	t.Helper()
	versioned := versionedCopy(t, c.object)
	versionedOld := versionedCopy(t, c.oldObject)

	data := map[string][]byte{}
	for _, obj := range c.cluster {
		raw, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		data[registryKey(obj)] = raw
	}
	backing := &memStore{data: data}
	srv := httptest.NewServer(backing)
	defer srv.Close()
	s := &store{client: &kine.Client{HTTP: rewriteClient(srv)}}

	req := admit.Request{
		Phase:       c.phaseName(),
		Name:        c.objName,
		Namespace:   c.namespace,
		Resource:    c.resource,
		Subresource: c.subresource,
		Operation:   string(c.op()),
		DryRun:      c.dryRun,
		Kind:        c.kind,
		Object:      toMapObject(t, withKind(versioned, c.kind)),
		OldObject:   toMapObject(t, withKind(versionedOld, c.kind)),
		User:        admitUser(c.userInfo()),
	}
	fn := pair.admit
	if c.phaseName() == "validate" {
		fn = pair.validate
	}
	if fn == nil {
		t.Fatalf("this project has no %s function for %s", c.phaseName(), c.plugin)
	}
	err := fn(context.Background(), s, c.authorizer, &req)
	if err != nil {
		return oursDenial(c, err)
	}
	out := outcome{allowed: true}
	out.quotaUsed = oursQuotaUsed(t, backing, c)
	if c.object != nil {
		out.object = fromMapObject(t, req.Object, versioned)
	}
	return out
}

func oursQuotaUsed(t *testing.T, backing *memStore, c diffCase) map[string]corev1.ResourceList {
	t.Helper()
	if !c.hasQuotas() {
		return nil
	}
	used := map[string]corev1.ResourceList{}
	prefix := "/registry/resourcequotas/" + c.namespace + "/"
	for key, raw := range backing.data {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		var rq corev1.ResourceQuota
		if err := json.Unmarshal(raw, &rq); err != nil {
			t.Fatal(err)
		}
		used[rq.Name] = rq.Status.Used
	}
	return used
}

func admitUser(info user.Info) admit.User {
	return admit.User{Username: info.GetName(), UID: info.GetUID(), Groups: info.GetGroups(), Extra: info.GetExtra()}
}

func oursDenial(c diffCase, err error) outcome {
	resp := denyResponse(err)
	if resp.Status != nil {
		return upstreamDenial(&apierrors.StatusError{ErrStatus: *resp.Status})
	}
	attrs := c.attributes(nil, nil)
	var mapped error
	switch resp.Reason {
	case string(metav1.StatusReasonInvalid):
		gk := schema.GroupKind{Group: c.kind.Group, Kind: c.kind.Kind}
		mapped = apierrors.NewInvalid(gk, c.objName, field.ErrorList{field.Invalid(field.NewPath("spec"), "", resp.Message)})
	case string(metav1.StatusReasonInternalError):
		mapped = apierrors.NewInternalError(fmt.Errorf("%s", resp.Message))
	default:
		mapped = admission.NewForbidden(attrs, fmt.Errorf("%s", resp.Message))
	}
	return upstreamDenial(mapped)
}

func (o outcome) summary() string {
	if o.allowed {
		return "allowed"
	}
	return fmt.Sprintf("denied %d %s", o.code, o.reason)
}

func changedLines(diff string) string {
	var lines []string
	for _, line := range strings.Split(diff, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "+") {
			lines = append(lines, strings.Join(strings.Fields(trimmed), " "))
		}
	}
	return strings.Join(lines, "\n")
}

type comparison struct {
	outcomeDiff string
	messageDiff string
	messages    string
}

var objectComparison = []cmp.Option{
	cmpopts.EquateEmpty(),
	cmp.Comparer(func(a, b resource.Quantity) bool { return a.Cmp(b) == 0 }),
}

func normalizePod(obj runtime.Object) runtime.Object {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return obj
	}
	pod = pod.DeepCopy()
	pod.Spec.DeprecatedServiceAccount = ""
	isToken := func(name string) bool {
		return name == "kube-api-access" || strings.HasPrefix(name, "kube-api-access-")
	}
	for i := range pod.Spec.Volumes {
		if isToken(pod.Spec.Volumes[i].Name) {
			pod.Spec.Volumes[i].Name = "kube-api-access"
		}
	}
	for _, containers := range [][]corev1.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
		for i := range containers {
			for j := range containers[i].VolumeMounts {
				if isToken(containers[i].VolumeMounts[j].Name) {
					containers[i].VolumeMounts[j].Name = "kube-api-access"
				}
			}
		}
	}
	return pod
}

func compareOutcomes(upstream, ours outcome, exact bool) comparison {
	var cmpResult comparison
	if !exact {
		upstream.object = normalizePod(upstream.object)
		ours.object = normalizePod(ours.object)
	}
	if upstream.summary() != ours.summary() {
		cmpResult.outcomeDiff = fmt.Sprintf("upstream %s; ours %s", upstream.summary(), ours.summary())
		cmpResult.messages = fmt.Sprintf("upstream %q; ours %q", upstream.message, ours.message)
		return cmpResult
	}
	if upstream.allowed {
		if !apiequality.Semantic.DeepEqual(upstream.object, ours.object) {
			cmpResult.outcomeDiff = "object (- upstream, + ours):\n" + changedLines(cmp.Diff(upstream.object, ours.object, objectComparison...))
		} else if diff := cmp.Diff(upstream.quotaUsed, ours.quotaUsed, objectComparison...); diff != "" {
			cmpResult.outcomeDiff = "quota status.used (- upstream, + ours):\n" + changedLines(diff)
		}
		return cmpResult
	}
	upstreamMessage, oursMessage := upstream.message, ours.message
	if !exact {
		upstreamMessage, oursMessage = normalizeForbiddenMessage(upstreamMessage), normalizeForbiddenMessage(oursMessage)
	}
	if upstreamMessage != oursMessage {
		cmpResult.messageDiff = "differs"
		cmpResult.messages = fmt.Sprintf("upstream %q; ours %q", upstream.message, ours.message)
	}
	if exact && !apiequality.Semantic.DeepEqual(upstream.details, ours.details) {
		cmpResult.outcomeDiff = "status.details (- upstream, + ours):\n" + changedLines(cmp.Diff(upstream.details, ours.details))
	}
	return cmpResult
}

func normalizeForbiddenMessage(msg string) string {
	open := strings.Index(msg, "[")
	close := strings.LastIndex(msg, "]")
	if open < 0 || close < 0 || close <= open {
		return msg
	}
	prefix := msg[:open+1]
	suffix := msg[close:]
	inner := msg[open+1 : close]
	parts := strings.Split(inner, ", ")
	sort.Strings(parts)
	return prefix + strings.Join(parts, ", ") + suffix
}

func (c comparison) signature() string {
	if c.outcomeDiff != "" {
		return "outcome: " + c.outcomeDiff
	}
	if c.messageDiff != "" {
		return "message: " + c.messageDiff
	}
	return ""
}

func runDiffCases(t *testing.T, cases []diffCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.plugin+"/"+c.name, func(t *testing.T) {
			pair, ok := pluginPairs[c.plugin]
			if !ok {
				t.Fatalf("unknown plugin %q", c.plugin)
			}
			upstream := runUpstream(t, c, pair)
			ours := runOurs(t, c, pair)
			t.Logf("upstream: %s; ours: %s", upstream.summary(), ours.summary())
			result := compareOutcomes(upstream, ours, pair.swapped)
			got := result.signature()
			if c.known == nil {
				if got != "" {
					t.Fatalf("this project differs from upstream:\n%s\n%s", got, result.messages)
				}
				return
			}
			if got == "" {
				t.Fatalf("known difference (%s) disappeared", c.known.reason)
			}
			switch {
			case c.known.messageOnly:
				if result.messageDiff == "" {
					t.Fatalf("known difference (%s) changed: want a message-only difference, got\n%s", c.known.reason, got)
				}
			case len(c.known.rewrites) > 0:
				rewritten := upstream
				rewritten.object = upstream.object.DeepCopyObject()
				for _, rewrite := range c.known.rewrites {
					rewrite(rewritten.object)
				}
				if rest := compareOutcomes(rewritten, ours, pair.swapped).signature(); rest != "" {
					t.Fatalf("known difference (%s) changed: after applying it upstream still differs:\n%s", c.known.reason, rest)
				}
			default:
				if got != c.known.signature {
					t.Fatalf("known difference (%s) changed\nwant:\n%s\ngot:\n%s\n%s", c.known.reason, c.known.signature, got, result.messages)
				}
			}
		})
	}
}

func admissionOperation(op string) admission.Operation { return admission.Operation(op) }

func applyKnown(t *testing.T, cases []diffCase, known map[string]*knownDifference) []diffCase {
	t.Helper()
	used := map[string]bool{}
	for i := range cases {
		if k, ok := known[cases[i].name]; ok {
			cases[i].known = k
			used[cases[i].name] = true
		}
	}
	for name := range known {
		if !used[name] {
			t.Fatalf("known difference for unknown case %q", name)
		}
	}
	return cases
}

type allowSigners map[string]bool

func (a allowSigners) Authorize(_ context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if attrs.GetResource() == "signers" && a[attrs.GetUser().GetName()+"/"+attrs.GetVerb()+"/"+attrs.GetName()] {
		return authorizer.DecisionAllow, "", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}
