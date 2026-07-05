//go:build js && wasm

// Package clientset aggregates pkg/leanclient/gen's five per-group
// Clients into a single kubernetes.Interface -- a separate package from
// pkg/leanclient itself so pkg/leanclient/gen/* (which imports
// pkg/leanclient for its verb/watch helpers) doesn't form an import
// cycle by also being imported from here.
package clientset

import (
	kubernetes "k8s.io/client-go/kubernetes"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	discoveryv1client "k8s.io/client-go/kubernetes/typed/discovery/v1"
	restclient "k8s.io/client-go/rest"

	"github.com/k8flare/k8flare/pkg/leanclient/gen/appsv1"
	"github.com/k8flare/k8flare/pkg/leanclient/gen/batchv1"
	"github.com/k8flare/k8flare/pkg/leanclient/gen/coordinationv1"
	"github.com/k8flare/k8flare/pkg/leanclient/gen/corev1"
	"github.com/k8flare/k8flare/pkg/leanclient/gen/discoveryv1"
)

// Clientset implements kubernetes.Interface (this repo's pruned, 5-group
// third_party/clientgo-lean-overlays/kubernetes/clientset.go version --
// see that file's doc comment) by delegating each group accessor to the
// matching pkg/leanclient/gen/<group> package's Client, all five sharing
// cfg's Transport.
type Clientset struct {
	core         *corev1.Client
	apps         *appsv1.Client
	batch        *batchv1.Client
	discovery    *discoveryv1.Client
	coordination *coordinationv1.Client
}

var _ kubernetes.Interface = (*Clientset)(nil)

// NewForConfig mirrors client-go's own generated Clientset constructor's
// name (kubernetes.NewForConfig), so callers moving between the two only
// change an import.
func NewForConfig(cfg *restclient.Config) (*Clientset, error) {
	core, err := corev1.New(cfg)
	if err != nil {
		return nil, err
	}
	apps, err := appsv1.New(cfg)
	if err != nil {
		return nil, err
	}
	batch, err := batchv1.New(cfg)
	if err != nil {
		return nil, err
	}
	discovery, err := discoveryv1.New(cfg)
	if err != nil {
		return nil, err
	}
	coordination, err := coordinationv1.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Clientset{core: core, apps: apps, batch: batch, discovery: discovery, coordination: coordination}, nil
}

func (c *Clientset) CoreV1() corev1client.CoreV1Interface { return c.core }

func (c *Clientset) AppsV1() appsv1client.AppsV1Interface { return c.apps }

func (c *Clientset) BatchV1() batchv1client.BatchV1Interface { return c.batch }

func (c *Clientset) DiscoveryV1() discoveryv1client.DiscoveryV1Interface { return c.discovery }

func (c *Clientset) CoordinationV1() coordinationv1client.CoordinationV1Interface {
	return c.coordination
}
