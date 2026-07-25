// js/wasm overlay: the real implementation polls etcd endpoints for
// feature support; there is no etcd on this platform.
package feature

var DefaultFeatureSupportChecker FeatureSupportChecker = &noEtcd{}

type FeatureSupportChecker interface {
	Supports(feature string) bool
}

type noEtcd struct{}

func (n *noEtcd) Supports(string) bool { return false }
