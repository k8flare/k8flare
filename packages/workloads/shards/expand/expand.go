// Package expand carries the volume-expansion controller. It is a shard of its
// own because it is the one controller that links pkg/volume/csi and the CSI
// migration and translation libraries.
package expandshard

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"

	csitrans "k8s.io/csi-translation-lib"
	"k8s.io/kubernetes/pkg/controller/volume/expand"
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
		// No in-tree plugin here: csi.ProbeVolumePlugins returns only csiPlugin,
		// which has no ExpandVolumeDevice, so FindExpandablePluginBySpec can never
		// return it and the controller always falls through to the
		// ExternalExpanding event. Passing it would link 30 MB of pkg/volume/csi
		// that the controller cannot reach. The migration path below is unaffected.
		exp, err := expand.NewExpandController(ctx, client, core.PersistentVolumeClaims(), nil, translator, csimigration.NewPluginManager(translator))
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { exp.Run(ctx) })
	}

	return runs, nil
}
