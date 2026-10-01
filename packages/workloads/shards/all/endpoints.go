package all

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/kubernetes/pkg/controller/endpoint"
)

type controllerOwnedLeftovers struct {
	coreinformers.EndpointsInformer
}

func (i controllerOwnedLeftovers) Lister() corelisters.EndpointsLister {
	return controllerOwnedEndpoints{i.EndpointsInformer.Lister()}
}

type controllerOwnedEndpoints struct {
	corelisters.EndpointsLister
}

func (l controllerOwnedEndpoints) List(selector labels.Selector) ([]*v1.Endpoints, error) {
	listed, err := l.EndpointsLister.List(selector)
	if err != nil {
		return nil, err
	}
	owned := listed[:0:0]
	for _, ep := range listed {
		if ep.Labels[endpoint.LabelManagedBy] == endpoint.ControllerName {
			owned = append(owned, ep)
		}
	}
	return owned, nil
}
