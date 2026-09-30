package workloads

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1client "k8s.io/client-go/kubernetes/typed/discovery/v1"
	policyv1client "k8s.io/client-go/kubernetes/typed/policy/v1"
)

func observed[T runtime.Object](s *snapshotInformer, got T, err error) (T, error) {
	if err == nil {
		s.note(got)
	}
	return got, err
}

func removed(s *snapshotInformer, namespace, name string, err error) error {
	if err == nil {
		s.forget(namespace, name)
	}
	return err
}

func (a observeApps) ReplicaSets(namespace string) appsv1client.ReplicaSetInterface {
	return observeReplicaSets{ReplicaSetInterface: a.AppsV1Interface.ReplicaSets(namespace), namespace: namespace, s: a.c.replicaSets}
}

func (a observeApps) Deployments(namespace string) appsv1client.DeploymentInterface {
	return observeDeployments{DeploymentInterface: a.AppsV1Interface.Deployments(namespace), s: a.c.deployments}
}

func (a observeApps) DaemonSets(namespace string) appsv1client.DaemonSetInterface {
	return observeDaemonSets{DaemonSetInterface: a.AppsV1Interface.DaemonSets(namespace), namespace: namespace, s: a.c.daemonSets}
}

func (a observeApps) ControllerRevisions(namespace string) appsv1client.ControllerRevisionInterface {
	return observeRevisions{ControllerRevisionInterface: a.AppsV1Interface.ControllerRevisions(namespace), namespace: namespace, s: a.c.revisions}
}

type observeReplicaSets struct {
	appsv1client.ReplicaSetInterface
	namespace string
	s         *snapshotInformer
}

func (r observeReplicaSets) Create(ctx context.Context, rs *appsv1.ReplicaSet, opts metav1.CreateOptions) (*appsv1.ReplicaSet, error) {
	got, err := r.ReplicaSetInterface.Create(ctx, rs, opts)
	return observed(r.s, got, err)
}

func (r observeReplicaSets) Update(ctx context.Context, rs *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	got, err := r.ReplicaSetInterface.Update(ctx, rs, opts)
	return observed(r.s, got, err)
}

func (r observeReplicaSets) UpdateStatus(ctx context.Context, rs *appsv1.ReplicaSet, opts metav1.UpdateOptions) (*appsv1.ReplicaSet, error) {
	got, err := r.ReplicaSetInterface.UpdateStatus(ctx, rs, opts)
	return observed(r.s, got, err)
}

func (r observeReplicaSets) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*appsv1.ReplicaSet, error) {
	got, err := r.ReplicaSetInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(r.s, got, err)
}

func (r observeReplicaSets) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return removed(r.s, r.namespace, name, r.ReplicaSetInterface.Delete(ctx, name, opts))
}

type observeDeployments struct {
	appsv1client.DeploymentInterface
	s *snapshotInformer
}

func (d observeDeployments) Update(ctx context.Context, dep *appsv1.Deployment, opts metav1.UpdateOptions) (*appsv1.Deployment, error) {
	got, err := d.DeploymentInterface.Update(ctx, dep, opts)
	return observed(d.s, got, err)
}

func (d observeDeployments) UpdateStatus(ctx context.Context, dep *appsv1.Deployment, opts metav1.UpdateOptions) (*appsv1.Deployment, error) {
	got, err := d.DeploymentInterface.UpdateStatus(ctx, dep, opts)
	return observed(d.s, got, err)
}

func (d observeDeployments) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*appsv1.Deployment, error) {
	got, err := d.DeploymentInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(d.s, got, err)
}

type observeDaemonSets struct {
	appsv1client.DaemonSetInterface
	namespace string
	s         *snapshotInformer
}

func (d observeDaemonSets) Create(ctx context.Context, ds *appsv1.DaemonSet, opts metav1.CreateOptions) (*appsv1.DaemonSet, error) {
	got, err := d.DaemonSetInterface.Create(ctx, ds, opts)
	return observed(d.s, got, err)
}

func (d observeDaemonSets) Update(ctx context.Context, ds *appsv1.DaemonSet, opts metav1.UpdateOptions) (*appsv1.DaemonSet, error) {
	got, err := d.DaemonSetInterface.Update(ctx, ds, opts)
	return observed(d.s, got, err)
}

func (d observeDaemonSets) UpdateStatus(ctx context.Context, ds *appsv1.DaemonSet, opts metav1.UpdateOptions) (*appsv1.DaemonSet, error) {
	got, err := d.DaemonSetInterface.UpdateStatus(ctx, ds, opts)
	return observed(d.s, got, err)
}

func (d observeDaemonSets) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*appsv1.DaemonSet, error) {
	got, err := d.DaemonSetInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(d.s, got, err)
}

func (d observeDaemonSets) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return removed(d.s, d.namespace, name, d.DaemonSetInterface.Delete(ctx, name, opts))
}

type observeRevisions struct {
	appsv1client.ControllerRevisionInterface
	namespace string
	s         *snapshotInformer
}

func (r observeRevisions) Create(ctx context.Context, rev *appsv1.ControllerRevision, opts metav1.CreateOptions) (*appsv1.ControllerRevision, error) {
	got, err := r.ControllerRevisionInterface.Create(ctx, rev, opts)
	return observed(r.s, got, err)
}

func (r observeRevisions) Update(ctx context.Context, rev *appsv1.ControllerRevision, opts metav1.UpdateOptions) (*appsv1.ControllerRevision, error) {
	got, err := r.ControllerRevisionInterface.Update(ctx, rev, opts)
	return observed(r.s, got, err)
}

func (r observeRevisions) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*appsv1.ControllerRevision, error) {
	got, err := r.ControllerRevisionInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(r.s, got, err)
}

func (r observeRevisions) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return removed(r.s, r.namespace, name, r.ControllerRevisionInterface.Delete(ctx, name, opts))
}

func (c *observeClient) BatchV1() batchv1client.BatchV1Interface {
	return observeBatch{BatchV1Interface: c.Interface.BatchV1(), c: c}
}

type observeBatch struct {
	batchv1client.BatchV1Interface
	c *observeClient
}

func (b observeBatch) Jobs(namespace string) batchv1client.JobInterface {
	return observeJobs{JobInterface: b.BatchV1Interface.Jobs(namespace), namespace: namespace, s: b.c.jobs}
}

func (b observeBatch) CronJobs(namespace string) batchv1client.CronJobInterface {
	return observeCronJobs{CronJobInterface: b.BatchV1Interface.CronJobs(namespace), s: b.c.cronJobs}
}

type observeJobs struct {
	batchv1client.JobInterface
	namespace string
	s         *snapshotInformer
}

func (j observeJobs) Create(ctx context.Context, job *batchv1.Job, opts metav1.CreateOptions) (*batchv1.Job, error) {
	got, err := j.JobInterface.Create(ctx, job, opts)
	return observed(j.s, got, err)
}

func (j observeJobs) Update(ctx context.Context, job *batchv1.Job, opts metav1.UpdateOptions) (*batchv1.Job, error) {
	got, err := j.JobInterface.Update(ctx, job, opts)
	return observed(j.s, got, err)
}

func (j observeJobs) UpdateStatus(ctx context.Context, job *batchv1.Job, opts metav1.UpdateOptions) (*batchv1.Job, error) {
	got, err := j.JobInterface.UpdateStatus(ctx, job, opts)
	return observed(j.s, got, err)
}

func (j observeJobs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*batchv1.Job, error) {
	got, err := j.JobInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(j.s, got, err)
}

func (j observeJobs) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return removed(j.s, j.namespace, name, j.JobInterface.Delete(ctx, name, opts))
}

type observeCronJobs struct {
	batchv1client.CronJobInterface
	s *snapshotInformer
}

func (c observeCronJobs) Update(ctx context.Context, cron *batchv1.CronJob, opts metav1.UpdateOptions) (*batchv1.CronJob, error) {
	got, err := c.CronJobInterface.Update(ctx, cron, opts)
	return observed(c.s, got, err)
}

func (c observeCronJobs) UpdateStatus(ctx context.Context, cron *batchv1.CronJob, opts metav1.UpdateOptions) (*batchv1.CronJob, error) {
	got, err := c.CronJobInterface.UpdateStatus(ctx, cron, opts)
	return observed(c.s, got, err)
}

func (c observeCronJobs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*batchv1.CronJob, error) {
	got, err := c.CronJobInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(c.s, got, err)
}

func (c *observeClient) DiscoveryV1() discoveryv1client.DiscoveryV1Interface {
	return observeDiscovery{DiscoveryV1Interface: c.Interface.DiscoveryV1(), s: c.slices}
}

type observeDiscovery struct {
	discoveryv1client.DiscoveryV1Interface
	s *snapshotInformer
}

func (d observeDiscovery) EndpointSlices(namespace string) discoveryv1client.EndpointSliceInterface {
	return observeSlices{EndpointSliceInterface: d.DiscoveryV1Interface.EndpointSlices(namespace), namespace: namespace, s: d.s}
}

type observeSlices struct {
	discoveryv1client.EndpointSliceInterface
	namespace string
	s         *snapshotInformer
}

func (e observeSlices) Create(ctx context.Context, slice *discoveryv1.EndpointSlice, opts metav1.CreateOptions) (*discoveryv1.EndpointSlice, error) {
	got, err := e.EndpointSliceInterface.Create(ctx, slice, opts)
	return observed(e.s, got, err)
}

func (e observeSlices) Update(ctx context.Context, slice *discoveryv1.EndpointSlice, opts metav1.UpdateOptions) (*discoveryv1.EndpointSlice, error) {
	got, err := e.EndpointSliceInterface.Update(ctx, slice, opts)
	return observed(e.s, got, err)
}

func (e observeSlices) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*discoveryv1.EndpointSlice, error) {
	got, err := e.EndpointSliceInterface.Patch(ctx, name, pt, data, opts, subresources...)
	return observed(e.s, got, err)
}

func (e observeSlices) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return removed(e.s, e.namespace, name, e.EndpointSliceInterface.Delete(ctx, name, opts))
}

func (c *observeClient) PolicyV1() policyv1client.PolicyV1Interface {
	return observePolicy{PolicyV1Interface: c.Interface.PolicyV1(), s: c.budgets}
}

type observePolicy struct {
	policyv1client.PolicyV1Interface
	s *snapshotInformer
}

func (p observePolicy) PodDisruptionBudgets(namespace string) policyv1client.PodDisruptionBudgetInterface {
	return observeBudgets{PodDisruptionBudgetInterface: p.PolicyV1Interface.PodDisruptionBudgets(namespace), s: p.s}
}

type observeBudgets struct {
	policyv1client.PodDisruptionBudgetInterface
	s *snapshotInformer
}

func (b observeBudgets) UpdateStatus(ctx context.Context, pdb *policyv1.PodDisruptionBudget, opts metav1.UpdateOptions) (*policyv1.PodDisruptionBudget, error) {
	got, err := b.PodDisruptionBudgetInterface.UpdateStatus(ctx, pdb, opts)
	return observed(b.s, got, err)
}

func (c observeCore) Endpoints(namespace string) corev1client.EndpointsInterface {
	return observeEndpoints{EndpointsInterface: c.CoreV1Interface.Endpoints(namespace), namespace: namespace, s: c.c.endpoints}
}

func (c observeCore) ResourceQuotas(namespace string) corev1client.ResourceQuotaInterface {
	return observeQuotas{ResourceQuotaInterface: c.CoreV1Interface.ResourceQuotas(namespace), s: c.c.quotas}
}

func (c observeCore) Namespaces() corev1client.NamespaceInterface {
	return observeNamespaces{NamespaceInterface: c.CoreV1Interface.Namespaces(), s: c.c.namespaces}
}

type observeEndpoints struct {
	corev1client.EndpointsInterface
	namespace string
	s         *snapshotInformer
}

func (e observeEndpoints) Create(ctx context.Context, eps *corev1.Endpoints, opts metav1.CreateOptions) (*corev1.Endpoints, error) {
	got, err := e.EndpointsInterface.Create(ctx, eps, opts)
	return observed(e.s, got, err)
}

func (e observeEndpoints) Update(ctx context.Context, eps *corev1.Endpoints, opts metav1.UpdateOptions) (*corev1.Endpoints, error) {
	got, err := e.EndpointsInterface.Update(ctx, eps, opts)
	return observed(e.s, got, err)
}

func (e observeEndpoints) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return removed(e.s, e.namespace, name, e.EndpointsInterface.Delete(ctx, name, opts))
}

type observeQuotas struct {
	corev1client.ResourceQuotaInterface
	s *snapshotInformer
}

func (q observeQuotas) Update(ctx context.Context, quota *corev1.ResourceQuota, opts metav1.UpdateOptions) (*corev1.ResourceQuota, error) {
	got, err := q.ResourceQuotaInterface.Update(ctx, quota, opts)
	return observed(q.s, got, err)
}

func (q observeQuotas) UpdateStatus(ctx context.Context, quota *corev1.ResourceQuota, opts metav1.UpdateOptions) (*corev1.ResourceQuota, error) {
	got, err := q.ResourceQuotaInterface.UpdateStatus(ctx, quota, opts)
	return observed(q.s, got, err)
}

type observeNamespaces struct {
	corev1client.NamespaceInterface
	s *snapshotInformer
}

func (n observeNamespaces) Update(ctx context.Context, ns *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	got, err := n.NamespaceInterface.Update(ctx, ns, opts)
	return observed(n.s, got, err)
}

func (n observeNamespaces) UpdateStatus(ctx context.Context, ns *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	got, err := n.NamespaceInterface.UpdateStatus(ctx, ns, opts)
	return observed(n.s, got, err)
}

func (n observeNamespaces) Finalize(ctx context.Context, ns *corev1.Namespace, opts metav1.UpdateOptions) (*corev1.Namespace, error) {
	got, err := n.NamespaceInterface.Finalize(ctx, ns, opts)
	return observed(n.s, got, err)
}
