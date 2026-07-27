//go:build js && wasm

package clusterop

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerywatch "k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"

	k8flarev1alpha1 "github.com/k8flare/k8flare/pkg/apis/k8flare/v1alpha1"
	"github.com/k8flare/k8flare/pkg/leanclient"
)

// clustersClient is a typed client for k8flare.com/v1alpha1 Clusters,
// written by hand on top of pkg/leanclient's verb generics rather than
// emitted by cmd/k8flare-gen's leanclient step. That generator exists for
// one specific reason: to satisfy client-go's generated per-type
// interfaces (corev1.PodInterface and friends) so real upstream
// controllers accept this repo's clients. k8flare.com is this project's
// own group -- upstream has no interface to satisfy, and nothing but the
// cluster operator ever calls it -- so generating Apply/ApplyStatus
// methods and an applyconfigurations dependency here would be linked
// weight for an interface no caller has. The verb generics themselves are
// still shared, so this bypass costs a few dozen lines, not a parallel
// client stack.
type clustersClient struct {
	rc restclient.Interface
}

var (
	clusterGV       = k8flarev1alpha1.SchemeGroupVersion
	clusterGVK      = clusterGV.WithKind("Cluster")
	clustersResName = "clusters"
)

func newClustersClient(cfg *restclient.Config) (*clustersClient, error) {
	rc, err := leanclient.RESTClientFor(cfg, "/apis", clusterGV)
	if err != nil {
		return nil, err
	}
	return &clustersClient{rc: rc}, nil
}

// Clusters are cluster-scoped, so every call below passes an empty
// namespace (pkg/leanclient's verbs omit the namespace segment then).

func (c *clustersClient) Get(ctx context.Context, name string) (*k8flarev1alpha1.Cluster, error) {
	return leanclient.Get[k8flarev1alpha1.Cluster](ctx, c.rc, clustersResName, "", name, metav1.GetOptions{})
}

func (c *clustersClient) List(ctx context.Context, opts metav1.ListOptions) (*k8flarev1alpha1.ClusterList, error) {
	return leanclient.List[k8flarev1alpha1.ClusterList](ctx, c.rc, clustersResName, "", opts)
}

func (c *clustersClient) Watch(ctx context.Context, opts metav1.ListOptions) (apimachinerywatch.Interface, error) {
	return leanclient.Watch(ctx, c.rc, clustersResName, "", opts, func() *k8flarev1alpha1.Cluster {
		return &k8flarev1alpha1.Cluster{}
	})
}

func (c *clustersClient) Create(ctx context.Context, obj *k8flarev1alpha1.Cluster) (*k8flarev1alpha1.Cluster, error) {
	return leanclient.Create(ctx, c.rc, clustersResName, "", obj, clusterGVK, metav1.CreateOptions{})
}

func (c *clustersClient) Update(ctx context.Context, obj *k8flarev1alpha1.Cluster) (*k8flarev1alpha1.Cluster, error) {
	return leanclient.Update(ctx, c.rc, clustersResName, "", obj.Name, obj, clusterGVK, metav1.UpdateOptions{})
}

func (c *clustersClient) UpdateStatus(ctx context.Context, obj *k8flarev1alpha1.Cluster) (*k8flarev1alpha1.Cluster, error) {
	return leanclient.UpdateSubresource(ctx, c.rc, clustersResName, "", obj.Name, "status", obj, clusterGVK, metav1.UpdateOptions{})
}
