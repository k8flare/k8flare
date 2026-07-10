//go:build js && wasm && !schedwidth

// *Clientset alone satisfies kubernetes.Interface under plain and
// `-tags leanwidth` builds (stubs.go/stubs_leanwidth.go cover the rest).
// Not under `-tags schedwidth`: StorageV1/ResourceV1/PolicyV1 come only
// from SchedulerClientset there (scheduler.go), whose own
// `var _ kubernetes.Interface = (*SchedulerClientset)(nil)` covers that
// build instead -- see clientset.go's doc comment.
package clientset

import kubernetes "k8s.io/client-go/kubernetes"

var _ kubernetes.Interface = (*Clientset)(nil)
