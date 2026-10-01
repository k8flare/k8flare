package workloads

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestListSourcesKeepsNoMoreListsOpenThanThePlatformAllows(t *testing.T) {
	var open, most atomic.Int64
	page := func(ctx context.Context, _ metav1.ListOptions) (runtime.Object, error) {
		now := open.Add(1)
		for {
			seen := most.Load()
			if now <= seen || most.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		open.Add(-1)
		return &v1.ConfigMapList{ListMeta: metav1.ListMeta{ResourceVersion: "7"}, Items: []v1.ConfigMap{{}}}, nil
	}
	src := make([]source, 36)
	for i := range src {
		src[i] = source{name: "configmaps", example: &v1.ConfigMap{}, page: page}
	}
	loaded, revisions, err := listSources(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if got := most.Load(); got != listConcurrency {
		t.Fatalf("most lists open at once = %d, want %d", got, listConcurrency)
	}
	for i := range src {
		if len(loaded[i]) != 1 || revisions[i] != 7 {
			t.Fatalf("source %d: %d objects at revision %d", i, len(loaded[i]), revisions[i])
		}
	}
}
