package workloads

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/kubernetes/pkg/controller/namespace/deletion"
)

const (
	namespaceRetry = 2 * time.Second
	namespaceBatch = 40
)

type NamespaceResult struct {
	Terminating int      `json:"terminating"`
	Deleted     int      `json:"deleted"`
	Remaining   int      `json:"remaining"`
	NextMs      int64    `json:"nextMs"`
	Names       []string `json:"names,omitempty"`
}

type NamespaceMessage struct {
	Kind  string   `json:"kind"`
	Key   string   `json:"key"`
	Names []string `json:"names"`
}

func NamespaceNames(msgs []NamespaceMessage) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, msg := range msgs {
		if msg.Kind == "change" && strings.HasPrefix(msg.Key, "/registry/namespaces/") {
			add(strings.TrimPrefix(msg.Key, "/registry/namespaces/"))
		}
		if msg.Kind == "retry" {
			for _, name := range msg.Names {
				add(name)
			}
		}
	}
	return names
}

type Deleter struct {
	inner deletion.NamespacedResourcesDeleterInterface
}

func NewDeleter(ctx context.Context, client kubernetes.Interface, meta metadata.Interface) *Deleter {
	d := &Deleter{}
	if client != nil && meta != nil {
		d.inner = deletion.NewNamespacedResourcesDeleter(
			ctx,
			client.CoreV1().Namespaces(),
			meta,
			client.CoreV1(),
			client.Discovery().ServerPreferredNamespacedResources,
			v1.FinalizerKubernetes,
		)
	}
	return d
}

func listTerminating(ctx context.Context, client kubernetes.Interface) ([]string, error) {
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	type item struct {
		name string
		at   time.Time
	}
	var items []item
	for i := range list.Items {
		if list.Items[i].DeletionTimestamp == nil {
			continue
		}
		items = append(items, item{name: list.Items[i].Name, at: list.Items[i].DeletionTimestamp.Time})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].at.After(items[j].at)
	})
	names := make([]string, len(items))
	for i := range items {
		names[i] = items[i].name
	}
	return names, nil
}

func pickTerminating(names []string, limit int) (selected []string, more bool) {
	if limit <= 0 || len(names) <= limit {
		return names, false
	}
	return names[:limit], true
}

func mergeTerminating(hinted, listed []string) []string {
	seen := map[string]bool{}
	var names []string
	for _, name := range hinted {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, name := range listed {
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func clearCore(ctx context.Context, client kubernetes.Interface, ns string) error {
	bg := metav1.DeletePropagationBackground
	zero := int64(0)
	opts := metav1.DeleteOptions{PropagationPolicy: &bg, GracePeriodSeconds: &zero}
	core := client.CoreV1()
	pods, err := core.Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if pods != nil && len(pods.Items) > 0 {
		for i := range pods.Items {
			if err := core.Pods(ns).Delete(ctx, pods.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		left, err := core.Pods(ns).List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if left != nil && len(left.Items) > 0 {
			return nil
		}
	}
	cms, err := core.ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if cms != nil {
		for i := range cms.Items {
			if err := core.ConfigMaps(ns).Delete(ctx, cms.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	sas, err := core.ServiceAccounts(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if sas != nil {
		for i := range sas.Items {
			if err := core.ServiceAccounts(ns).Delete(ctx, sas.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	secrets, err := core.Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if secrets != nil {
		for i := range secrets.Items {
			if err := core.Secrets(ns).Delete(ctx, secrets.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	svcs, err := core.Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if svcs != nil {
		for i := range svcs.Items {
			if err := core.Services(ns).Delete(ctx, svcs.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	eps, err := core.Endpoints(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if eps != nil {
		for i := range eps.Items {
			if err := core.Endpoints(ns).Delete(ctx, eps.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	templates, err := core.PodTemplates(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if templates != nil {
		for i := range templates.Items {
			if err := core.PodTemplates(ns).Delete(ctx, templates.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	rcs, err := core.ReplicationControllers(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if rcs != nil {
		for i := range rcs.Items {
			if err := core.ReplicationControllers(ns).Delete(ctx, rcs.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return clearNamespaced(ctx, client, ns, opts)
}

func clearNamespaced(ctx context.Context, client kubernetes.Interface, ns string, opts metav1.DeleteOptions) error {
	pdbs, err := client.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if pdbs != nil {
		for i := range pdbs.Items {
			if err := client.PolicyV1().PodDisruptionBudgets(ns).Delete(ctx, pdbs.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	apps := client.AppsV1()
	deploys, err := apps.Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if deploys != nil {
		for i := range deploys.Items {
			if err := apps.Deployments(ns).Delete(ctx, deploys.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	sets, err := apps.ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if sets != nil {
		for i := range sets.Items {
			if err := apps.ReplicaSets(ns).Delete(ctx, sets.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	stateful, err := apps.StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if stateful != nil {
		for i := range stateful.Items {
			if err := apps.StatefulSets(ns).Delete(ctx, stateful.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	daemons, err := apps.DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if daemons != nil {
		for i := range daemons.Items {
			if err := apps.DaemonSets(ns).Delete(ctx, daemons.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	batch := client.BatchV1()
	jobs, err := batch.Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if jobs != nil {
		for i := range jobs.Items {
			if err := batch.Jobs(ns).Delete(ctx, jobs.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	crons, err := batch.CronJobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if crons != nil {
		for i := range crons.Items {
			if err := batch.CronJobs(ns).Delete(ctx, crons.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	claims, err := client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if claims != nil {
		for i := range claims.Items {
			if err := client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, claims.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	slices, err := client.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if slices != nil {
		for i := range slices.Items {
			if err := client.DiscoveryV1().EndpointSlices(ns).Delete(ctx, slices.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	quotas, err := client.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if quotas != nil {
		for i := range quotas.Items {
			if err := client.CoreV1().ResourceQuotas(ns).Delete(ctx, quotas.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	limits, err := client.CoreV1().LimitRanges(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if limits != nil {
		for i := range limits.Items {
			if err := client.CoreV1().LimitRanges(ns).Delete(ctx, limits.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	roles, err := client.RbacV1().Roles(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if roles != nil {
		for i := range roles.Items {
			if err := client.RbacV1().Roles(ns).Delete(ctx, roles.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	bindings, err := client.RbacV1().RoleBindings(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if bindings != nil {
		for i := range bindings.Items {
			if err := client.RbacV1().RoleBindings(ns).Delete(ctx, bindings.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	revisions, err := client.AppsV1().ControllerRevisions(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if revisions != nil {
		for i := range revisions.Items {
			if err := client.AppsV1().ControllerRevisions(ns).Delete(ctx, revisions.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	leases, err := client.CoordinationV1().Leases(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if leases != nil {
		for i := range leases.Items {
			if err := client.CoordinationV1().Leases(ns).Delete(ctx, leases.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	if err := client.EventsV1().Events(ns).DeleteCollection(ctx, opts, metav1.ListOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := client.CoreV1().Events(ns).DeleteCollection(ctx, opts, metav1.ListOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	hpas, err := client.AutoscalingV1().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if hpas != nil {
		for i := range hpas.Items {
			if err := client.AutoscalingV1().HorizontalPodAutoscalers(ns).Delete(ctx, hpas.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	resourceClaims, err := client.ResourceV1().ResourceClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if resourceClaims != nil {
		for i := range resourceClaims.Items {
			if err := client.ResourceV1().ResourceClaims(ns).Delete(ctx, resourceClaims.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	claimTemplates, err := client.ResourceV1().ResourceClaimTemplates(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if claimTemplates != nil {
		for i := range claimTemplates.Items {
			if err := client.ResourceV1().ResourceClaimTemplates(ns).Delete(ctx, claimTemplates.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	policies, err := client.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if policies != nil {
		for i := range policies.Items {
			if err := client.NetworkingV1().NetworkPolicies(ns).Delete(ctx, policies.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	ingresses, err := client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if ingresses != nil {
		for i := range ingresses.Items {
			if err := client.NetworkingV1().Ingresses(ns).Delete(ctx, ingresses.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	caps, err := client.StorageV1().CSIStorageCapacities(ns).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if caps != nil {
		for i := range caps.Items {
			if err := client.StorageV1().CSIStorageCapacities(ns).Delete(ctx, caps.Items[i].Name, opts); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func namespaceContentGone(ctx context.Context, client kubernetes.Interface, ns string) (bool, error) {
	pods, err := namespacePodsGone(ctx, client, ns)
	if err != nil || !pods {
		return pods, err
	}
	checks := []func() (int, error){
		func() (int, error) {
			list, err := client.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().ServiceAccounts(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().Endpoints(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().PodTemplates(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().ReplicationControllers(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().LimitRanges(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.RbacV1().Roles(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.RbacV1().RoleBindings(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.AppsV1().ControllerRevisions(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoordinationV1().Leases(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.EventsV1().Events(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.AutoscalingV1().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.ResourceV1().ResourceClaims(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.ResourceV1().ResourceClaimTemplates(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
		func() (int, error) {
			list, err := client.StorageV1().CSIStorageCapacities(ns).List(ctx, metav1.ListOptions{Limit: 1})
			if list == nil {
				return 0, err
			}
			return len(list.Items), err
		},
	}
	for _, check := range checks {
		n, err := check()
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}
		if n > 0 {
			return false, nil
		}
	}
	return true, nil
}

var namespaceDeletionConditions = []v1.NamespaceCondition{
	{Type: v1.NamespaceDeletionDiscoveryFailure, Reason: "ResourcesDiscovered", Message: "All resources successfully discovered"},
	{Type: v1.NamespaceDeletionGVParsingFailure, Reason: "ParsedGroupVersions", Message: "All legacy kube types successfully parsed"},
	{Type: v1.NamespaceDeletionContentFailure, Reason: "ContentDeleted", Message: "All content successfully deleted, may be waiting on finalization"},
	{Type: v1.NamespaceContentRemaining, Reason: "ContentRemoved", Message: "All content successfully removed"},
	{Type: v1.NamespaceFinalizersRemaining, Reason: "ContentHasNoFinalizers", Message: "All content-preserving finalizers finished"},
}

func reportDeletionConditions(ctx context.Context, client kubernetes.Interface, ns *v1.Namespace) error {
	fresh := ns.DeepCopy()
	changed := false
	for _, want := range namespaceDeletionConditions {
		present := false
		for _, have := range fresh.Status.Conditions {
			if have.Type == want.Type {
				present = true
			}
		}
		if present {
			continue
		}
		want.Status = v1.ConditionFalse
		want.LastTransitionTime = metav1.Now()
		fresh.Status.Conditions = append(fresh.Status.Conditions, want)
		changed = true
	}
	if !changed {
		return nil
	}
	_, err := client.CoreV1().Namespaces().UpdateStatus(ctx, fresh, metav1.UpdateOptions{})
	return err
}

func namespacePodsGone(ctx context.Context, client kubernetes.Interface, ns string) (bool, error) {
	pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return pods == nil || len(pods.Items) == 0, nil
}

func finalizeKubernetes(ctx context.Context, client kubernetes.Interface, ns *v1.Namespace) error {
	fresh := ns.DeepCopy()
	kept := make([]v1.FinalizerName, 0, len(fresh.Spec.Finalizers))
	for _, f := range fresh.Spec.Finalizers {
		if f != v1.FinalizerKubernetes {
			kept = append(kept, f)
		}
	}
	fresh.Spec.Finalizers = kept
	_, err := client.CoreV1().Namespaces().Finalize(ctx, fresh, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (d *Deleter) DeleteTerminating(ctx context.Context, client kubernetes.Interface, names []string) (*NamespaceResult, error) {
	listed, err := listTerminating(ctx, client)
	if err != nil {
		return nil, err
	}
	hinted := mergeTerminating(names, nil)
	var selected []string
	var more bool
	if len(hinted) > 0 {
		selected, more = pickTerminating(hinted, namespaceBatch)
		if len(listed) > len(selected) {
			more = true
		}
	} else {
		selected, more = pickTerminating(mergeTerminating(nil, listed), namespaceBatch)
	}
	result := &NamespaceResult{}
	done := map[string]bool{}
	for _, name := range selected {
		ns, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if ns.DeletionTimestamp == nil {
			continue
		}
		result.Terminating++
		if err := clearCore(ctx, client, ns.Name); err != nil {
			println("namespaces: clear", ns.Name, "failed:", err.Error())
			result.Remaining++
			result.NextMs = soonest(result.NextMs, namespaceRetry)
			continue
		}
		if podsGone, _ := namespacePodsGone(ctx, client, ns.Name); !podsGone {
			if err := reportDeletionConditions(ctx, client, ns); err != nil {
				println("namespaces: conditions", ns.Name, "failed:", err.Error())
			}
		}
		if gone, _ := namespaceContentGone(ctx, client, ns.Name); !gone {
			result.Remaining++
			result.NextMs = soonest(result.NextMs, namespaceRetry)
			continue
		}
		if d.inner != nil {
			if err := d.inner.Delete(ctx, ns.Name); err != nil {
				var remain *deletion.ResourcesRemainingError
				if errors.As(err, &remain) {
					result.Remaining++
					wait := time.Duration(remain.Estimate) * time.Second
					if wait <= 0 {
						wait = namespaceRetry
					}
					result.NextMs = soonest(result.NextMs, wait)
					continue
				}
				println("namespaces: delete", ns.Name, "failed:", err.Error())
				result.Remaining++
				result.NextMs = soonest(result.NextMs, namespaceRetry)
				continue
			}
			result.Deleted++
			done[ns.Name] = true
			continue
		}
		if err := finalizeKubernetes(ctx, client, ns); err != nil {
			println("namespaces: finalize", ns.Name, "failed:", err.Error())
			result.Remaining++
			result.NextMs = soonest(result.NextMs, namespaceRetry)
			continue
		}
		result.Deleted++
		done[ns.Name] = true
	}
	if more || result.Remaining > 0 {
		result.NextMs = soonest(result.NextMs, namespaceRetry)
		for _, name := range listed {
			if !done[name] {
				result.Names = append(result.Names, name)
			}
		}
	}
	return result, nil
}
