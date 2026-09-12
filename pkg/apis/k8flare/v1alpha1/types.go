// Package v1alpha1 contains k8flare's own built-in API group,
// k8flare.com/v1alpha1: the types k8flare uses to manage itself, served by
// the same generic machinery as every upstream type (see
// pkg/apiserver/apidef.Table). It is a compile-time group, the way k3s
// embeds its own types -- NOT a CRD: apiextensions.k8s.io and dynamic type
// registration are explicitly out of scope (docs/cluster-api-design.md).
//
// DeepCopy/DeepCopyInto/DeepCopyObject below are hand-written. This repo
// has no deepcopy-gen in its toolchain and these are the only types in the
// group; if the group grows past a handful of types, wire up upstream's
// k8s.io/code-generator deepcopy-gen (through cmd/k8flare-gen) rather than
// hand-maintaining more of them.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Cluster is one k8flare tenant cluster: its Durable Object tree, its
// public endpoint, and the Secret holding its bootstrap token. It is
// cluster-scoped and lives in the management ("default") cluster, where
// creating/deleting one is how a tenant cluster is provisioned or torn
// down. Reconciliation is the cluster-operator's job (P2 of
// docs/cluster-api-design.md); P1 serves the type for CRUD only.
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// ClusterSpec is the user-declared desired state of a Cluster. The cluster
// name (metadata.name) is the identity -- it is also the /c/<name> path
// segment of the public endpoint -- so nothing here restates it.
type ClusterSpec struct {
	// DisplayName is an optional human-readable label. It has no effect on
	// routing or identity.
	DisplayName string `json:"displayName,omitempty"`
}

// ClusterStatus is the observed state of a Cluster, written by the
// cluster-operator.
type ClusterStatus struct {
	// Phase is a coarse lifecycle summary: "Provisioning", "Ready", or
	// "Terminating". Conditions carry the detail.
	Phase string `json:"phase,omitempty"`

	// DoName is the real name of this cluster's Durable Object tree,
	// "<name>@<uid>". The uid suffix keeps a re-created cluster of the same
	// name from inheriting the deleted one's DO state.
	DoName string `json:"doName,omitempty"`

	// Endpoint is the cluster's public API server URL.
	Endpoint string `json:"endpoint,omitempty"`

	// TokenSecretRef points at the Secret (in the management cluster)
	// holding this cluster's bootstrap token and kubeconfig. The Secret is
	// a distribution mirror; the tenant Cluster DO's vault stays
	// authoritative.
	TokenSecretRef *SecretReference `json:"tokenSecretRef,omitempty"`

	// Conditions is the standard condition list.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the metadata.generation this status was
	// computed from. The operator compares it before writing, so its own
	// status writes don't re-drive the poke pump (see
	// docs/cluster-api-design.md's "poke feedback prevention").
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// SecretReference identifies a Secret by namespace and name. Same shape as
// corev1.SecretReference, declared here rather than imported so this group's
// types don't drag a core/v1 dependency through every consumer for one
// two-string struct.
type SecretReference struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// ClusterList is a list of Clusters.
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Cluster `json:"items"`
}

// DeepCopyInto copies the receiver into out.
func (in *SecretReference) DeepCopyInto(out *SecretReference) {
	*out = *in
}

// DeepCopy returns a deep copy of the receiver.
func (in *SecretReference) DeepCopy() *SecretReference {
	if in == nil {
		return nil
	}
	out := new(SecretReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *ClusterSpec) DeepCopyInto(out *ClusterSpec) {
	*out = *in
}

// DeepCopy returns a deep copy of the receiver.
func (in *ClusterSpec) DeepCopy() *ClusterSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *ClusterStatus) DeepCopyInto(out *ClusterStatus) {
	*out = *in
	if in.TokenSecretRef != nil {
		out.TokenSecretRef = new(SecretReference)
		*out.TokenSecretRef = *in.TokenSecretRef
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *ClusterStatus) DeepCopy() *ClusterStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *Cluster) DeepCopyInto(out *Cluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the receiver.
func (in *Cluster) DeepCopy() *Cluster {
	if in == nil {
		return nil
	}
	out := new(Cluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy of the receiver as a runtime.Object.
func (in *Cluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *ClusterList) DeepCopyInto(out *ClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Cluster, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *ClusterList) DeepCopy() *ClusterList {
	if in == nil {
		return nil
	}
	out := new(ClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy of the receiver as a runtime.Object.
func (in *ClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
