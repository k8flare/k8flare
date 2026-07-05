//go:build js

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// GOOS=js overlay (k8flare, see third_party/k8s-js-overlays/README.md):
// identical to upstream registry.go EXCEPT the DynamicResources plugin is
// omitted. Rationale is the Worker Loader's hard 64MiB cap on the
// scheduler WASM, not taste: the DRA plugin's registry entry links
// k8s.io/dynamic-resource-allocation's structured-parameters machinery,
// which pushed the wasm-opt'd scheduler binary ~350KiB past the cap
// (67,464,036 vs 67,108,864 bytes; without it: 66,394,711 — measured, not
// estimated, 2026-07-05). DRA is unusable against this repo's apiserver
// anyway — ResourceClaim/ResourceSlice/DeviceClass are registered as
// permanently-empty stub types purely so informers can sync (see
// README.md's resource table). pkg/controllers.RunScheduler must also
// disable the plugin in its profile config, since the default profile
// still names it. The host build (cmd/scheduler, conformance CI) keeps
// the untouched upstream registry via scheduler-registry_notjs.go.
//
// NOTE: pkg/scheduler's core (scheduler.go, schedule_one.go,
// noderesources/fit.go) still imports plugins/dynamicresources directly,
// so the CEL evaluator etc. remain linked — this overlay only drops what
// the registry entry alone pulls in. Do not expect it to shrink the
// binary by more than the measured ~1MiB.

package plugins

import (
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultpreemption"
	plfeature "k8s.io/kubernetes/pkg/scheduler/framework/plugins/feature"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/gangscheduling"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/imagelocality"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/interpodaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodedeclaredfeatures"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodename"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeports"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/noderesources"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeunschedulable"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodevolumelimits"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/podgrouppodscount"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/podtopologyspread"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/schedulinggates"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/tainttoleration"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/topologyaware"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/volumebinding"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/volumerestrictions"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/volumezone"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

// NewInTreeRegistry builds the registry with all the in-tree plugins.
// A scheduler that runs out of tree plugins can register additional plugins
// through the WithFrameworkOutOfTreeRegistry option.
func NewInTreeRegistry() runtime.Registry {
	fts := plfeature.NewSchedulerFeaturesFromGates(feature.DefaultFeatureGate)
	registry := runtime.Registry{
		imagelocality.Name:                   imagelocality.New,
		tainttoleration.Name:                 runtime.FactoryAdapter(fts, tainttoleration.New),
		nodename.Name:                        runtime.FactoryAdapter(fts, nodename.New),
		nodeports.Name:                       runtime.FactoryAdapter(fts, nodeports.New),
		nodeaffinity.Name:                    runtime.FactoryAdapter(fts, nodeaffinity.New),
		nodedeclaredfeatures.Name:            runtime.FactoryAdapter(fts, nodedeclaredfeatures.New),
		podtopologyspread.Name:               runtime.FactoryAdapter(fts, podtopologyspread.New),
		nodeunschedulable.Name:               runtime.FactoryAdapter(fts, nodeunschedulable.New),
		noderesources.Name:                   runtime.FactoryAdapter(fts, noderesources.NewFit),
		noderesources.BalancedAllocationName: runtime.FactoryAdapter(fts, noderesources.NewBalancedAllocation),
		volumebinding.Name:                   runtime.FactoryAdapter(fts, volumebinding.New),
		volumerestrictions.Name:              runtime.FactoryAdapter(fts, volumerestrictions.New),
		volumezone.Name:                      runtime.FactoryAdapter(fts, volumezone.New),
		nodevolumelimits.CSIName:             runtime.FactoryAdapter(fts, nodevolumelimits.NewCSI),
		interpodaffinity.Name:                runtime.FactoryAdapter(fts, interpodaffinity.New),
		queuesort.Name:                       queuesort.New,
		defaultbinder.Name:                   defaultbinder.New,
		defaultpreemption.Name:               runtime.FactoryAdapter(fts, defaultpreemption.New),
		schedulinggates.Name:                 runtime.FactoryAdapter(fts, schedulinggates.New),
		gangscheduling.Name:                  runtime.FactoryAdapter(fts, gangscheduling.New),
		topologyaware.Name:                   runtime.FactoryAdapter(fts, topologyaware.New),
		podgrouppodscount.Name:               runtime.FactoryAdapter(fts, podgrouppodscount.New),
	}

	return registry
}
