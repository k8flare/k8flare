// Package storage carries the persistent-volume controllers, which pull pkg/volume/csi.
package storage

import (
	"context"
	"github.com/k8flare/k8flare/packages/workloads"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/volume/ephemeral"
	"k8s.io/kubernetes/pkg/controller/volume/pvcprotection"
	"k8s.io/kubernetes/pkg/controller/volume/pvprotection"
)

func init() { workloads.Register(build) }

func build(ctx context.Context, d workloads.Deps, controllers map[string]bool) ([]func(context.Context), error) {
	if !(controllers["pvcprotection"] || controllers["pvprotection"] || controllers["ephemeralvolume"]) {
		return nil, nil
	}
	client := d.Client
	core := d.Core()
	runs := []func(context.Context){}
	if controllers["pvcprotection"] {
		pvcProt, err := pvcprotection.NewPVCProtectionController(klog.FromContext(ctx), core.PersistentVolumeClaims(), core.Pods(), client)
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { pvcProt.Run(ctx, 1) })
	}
	if controllers["pvprotection"] {
		pvProt := pvprotection.NewPVProtectionController(klog.FromContext(ctx), core.PersistentVolumes(), client)
		runs = append(runs, func(ctx context.Context) { pvProt.Run(ctx, 1) })
	}
	if controllers["ephemeralvolume"] {
		eph, err := ephemeral.NewController(ctx, client, core.Pods(), core.PersistentVolumeClaims())
		if err != nil {
			return nil, err
		}
		runs = append(runs, func(ctx context.Context) { eph.Run(ctx, 1) })
	}

	return runs, nil
}
