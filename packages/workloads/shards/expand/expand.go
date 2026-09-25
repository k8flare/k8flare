// Package expand carries the volume-expansion controller. It is a shard of its
// own because it is the one controller that links pkg/volume/csi and the CSI
// migration and translation libraries.
package expandshard

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"

	csitrans "k8s.io/csi-translation-lib"
	"k8s.io/kubernetes/pkg/controller/volume/expand"
	"k8s.io/kubernetes/pkg/volume/csi"
	"k8s.io/kubernetes/pkg/volume/csimigration"
)

func init() { workloads.Register(build) }

func build(ctx context.Context, d workloads.Deps, controllers map[string]bool) ([]func(context.Context), error) {
	if !controllers["volumeexpand"] {
		return nil, nil
	}
	client := d.Client
	core := d.Core()
	runs := []func(context.Context){}
	if controllers["volumeexpand"] {
		translator := csitrans.New()
		exp, err := expand.NewExpandController(ctx, client, core.PersistentVolumeClaims(), csi.ProbeVolumePlugins(), translator, csimigration.NewPluginManager(translator))
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { exp.Run(ctx) })
	}

	return runs, nil
}
