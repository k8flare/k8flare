package workloads

import (
	"context"
	"errors"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/kubernetes/pkg/controller/namespace/deletion"
)

const namespaceRetry = 10 * time.Second

type NamespaceResult struct {
	Terminating int   `json:"terminating"`
	Deleted     int   `json:"deleted"`
	Remaining   int   `json:"remaining"`
	NextMs      int64 `json:"nextMs"`
}

type Deleter struct {
	deleter deletion.NamespacedResourcesDeleterInterface
}

func NewDeleter(ctx context.Context, client kubernetes.Interface, metadataClient metadata.Interface) *Deleter {
	return &Deleter{deleter: deletion.NewNamespacedResourcesDeleter(ctx, client.CoreV1().Namespaces(), metadataClient, client.CoreV1(),
		client.Discovery().ServerPreferredNamespacedResources, v1.FinalizerKubernetes)}
}

func (d *Deleter) DeleteTerminating(ctx context.Context, client kubernetes.Interface, names []string) (*NamespaceResult, error) {
	result := &NamespaceResult{}
	for _, name := range names {
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
		err = d.deleter.Delete(ctx, ns.Name)
		var remaining *deletion.ResourcesRemainingError
		switch {
		case err == nil:
			result.Deleted++
		case errors.As(err, &remaining):
			result.Remaining++
			result.NextMs = soonest(result.NextMs, time.Duration(remaining.Estimate)*time.Second)
		default:
			println("namespaces: delete", ns.Name, "failed:", err.Error())
			result.Remaining++
			result.NextMs = soonest(result.NextMs, namespaceRetry)
		}
	}
	return result, nil
}
