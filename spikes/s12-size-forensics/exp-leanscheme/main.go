//go:build js && wasm

// exp-leanscheme validates the hypothesis found while forensically
// isolating why a "narrow" typed client (single group) came out the same
// size as the full aggregate Clientset (50.50 vs 50.71 MiB raw): every
// generated typed/<group>/v1 package's setConfigDefaults() hardcodes
// `scheme.Scheme, scheme.Codecs` from k8s.io/client-go/kubernetes/scheme
// -- a package whose init() unconditionally registers ~40 API groups via
// AddToScheme, regardless of which single group's typed client you asked
// for. That's the real fixed cost "narrowing which group" never touched.
//
// This reuses the exact same generated per-group typed client structs
// (CoreV1Client, AppsV1Client, ...) via their New(rest.Interface)
// constructor -- NOT NewForConfig, which is what calls the expensive
// setConfigDefaults -- with a hand-built minimal runtime.Scheme
// registering only the 5 groups this repo's 10 controllers + scheduler
// actually touch (confirmed by grep: core, apps, batch, discovery,
// coordination; zero Server-Side Apply / applyconfigurations usage
// anywhere in pkg/controller/{replicaset,deployment,daemon,job,cronjob,
// endpoint,endpointslice,nodeipam,nodelifecycle,tainteviction} or
// pkg/scheduler).
package main

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1client "k8s.io/client-go/kubernetes/typed/discovery/v1"
	"k8s.io/client-go/rest"
)

func main() {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = discoveryv1.AddToScheme(scheme)
	_ = coordinationv1.AddToScheme(scheme)

	codecs := serializer.NewCodecFactory(scheme)
	negotiated := rest.CodecFactoryForGeneratedClient(scheme, codecs).WithoutConversion()

	coreGV := corev1.SchemeGroupVersion
	coreCfg := &rest.Config{Host: "https://example.invalid", APIPath: "/api"}
	coreCfg.GroupVersion = &coreGV
	coreCfg.NegotiatedSerializer = negotiated
	coreRC, err := rest.RESTClientFor(coreCfg)
	if err != nil {
		panic(err)
	}
	core := corev1client.New(coreRC)
	_ = core.Pods("default")
	_ = core.Nodes()
	_ = core.Services("default")
	_ = core.Endpoints("default")

	appsGV := appsv1.SchemeGroupVersion
	appsCfg := &rest.Config{Host: "https://example.invalid", APIPath: "/apis"}
	appsCfg.GroupVersion = &appsGV
	appsCfg.NegotiatedSerializer = negotiated
	appsRC, err := rest.RESTClientFor(appsCfg)
	if err != nil {
		panic(err)
	}
	apps := appsv1client.New(appsRC)
	_ = apps.ReplicaSets("default")
	_ = apps.Deployments("default")
	_ = apps.DaemonSets("default")
	_ = apps.ControllerRevisions("default")

	batchGV := batchv1.SchemeGroupVersion
	batchCfg := &rest.Config{Host: "https://example.invalid", APIPath: "/apis"}
	batchCfg.GroupVersion = &batchGV
	batchCfg.NegotiatedSerializer = negotiated
	batchRC, err := rest.RESTClientFor(batchCfg)
	if err != nil {
		panic(err)
	}
	batch := batchv1client.New(batchRC)
	_ = batch.Jobs("default")
	_ = batch.CronJobs("default")

	discGV := discoveryv1.SchemeGroupVersion
	discCfg := &rest.Config{Host: "https://example.invalid", APIPath: "/apis"}
	discCfg.GroupVersion = &discGV
	discCfg.NegotiatedSerializer = negotiated
	discRC, err := rest.RESTClientFor(discCfg)
	if err != nil {
		panic(err)
	}
	disc := discoveryv1client.New(discRC)
	_ = disc.EndpointSlices("default")

	coordGV := coordinationv1.SchemeGroupVersion
	coordCfg := &rest.Config{Host: "https://example.invalid", APIPath: "/apis"}
	coordCfg.GroupVersion = &coordGV
	coordCfg.NegotiatedSerializer = negotiated
	coordRC, err := rest.RESTClientFor(coordCfg)
	if err != nil {
		panic(err)
	}
	coord := coordinationv1client.New(coordRC)
	_ = coord.Leases("default")

	println("built")
}
