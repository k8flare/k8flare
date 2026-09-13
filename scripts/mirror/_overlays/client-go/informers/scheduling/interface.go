//go:build js

package scheduling

import (
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	v1alpha2 "k8s.io/client-go/informers/scheduling/v1alpha2"
)

type Interface interface {
	V1alpha2() v1alpha2.Interface
}

type group struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &group{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}
func (g *group) V1alpha2() v1alpha2.Interface {
	return v1alpha2.New(g.factory, g.namespace, g.tweakListOptions)
}
