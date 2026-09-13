//go:build js

// GOOS=js overlay: upstream polls etcd endpoints for feature support and
// links the etcd client. There is no etcd here.
package feature

import "k8s.io/apiserver/pkg/storage"

var DefaultFeatureSupportChecker FeatureSupportChecker = noEtcd{}

type FeatureSupportChecker interface {
	Supports(feature storage.Feature) bool
}

type noEtcd struct{}

func (noEtcd) Supports(storage.Feature) bool { return false }
