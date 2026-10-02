package admission

import (
	"context"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	_ "k8s.io/kubernetes/pkg/apis/storage/install"
	"k8s.io/kubernetes/plugin/pkg/admission/storage/persistentvolume/resize"
)

func applyPersistentVolumeClaimResize(ctx context.Context, s *store, req *admit.Request) error {
	f := newStoreInformerFactory(ctx, s)
	p, err := initUpstreamPlugin(resize.Register, resize.PluginName, f)
	if err != nil {
		return err
	}
	return runWithFactory(ctx, p, f, req)
}
