// Package pv carries the persistent-volume binder. It is a shard of its own
// because it is the only controller that links pkg/volume/csi and the volume
// plugin machinery, which is 30 MB of a 64 MiB worker.
package pv

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"
	"time"

	"k8s.io/kubernetes/pkg/controller/volume/persistentvolume"
)

func init() { workloads.Register(build) }

func build(ctx context.Context, d workloads.Deps, controllers map[string]bool) ([]func(context.Context), error) {
	if !controllers["persistentvolume"] {
		return nil, nil
	}
	client, factory := d.Client, d.Factory
	core := d.Core()
	runs := []func(context.Context){}
	if controllers["persistentvolume"] {
		pvb, err := persistentvolume.NewController(ctx, persistentvolume.ControllerParameters{
			KubeClient:                client,
			SyncPeriod:                15 * time.Minute,
			VolumeInformer:            core.PersistentVolumes(),
			ClaimInformer:             core.PersistentVolumeClaims(),
			ClassInformer:             factory.Storage().V1().StorageClasses(),
			PodInformer:               core.Pods(),
			NodeInformer:              core.Nodes(),
			EnableDynamicProvisioning: false,
		})
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { pvb.Run(ctx) })
	}

	return runs, nil
}
