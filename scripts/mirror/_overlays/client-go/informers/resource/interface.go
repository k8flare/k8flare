//go:build js

package resource

import (
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	v1 "k8s.io/client-go/informers/resource/v1"
	v1beta2 "k8s.io/client-go/informers/resource/v1beta2"
)

type Interface interface {
	V1() v1.Interface
	V1beta2() v1beta2.Interface
}

type group struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &group{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}
func (g *group) V1() v1.Interface {
	return v1.New(g.factory, g.namespace, g.tweakListOptions)
}
func (g *group) V1beta2() v1beta2.Interface {
	return v1beta2.New(g.factory, g.namespace, g.tweakListOptions)
}
