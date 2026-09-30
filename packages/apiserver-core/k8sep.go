package core

import (
	"context"
	"net"
	"sort"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
)

const kubernetesAPIProxyPort int32 = 6443

var endpointSliceCodec = func() runtime.Codec {
	s := runtime.NewScheme()
	utilruntime.Must(discoveryv1.AddToScheme(s))
	return serializer.NewCodecFactory(s).LegacyCodec(discoveryv1.SchemeGroupVersion)
}()

var k8sEP = struct {
	nodes     *registry.Store
	endpoints *registry.Store
	kine      *kine.Client
}{}

func bindKubernetesEndpoints(stores map[string]*registry.Store) {
	k8sEP.nodes = stores["nodes"]
	k8sEP.endpoints = stores["endpoints"]
	k8sEP.kine = nsAccounts.kine
}

func reconcileKubernetesEndpoints(ctx context.Context) {
	if k8sEP.nodes == nil || k8sEP.endpoints == nil {
		return
	}
	listed, err := k8sEP.nodes.List(genericapirequest.WithNamespace(ctx, metav1.NamespaceNone), &metainternalversion.ListOptions{})
	if err != nil {
		println("apiserver: kubernetes endpoints list nodes:", err.Error())
		return
	}
	nodes, ok := listed.(*corev1.NodeList)
	if !ok {
		return
	}
	ips := readyNodeIPs(nodes.Items)
	desired := kubernetesEndpoints(ips)
	writeKubernetesSlice(ctx, ips)
	epCtx := genericapirequest.WithNamespace(ctx, metav1.NamespaceDefault)
	got, err := k8sEP.endpoints.Get(epCtx, desired.Name, &metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := k8sEP.endpoints.Create(epCtx, desired, rest.ValidateAllObjectFunc, &metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
				println("apiserver: kubernetes endpoints create:", err.Error())
			}
		}
		return
	}
	cur := got.(*corev1.Endpoints)
	desired.ObjectMeta = cur.ObjectMeta
	if _, _, err := k8sEP.endpoints.Update(epCtx, desired.Name, rest.DefaultUpdatedObjectInfo(desired), rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{}); err != nil {
		println("apiserver: kubernetes endpoints update:", err.Error())
	}
}

func afterNodeWrite(context.Context, runtime.Object) genericregistry.FinishFunc {
	return func(ctx context.Context, success bool) {
		if success {
			reconcileKubernetesEndpoints(ctx)
		}
	}
}

func readyNodeIPs(nodes []corev1.Node) []string {
	seen := map[string]bool{}
	var ips []string
	for i := range nodes {
		if !nodeReady(&nodes[i]) {
			continue
		}
		for _, addr := range nodes[i].Status.Addresses {
			if addr.Type != corev1.NodeInternalIP {
				continue
			}
			if net.ParseIP(addr.Address) == nil || seen[addr.Address] {
				continue
			}
			seen[addr.Address] = true
			ips = append(ips, addr.Address)
		}
	}
	sort.Strings(ips)
	return ips
}

func nodeReady(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func kubernetesEndpoints(ips []string) *corev1.Endpoints {
	addrs := make([]corev1.EndpointAddress, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, corev1.EndpointAddress{IP: ip})
	}
	var subsets []corev1.EndpointSubset
	if len(addrs) > 0 {
		subsets = []corev1.EndpointSubset{{
			Addresses: addrs,
			Ports:     []corev1.EndpointPort{{Name: "https", Port: kubernetesAPIProxyPort, Protocol: corev1.ProtocolTCP}},
		}}
	}
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: metav1.NamespaceDefault, Labels: map[string]string{"endpointslice.kubernetes.io/skip-mirror": "true"}},
		Subsets:    subsets,
	}
}

func writeKubernetesSlice(ctx context.Context, ips []string) {
	if k8sEP.kine == nil {
		return
	}
	store := kine.NewStorage(k8sEP.kine, endpointSliceCodec, func() runtime.Object { return &discoveryv1.EndpointSlice{} })
	key := "/endpointslices/default/kubernetes"
	if err := store.Create(ctx, key, kubernetesSlice(ips), nil, 0); err == nil {
		return
	} else if err != kine.ErrConflict && !storage.IsExist(err) {
		println("apiserver: kubernetes endpointslice create:", err.Error())
	}
	cur := &discoveryv1.EndpointSlice{}
	if err := store.GuaranteedUpdate(ctx, key, cur, true, nil, func(input runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
		old, _ := input.(*discoveryv1.EndpointSlice)
		next := kubernetesSlice(ips)
		if old != nil {
			next.ObjectMeta = old.ObjectMeta
			ensureSliceIdentity(next)
		}
		return next, nil, nil
	}, nil); err != nil {
		println("apiserver: kubernetes endpointslice:", err.Error())
	}
}

func ensureSliceIdentity(slice *discoveryv1.EndpointSlice) {
	if slice.UID == "" {
		slice.UID = uuid.NewUUID()
	}
	if slice.CreationTimestamp.IsZero() {
		slice.CreationTimestamp = metav1.Now()
	}
}

func kubernetesSlice(ips []string) *discoveryv1.EndpointSlice {
	ready, serving, terminating := false, true, true
	port := kubernetesAPIProxyPort
	proto := corev1.ProtocolTCP
	name := "https"
	eps := make([]discoveryv1.Endpoint, 0, len(ips))
	for _, ip := range ips {
		addr := ip
		eps = append(eps, discoveryv1.Endpoint{Addresses: []string{addr}, Conditions: discoveryv1.EndpointConditions{Ready: &ready, Serving: &serving, Terminating: &terminating}})
	}
	return &discoveryv1.EndpointSlice{
		TypeMeta: metav1.TypeMeta{APIVersion: discoveryv1.SchemeGroupVersion.String(), Kind: "EndpointSlice"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kubernetes",
			Namespace:         metav1.NamespaceDefault,
			UID:               uuid.NewUUID(),
			CreationTimestamp: metav1.Now(),
			Labels:            map[string]string{"kubernetes.io/service-name": "kubernetes"},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   eps,
		Ports:       []discoveryv1.EndpointPort{{Name: &name, Protocol: &proto, Port: &port}},
	}
}
