//go:build js && wasm && leanwidth

// *Clientset alone satisfies kubernetes.Interface under `-tags leanwidth`
// (stubs_leanwidth.go covers the methods it does not implement).
//
// The untagged build is gone (2026-09-13): stubs.go carried ~45 panic stubs
// for it and no shipped configuration ever compiled the file -- every WASM
// entrypoint passes leanwidth or schedwidth, and the one untagged GOOS=js
// build, selector-wasm, does not import this package.
// Not under `-tags schedwidth`: StorageV1/ResourceV1/PolicyV1 come only
// from SchedulerClientset there (scheduler.go), whose own
// `var _ kubernetes.Interface = (*SchedulerClientset)(nil)` covers that
// build instead -- see clientset.go's doc comment.
package clientset

import kubernetes "k8s.io/client-go/kubernetes"

var _ kubernetes.Interface = (*Clientset)(nil)
