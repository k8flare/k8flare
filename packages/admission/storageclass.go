package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	_ "k8s.io/kubernetes/pkg/apis/storage/install"
	"k8s.io/kubernetes/plugin/pkg/admission/storage/storageclass/setdefault"
)

func applyDefaultStorageClass(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPlugin(setdefault.Register, setdefault.PluginName, f)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
