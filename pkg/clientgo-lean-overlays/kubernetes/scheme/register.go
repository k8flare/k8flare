/*
Copyright The Kubernetes Authors.

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

// k8flare lean overlay for kubernetes/scheme/register.go (see
// pkg/clientgo-lean-overlays/README.md). Upstream registers all
// ~55 API group-versions into this package-level Scheme at init() time --
// and because scheme registration happens via init side effects, Go's
// linker cannot dead-code-eliminate any of it: importing
// k8s.io/client-go/discovery (which every typed client reaches through
// applyconfigurations/meta/v1) linked ~21MiB of generated API-type code
// (DeepCopy/proto for every group) into the GOOS=js control-plane
// binaries, measured by wasm name-section attribution on 2026-07-05.
// This is the single biggest lever keeping the kube-controller-manager
// and kube-scheduler WASM binaries under the Worker Loader's hard 64MiB
// cap.
//
// This overlay keeps only the group-versions those two binaries actually
// serialize at runtime: the five leanclient groups (core/apps/batch/
// coordination/discovery v1), events/v1 (the scheduler's
// EventBroadcasterAdapter sink), policy/v1 (PDB informers), and
// storage/v1 (CSI informers), scheduling/v1 (PriorityClass). A type
// outside this list would fail encode/decode at runtime with a "no kind
// registered" error -- loud, not silent -- and fixing it means adding
// one import + one AddToScheme line here.
//
// resource/v1 (DeviceClass/ResourceClaim/ResourceSlice -- the real,
// unmodified upstream scheduler's DynamicResources/DRA machinery, GA and
// LockToDefault:true in v1.36.2-k3s1, see docs/platform-verification.md's
// S8 kube-scheduler-wasm-fork entry) is registered separately, in
// register_sched.go, gated by `!leanwidth`: KCM's `-tags leanwidth`
// build never touches DRA (pkg/leanclient's ResourceV1() is a permanent
// panic stub there), so registering it unconditionally here regressed
// KCM's shipped binary by ~1.18MiB for zero benefit (found during
// scheduler-narrow-fork-v3's review, before merge -- see that entry).
//
// Host builds (cmd/agent, cmd/scheduler, go test) never see this file:
// it is only swapped into .build/clientgo-lean-mirror by
// packages/wasm-build/src/gen-clientgo-lean-mirror.ts, which only
// go.wasm.mod's k8s.io/client-go replace points at.

package scheme

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	eventsv1 "k8s.io/api/events/v1"
	policyv1 "k8s.io/api/policy/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var Scheme = runtime.NewScheme()
var Codecs = serializer.NewCodecFactory(Scheme)
var ParameterCodec = runtime.NewParameterCodec(Scheme)
var localSchemeBuilder = runtime.SchemeBuilder{
	appsv1.AddToScheme,
	batchv1.AddToScheme,
	coordinationv1.AddToScheme,
	corev1.AddToScheme,
	discoveryv1.AddToScheme,
	eventsv1.AddToScheme,
	policyv1.AddToScheme,
	schedulingv1.AddToScheme,
	storagev1.AddToScheme,
}

// AddToScheme adds all types of this clientset into the given scheme. This allows composition
// of clientsets, like in:
//
//	import (
//	  "k8s.io/client-go/kubernetes"
//	  clientsetscheme "k8s.io/client-go/kubernetes/scheme"
//	  aggregatorclientsetscheme "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/scheme"
//	)
//
//	kclientset, _ := kubernetes.NewForConfig(c)
//	_ = aggregatorclientsetscheme.AddToScheme(clientsetscheme.Scheme)
//
// After this, RawExtensions in Kubernetes types will serialize kube-aggregator types
// correctly.
var AddToScheme = localSchemeBuilder.AddToScheme

func init() {
	v1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})
	utilruntime.Must(AddToScheme(Scheme))
}
