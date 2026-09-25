//go:build !js

package apiserver_test

import (
	"testing"
	"time"

	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// TestInformerWatchList drives client-go's watch-list path (what the
// kubelet uses) against an empty and a non-empty collection: the informer
// must sync, which needs the initial-events-end bookmark, and must see a
// later create.
func TestInformerWatchList(t *testing.T) {
	t.Setenv("KUBE_FEATURE_WatchListClient", "true")
	base, protobufClient := startDevURL(t)
	jsonClient, err := kubernetes.NewForConfig(&rest.Config{Host: base, BearerToken: devToken, ContentConfig: rest.ContentConfig{ContentType: "application/json", AcceptContentTypes: "application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	// The control plane serves JSON only, so the client that asks for protobuf
	// has to negotiate down rather than fail: that case pins the fallback.
	for name, cs := range map[string]*kubernetes.Clientset{"negotiates-down": protobufClient, "json": jsonClient} {
		t.Run(name, func(t *testing.T) { informerWatchList(t, cs, name) })
	}
}

// Go clients ask for gzip, and the runtime would compress a JSON watch
// stream and hold it back until it closes; the json case pins that a
// streamed response is not compressed.
func informerWatchList(t *testing.T, cs *kubernetes.Clientset, suffix string) {
	c := ctx(t)
	factory := informers.NewSharedInformerFactory(cs, 0)
	rcInformer := factory.Node().V1().RuntimeClasses().Informer()
	nsInformer := factory.Core().V1().Namespaces().Informer()
	added := make(chan string, 8)
	rcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{AddFunc: func(obj any) {
		added <- obj.(*nodev1.RuntimeClass).Name
	}})
	factory.Start(c.Done())
	synced := factory.WaitForCacheSync(c.Done())
	for typ, ok := range synced {
		if !ok {
			t.Fatalf("informer for %v did not sync", typ)
		}
	}
	if len(nsInformer.GetStore().List()) < 4 {
		t.Fatalf("namespace informer synced with %d items", len(nsInformer.GetStore().List()))
	}
	if _, err := cs.NodeV1().RuntimeClasses().Create(c, &nodev1.RuntimeClass{ObjectMeta: metav1.ObjectMeta{Name: "rc-" + suffix}, Handler: "runc"}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(20 * time.Second)
	for {
		select {
		case name := <-added:
			if name == "rc-"+suffix {
				return
			}
		case <-deadline:
			t.Fatal("informer did not see the created RuntimeClass")
		}
	}
}
