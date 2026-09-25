package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	pdbhelper "k8s.io/component-helpers/apps/poddisruptionbudget"
)

func init() {
	utilruntime.Must(policyv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(policyv1beta1.AddToScheme(scheme.Scheme))
}

type evictionREST struct {
	pods *registry.Store
	kine *kine.Client
}

var (
	_ rest.NamedCreater             = (*evictionREST)(nil)
	_ rest.GroupVersionKindProvider = (*evictionREST)(nil)
	_ rest.GroupVersionAcceptor     = (*evictionREST)(nil)
	_ rest.Scoper                   = (*evictionREST)(nil)
	_ rest.SingularNameProvider     = (*evictionREST)(nil)
)

var evictionGVK = schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "Eviction"}

var evictionsRetry = wait.Backoff{Steps: 20, Duration: 500 * time.Millisecond, Factor: 1.0, Jitter: 0.1}

func newEvictionREST(pods *registry.Store, client *kine.Client) rest.Storage {
	return &evictionREST{pods: pods, kine: client}
}

func (evictionREST) New() runtime.Object     { return &policyv1.Eviction{} }
func (evictionREST) Destroy()                {}
func (evictionREST) NamespaceScoped() bool   { return true }
func (evictionREST) GetSingularName() string { return "eviction" }
func (evictionREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return evictionGVK
}
func (evictionREST) AcceptsGroupVersion(gv schema.GroupVersion) bool {
	return gv == policyv1.SchemeGroupVersion || gv == policyv1beta1.SchemeGroupVersion
}

func (r *evictionREST) Create(ctx context.Context, name string, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	eviction, err := evictionFrom(obj)
	if err != nil {
		return nil, err
	}
	if name != eviction.Name {
		return nil, apierrors.NewBadRequest("name in URL does not match name in Eviction object")
	}
	deleteOptions, err := evictionDeleteOptions(eviction, options)
	if err != nil {
		return nil, err
	}
	if createValidation != nil {
		if err := createValidation(ctx, eviction.DeepCopyObject()); err != nil {
			return nil, err
		}
	}
	var pod *corev1.Pod
	deleted := false
	shouldRetry := apierrors.IsConflict
	if !resourceVersionUnset(deleteOptions) {
		shouldRetry = func(error) bool { return false }
	}
	err = retry.OnError(evictionsRetry, shouldRetry, func() error {
		got, getErr := r.pods.Get(ctx, name, &metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		next, ok := got.(*corev1.Pod)
		if !ok {
			return apierrors.NewInternalError(fmt.Errorf("unexpected pod type %T", got))
		}
		pod = next
		if !canIgnorePDB(pod) {
			return nil
		}
		opts := deleteOptions
		if shouldPinPodResourceVersion(pod) && resourceVersionUnset(deleteOptions) {
			opts = deleteOptions.DeepCopy()
			rv := pod.ResourceVersion
			if opts.Preconditions == nil {
				opts.Preconditions = &metav1.Preconditions{}
			}
			opts.Preconditions.ResourceVersion = &rv
		}
		if delErr := r.deletePod(ctx, name, opts); delErr != nil {
			return delErr
		}
		deleted = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if deleted {
		return &metav1.Status{Status: metav1.StatusSuccess}, nil
	}
	pinnedPDB, err := r.guardPDB(ctx, pod, dryrun.IsDryRun(deleteOptions.DryRun))
	if err != nil {
		return nil, err
	}
	if pinnedPDB != "" && shouldPinPodResourceVersion(pod) && resourceVersionUnset(deleteOptions) {
		deleteOptions = deleteOptions.DeepCopy()
		rv := pod.ResourceVersion
		if deleteOptions.Preconditions == nil {
			deleteOptions.Preconditions = &metav1.Preconditions{}
		}
		deleteOptions.Preconditions.ResourceVersion = &rv
	}
	if err := r.deletePod(ctx, name, deleteOptions); err != nil {
		if pinnedPDB != "" && apierrors.IsConflict(err) && resourceVersionUnset(eviction.DeleteOptions) {
			return nil, tooManyRequests(pinnedPDB)
		}
		return nil, err
	}
	return &metav1.Status{Status: metav1.StatusSuccess}, nil
}

func evictionFrom(obj runtime.Object) (*policyv1.Eviction, error) {
	switch o := obj.(type) {
	case *policyv1.Eviction:
		return o, nil
	case *policyv1beta1.Eviction:
		return &policyv1.Eviction{ObjectMeta: o.ObjectMeta, DeleteOptions: o.DeleteOptions}, nil
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("not a Eviction object: %T", obj))
	}
}

func evictionDeleteOptions(eviction *policyv1.Eviction, options *metav1.CreateOptions) (*metav1.DeleteOptions, error) {
	if options == nil {
		options = &metav1.CreateOptions{}
	}
	if eviction.DeleteOptions == nil {
		return &metav1.DeleteOptions{DryRun: options.DryRun}, nil
	}
	if len(eviction.DeleteOptions.DryRun) == 0 {
		eviction.DeleteOptions.DryRun = options.DryRun
		return eviction.DeleteOptions, nil
	}
	if len(options.DryRun) == 0 {
		return eviction.DeleteOptions, nil
	}
	if fmt.Sprint(options.DryRun) != fmt.Sprint(eviction.DeleteOptions.DryRun) {
		return nil, fmt.Errorf("Non-matching dry-run options in request and content: %v and %v", options.DryRun, eviction.DeleteOptions.DryRun)
	}
	return eviction.DeleteOptions, nil
}

func (r *evictionREST) deletePod(ctx context.Context, name string, options *metav1.DeleteOptions) error {
	if !dryrun.IsDryRun(options.DryRun) {
		_, _, err := r.pods.Update(ctx, name, rest.DefaultUpdatedObjectInfo(nil, func(_ context.Context, _, old runtime.Object) (runtime.Object, error) {
			pod := old.DeepCopyObject().(*corev1.Pod)
			if err := matchPodPreconditions(pod, options); err != nil {
				return nil, err
			}
			setDisruptionTarget(pod)
			return pod, nil
		}), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		if !resourceVersionUnset(options) {
			latest, getErr := r.pods.Get(ctx, name, &metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			options = options.DeepCopy()
			rv := latest.(*corev1.Pod).ResourceVersion
			options.Preconditions.ResourceVersion = &rv
		}
	} else if got, err := r.pods.Get(ctx, name, &metav1.GetOptions{}); err != nil {
		return err
	} else if err := matchPodPreconditions(got.(*corev1.Pod), options); err != nil {
		return err
	}
	_, _, err := r.pods.Delete(ctx, name, rest.ValidateAllObjectFunc, options)
	return err
}

func matchPodPreconditions(pod *corev1.Pod, options *metav1.DeleteOptions) error {
	if options == nil || options.Preconditions == nil {
		return nil
	}
	if uid := options.Preconditions.UID; uid != nil && len(*uid) > 0 && *uid != pod.UID {
		return apierrors.NewConflict(corev1.Resource("pods"), pod.Name, fmt.Errorf("the UID in the precondition (%s) does not match the UID in record (%s). The object might have been deleted and then recreated", *uid, pod.UID))
	}
	if rv := options.Preconditions.ResourceVersion; rv != nil && len(*rv) > 0 && *rv != pod.ResourceVersion {
		return apierrors.NewConflict(corev1.Resource("pods"), pod.Name, fmt.Errorf("the ResourceVersion in the precondition (%s) does not match the ResourceVersion in record (%s). The object might have been modified", *rv, pod.ResourceVersion))
	}
	return nil
}

func resourceVersionUnset(options *metav1.DeleteOptions) bool {
	return options == nil || options.Preconditions == nil || options.Preconditions.ResourceVersion == nil || len(*options.Preconditions.ResourceVersion) == 0
}

func shouldPinPodResourceVersion(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed || !pod.DeletionTimestamp.IsZero() {
		return false
	}
	return true
}

func (r *evictionREST) guardPDB(ctx context.Context, pod *corev1.Pod, dryRun bool) (string, error) {
	pdbs, err := r.matchingPDBs(ctx, pod)
	if err != nil {
		return "", err
	}
	if len(pdbs) > 1 {
		return "", apierrors.NewInternalError(fmt.Errorf("This pod has more than one PodDisruptionBudget, which the eviction subresource does not support."))
	}
	if len(pdbs) == 0 {
		return "", nil
	}
	item := pdbs[0]
	pdb := item.pdb
	if !podReady(pod) {
		if pdb.Spec.UnhealthyPodEvictionPolicy != nil && *pdb.Spec.UnhealthyPodEvictionPolicy == policyv1.AlwaysAllow {
			return pdb.Name, nil
		}
		if pdb.Status.CurrentHealthy >= pdb.Status.DesiredHealthy && pdb.Status.DesiredHealthy > 0 {
			return pdb.Name, nil
		}
	}
	refresh := false
	err = retry.RetryOnConflict(evictionsRetry, func() error {
		if refresh {
			pdbs, err = r.matchingPDBs(ctx, pod)
			if err != nil {
				return err
			}
			if len(pdbs) > 1 {
				return apierrors.NewInternalError(fmt.Errorf("This pod has more than one PodDisruptionBudget, which the eviction subresource does not support."))
			}
			if len(pdbs) == 0 {
				return nil
			}
			item = pdbs[0]
		}
		decErr := r.checkAndDecrement(ctx, item, pod.Name, dryRun)
		if decErr != nil {
			refresh = true
			return decErr
		}
		return nil
	})
	if apierrors.IsConflict(err) {
		return "", tooManyRequests(item.pdb.Name)
	}
	return "", err
}

type pdbItem struct {
	key      string
	revision int64
	pdb      policyv1.PodDisruptionBudget
}

func (r *evictionREST) matchingPDBs(ctx context.Context, pod *corev1.Pod) ([]pdbItem, error) {
	if r.kine == nil {
		return nil, nil
	}
	kvs, _, _, err := r.kine.List(ctx, "/registry/poddisruptionbudgets/"+pod.Namespace+"/", "", 0)
	if err != nil {
		return nil, err
	}
	var out []pdbItem
	for _, kv := range kvs {
		data, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			return nil, err
		}
		var pdb policyv1.PodDisruptionBudget
		if err := json.Unmarshal(data, &pdb); err != nil {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		out = append(out, pdbItem{key: kv.Key, revision: kv.ModRevision, pdb: pdb})
	}
	return out, nil
}

func (r *evictionREST) checkAndDecrement(ctx context.Context, item pdbItem, podName string, dryRun bool) error {
	pdb := item.pdb
	if pdb.Status.ObservedGeneration < pdb.Generation {
		return tooManyRequests(pdb.Name)
	}
	if pdb.Status.DisruptionsAllowed < 0 {
		return apierrors.NewForbidden(policyv1.Resource("poddisruptionbudget"), pdb.Name, fmt.Errorf("pdb disruptions allowed is negative"))
	}
	if len(pdb.Status.DisruptedPods) > 2000 {
		return apierrors.NewForbidden(policyv1.Resource("poddisruptionbudget"), pdb.Name, fmt.Errorf("DisruptedPods map too big - too many evictions not confirmed by PDB controller"))
	}
	if pdb.Status.DisruptionsAllowed == 0 {
		err := apierrors.NewTooManyRequests("Cannot evict pod as it would violate the pod's disruption budget.", 0)
		err.ErrStatus.Details.Causes = append(err.ErrStatus.Details.Causes, metav1.StatusCause{
			Type:    policyv1.DisruptionBudgetCause,
			Message: disruptionDeniedMessage(pdb),
		})
		return err
	}
	if dryRun {
		return nil
	}
	pdb.Status.DisruptionsAllowed--
	if pdb.Status.DisruptionsAllowed == 0 {
		pdbhelper.UpdateDisruptionAllowedCondition(&pdb)
	}
	if pdb.Status.DisruptedPods == nil {
		pdb.Status.DisruptedPods = map[string]metav1.Time{}
	}
	pdb.Status.DisruptedPods[podName] = metav1.Time{Time: time.Now()}
	data, err := runtime.Encode(scheme.Codecs.LegacyCodec(policyv1.SchemeGroupVersion), &pdb)
	if err != nil {
		return err
	}
	_, err = r.kine.Put(ctx, item.key, data, item.revision)
	if err == kine.ErrConflict {
		return apierrors.NewConflict(policyv1.Resource("poddisruptionbudget"), pdb.Name, err)
	}
	return err
}

func disruptionDeniedMessage(pdb policyv1.PodDisruptionBudget) string {
	condition := meta.FindStatusCondition(pdb.Status.Conditions, policyv1.DisruptionAllowedCondition)
	switch {
	case condition != nil && condition.Status == metav1.ConditionFalse && len(condition.Message) > 0 && condition.Reason == policyv1.SyncFailedReason:
		return fmt.Sprintf("The disruption budget %s does not allow evicting pods currently because it failed sync: %s", pdb.Name, condition.Message)
	case pdb.Status.CurrentHealthy <= pdb.Status.DesiredHealthy:
		return fmt.Sprintf("The disruption budget %s needs %d healthy pods and has %d currently", pdb.Name, pdb.Status.DesiredHealthy, pdb.Status.CurrentHealthy)
	case condition != nil && condition.Status == metav1.ConditionFalse && len(condition.Message) > 0:
		return fmt.Sprintf("The disruption budget %s does not allow evicting pods currently (%s): %s", pdb.Name, condition.Reason, condition.Message)
	default:
		return fmt.Sprintf("The disruption budget %s does not allow evicting pods currently", pdb.Name)
	}
}

func tooManyRequests(name string) error {
	err := apierrors.NewTooManyRequests("Cannot evict pod as it would violate the pod's disruption budget.", 10)
	err.ErrStatus.Details.Causes = append(err.ErrStatus.Details.Causes, metav1.StatusCause{
		Type:    policyv1.DisruptionBudgetCause,
		Message: fmt.Sprintf("The disruption budget %s is still being processed by the server.", name),
	})
	return err
}

func canIgnorePDB(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodPending || !pod.ObjectMeta.DeletionTimestamp.IsZero() {
		return true
	}
	return false
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func setDisruptionTarget(pod *corev1.Pod) {
	for i := range pod.Status.Conditions {
		c := &pod.Status.Conditions[i]
		if c.Type != corev1.DisruptionTarget {
			continue
		}
		keep := c.LastTransitionTime
		alreadyTrue := c.Status == corev1.ConditionTrue
		c.Status = corev1.ConditionTrue
		c.Reason = "EvictionByEvictionAPI"
		c.Message = "Eviction API: evicting"
		if alreadyTrue {
			c.LastTransitionTime = keep
			return
		}
		c.LastTransitionTime = metav1.Now()
		return
	}
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               corev1.DisruptionTarget,
		Status:             corev1.ConditionTrue,
		Reason:             "EvictionByEvictionAPI",
		Message:            "Eviction API: evicting",
		LastTransitionTime: metav1.Now(),
	})
}
