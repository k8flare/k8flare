package workloads

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const seenPatch = `{"metadata":{"labels":{"seen":"1"}}}`

func seen[T metav1.Object](obj T) T {
	obj.SetLabels(map[string]string{"seen": "1"})
	return obj
}

func named() metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: "x", Namespace: "default", ResourceVersion: "1"}
}

func fresh() metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: "x", Namespace: "default"}
}

type writeCase struct {
	name    string
	seed    runtime.Object
	write   func(context.Context, kubernetes.Interface) error
	present bool
}

func writeCases() []writeCase {
	opts, create, patch := metav1.UpdateOptions{}, metav1.CreateOptions{}, metav1.PatchOptions{}
	job := &batchv1.Job{ObjectMeta: named()}
	cron := &batchv1.CronJob{ObjectMeta: named()}
	rs := &appsv1.ReplicaSet{ObjectMeta: named()}
	dep := &appsv1.Deployment{ObjectMeta: named()}
	ds := &appsv1.DaemonSet{ObjectMeta: named()}
	sts := &appsv1.StatefulSet{ObjectMeta: named()}
	rev := &appsv1.ControllerRevision{ObjectMeta: named()}
	eps := &corev1.Endpoints{ObjectMeta: named()}
	slice := &discoveryv1.EndpointSlice{ObjectMeta: named()}
	quota := &corev1.ResourceQuota{ObjectMeta: named()}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x", ResourceVersion: "1"}}
	pdb := &policyv1.PodDisruptionBudget{ObjectMeta: named()}
	pod := &corev1.Pod{ObjectMeta: named()}
	return []writeCase{
		{name: "job update", seed: job, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().Jobs("default").Update(ctx, seen(job.DeepCopy()), opts)
			return err
		}},
		{name: "job status", seed: job, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().Jobs("default").UpdateStatus(ctx, seen(job.DeepCopy()), opts)
			return err
		}},
		{name: "job patch", seed: job, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().Jobs("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "job create", seed: &batchv1.Job{}, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().Jobs("default").Create(ctx, seen(&batchv1.Job{ObjectMeta: fresh()}), create)
			return err
		}},
		{name: "job delete", seed: job, write: func(ctx context.Context, c kubernetes.Interface) error {
			return c.BatchV1().Jobs("default").Delete(ctx, "x", metav1.DeleteOptions{})
		}},
		{name: "cronjob update", seed: cron, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().CronJobs("default").Update(ctx, seen(cron.DeepCopy()), opts)
			return err
		}},
		{name: "cronjob status", seed: cron, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().CronJobs("default").UpdateStatus(ctx, seen(cron.DeepCopy()), opts)
			return err
		}},
		{name: "cronjob patch", seed: cron, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.BatchV1().CronJobs("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "replicaset update", seed: rs, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ReplicaSets("default").Update(ctx, seen(rs.DeepCopy()), opts)
			return err
		}},
		{name: "replicaset status", seed: rs, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ReplicaSets("default").UpdateStatus(ctx, seen(rs.DeepCopy()), opts)
			return err
		}},
		{name: "replicaset patch", seed: rs, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ReplicaSets("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "replicaset create", seed: &appsv1.ReplicaSet{}, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ReplicaSets("default").Create(ctx, seen(&appsv1.ReplicaSet{ObjectMeta: fresh()}), create)
			return err
		}},
		{name: "replicaset delete", seed: rs, write: func(ctx context.Context, c kubernetes.Interface) error {
			return c.AppsV1().ReplicaSets("default").Delete(ctx, "x", metav1.DeleteOptions{})
		}},
		{name: "deployment update", seed: dep, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().Deployments("default").Update(ctx, seen(dep.DeepCopy()), opts)
			return err
		}},
		{name: "deployment status", seed: dep, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().Deployments("default").UpdateStatus(ctx, seen(dep.DeepCopy()), opts)
			return err
		}},
		{name: "deployment patch", seed: dep, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().Deployments("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "daemonset update", seed: ds, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().DaemonSets("default").Update(ctx, seen(ds.DeepCopy()), opts)
			return err
		}},
		{name: "daemonset status", seed: ds, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().DaemonSets("default").UpdateStatus(ctx, seen(ds.DeepCopy()), opts)
			return err
		}},
		{name: "daemonset patch", seed: ds, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().DaemonSets("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "daemonset create", seed: &appsv1.DaemonSet{}, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().DaemonSets("default").Create(ctx, seen(&appsv1.DaemonSet{ObjectMeta: fresh()}), create)
			return err
		}},
		{name: "daemonset delete", seed: ds, write: func(ctx context.Context, c kubernetes.Interface) error {
			return c.AppsV1().DaemonSets("default").Delete(ctx, "x", metav1.DeleteOptions{})
		}},
		{name: "statefulset patch", seed: sts, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().StatefulSets("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "controllerrevision update", seed: rev, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ControllerRevisions("default").Update(ctx, seen(rev.DeepCopy()), opts)
			return err
		}},
		{name: "controllerrevision patch", seed: rev, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ControllerRevisions("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "controllerrevision create", seed: &appsv1.ControllerRevision{}, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.AppsV1().ControllerRevisions("default").Create(ctx, seen(&appsv1.ControllerRevision{ObjectMeta: fresh()}), create)
			return err
		}},
		{name: "controllerrevision delete", seed: rev, write: func(ctx context.Context, c kubernetes.Interface) error {
			return c.AppsV1().ControllerRevisions("default").Delete(ctx, "x", metav1.DeleteOptions{})
		}},
		{name: "endpoints update", seed: eps, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().Endpoints("default").Update(ctx, seen(eps.DeepCopy()), opts)
			return err
		}},
		{name: "endpoints create", seed: &corev1.Endpoints{}, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().Endpoints("default").Create(ctx, seen(&corev1.Endpoints{ObjectMeta: fresh()}), create)
			return err
		}},
		{name: "endpoints delete", seed: eps, write: func(ctx context.Context, c kubernetes.Interface) error {
			return c.CoreV1().Endpoints("default").Delete(ctx, "x", metav1.DeleteOptions{})
		}},
		{name: "endpointslice update", seed: slice, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.DiscoveryV1().EndpointSlices("default").Update(ctx, seen(slice.DeepCopy()), opts)
			return err
		}},
		{name: "endpointslice patch", seed: slice, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.DiscoveryV1().EndpointSlices("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
		{name: "endpointslice create", seed: &discoveryv1.EndpointSlice{}, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.DiscoveryV1().EndpointSlices("default").Create(ctx, seen(&discoveryv1.EndpointSlice{ObjectMeta: fresh()}), create)
			return err
		}},
		{name: "endpointslice delete", seed: slice, write: func(ctx context.Context, c kubernetes.Interface) error {
			return c.DiscoveryV1().EndpointSlices("default").Delete(ctx, "x", metav1.DeleteOptions{})
		}},
		{name: "resourcequota status", seed: quota, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().ResourceQuotas("default").UpdateStatus(ctx, seen(quota.DeepCopy()), opts)
			return err
		}},
		{name: "resourcequota update", seed: quota, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().ResourceQuotas("default").Update(ctx, seen(quota.DeepCopy()), opts)
			return err
		}},
		{name: "namespace status", seed: ns, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().Namespaces().UpdateStatus(ctx, seen(ns.DeepCopy()), opts)
			return err
		}},
		{name: "namespace finalize", seed: ns, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().Namespaces().Finalize(ctx, seen(ns.DeepCopy()), opts)
			return err
		}},
		{name: "namespace update", seed: ns, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().Namespaces().Update(ctx, seen(ns.DeepCopy()), opts)
			return err
		}},
		{name: "poddisruptionbudget status", seed: pdb, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.PolicyV1().PodDisruptionBudgets("default").UpdateStatus(ctx, seen(pdb.DeepCopy()), opts)
			return err
		}},
		{name: "pod patch", seed: pod, present: true, write: func(ctx context.Context, c kubernetes.Interface) error {
			_, err := c.CoreV1().Pods("default").Patch(ctx, "x", types.MergePatchType, []byte(seenPatch), patch)
			return err
		}},
	}
}

func TestControllerWritesEnterTheSnapshotWithinThePass(t *testing.T) {
	for _, tc := range writeCases() {
		t.Run(tc.name, func(t *testing.T) {
			informer := newSnapshotInformer(tc.seed.DeepCopyObject())
			var backing []runtime.Object
			if tc.seed.(metav1.Object).GetName() != "" {
				if err := informer.GetIndexer().Add(tc.seed.DeepCopyObject()); err != nil {
					t.Fatal(err)
				}
				backing = append(backing, tc.seed.DeepCopyObject())
			}
			client := observeWrites(fake.NewSimpleClientset(backing...), []loadedSource{{informer: informer}})
			if err := tc.write(context.Background(), client); err != nil {
				t.Fatal(err)
			}
			stored, exists, err := informer.GetIndexer().GetByKey(snapshotKey(tc.seed))
			if err != nil {
				t.Fatal(err)
			}
			if exists != tc.present {
				t.Fatalf("exists=%v want %v", exists, tc.present)
			}
			if tc.present && stored.(metav1.Object).GetLabels()["seen"] != "1" {
				t.Fatalf("snapshot copy is stale: labels=%v", stored.(metav1.Object).GetLabels())
			}
		})
	}
}

func snapshotKey(seed runtime.Object) string {
	if _, ok := seed.(*corev1.Namespace); ok {
		return "x"
	}
	return "default/x"
}
