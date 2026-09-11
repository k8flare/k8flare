package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func servicesResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, corev1.SchemeGroupVersion, "services", "service", true,
		func() runtime.Object { return &corev1.Service{} },
		func() runtime.Object { return &corev1.ServiceList{} },
	)
}

func endpointSlicesResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, discoveryv1.SchemeGroupVersion, "endpointslices", "endpointslice", true,
		func() runtime.Object { return &discoveryv1.EndpointSlice{} },
		func() runtime.Object { return &discoveryv1.EndpointSliceList{} },
	)
}

// ResolveVKubeProxyTarget resolves a ClusterIP:port pair -- the original
// destination pkg/agent/vkubeproxy.go's node-side TUN interception
// carries as X-K8flare-Target-IP/-Port headers -- to the (Pod UID,
// container port) pair a ready backing Pod-on-Containers Pod should
// receive the request on. This is the resolution half of nodes/
// podproxy.ts's handleVKubeProxy (ClusterIP -> Service -> EndpointSlice,
// the read side of what the real endpointslice controller in the kcm
// dynamic worker writes), moved here to run in-process against
// this apiserver's own ResourceStore rather than as two separate HTTP
// round trips through client.ts's getServiceByClusterIP and
// listEndpointSlicesForService.
//
// The final hop -- forwarding the request to the resolved Pod's NodeVM
// Durable Object -- stays in TS (nodes/podproxy.ts's forwardToPod): a
// Loader-loaded per-request dynamic worker has no DO bindings (S2), so
// only the shell Worker can make that last call. handleVKubeProxyResolve
// below is the internal endpoint TS calls for this half.
func ResolveVKubeProxyTarget(ctx context.Context, storage *Storage, clusterIP string, targetPort int) (podUID string, containerPort int, err error) {
	svcStore := servicesResourceStore(storage)
	fieldSelector := fmt.Sprintf("spec.clusterIP=%s", clusterIP)
	svcObj, err := svcStore.List(ctx, "", fieldSelector, "")
	if err != nil {
		return "", 0, fmt.Errorf("list services: %w", err)
	}
	svcList, ok := svcObj.(*corev1.ServiceList)
	if !ok || len(svcList.Items) == 0 {
		return "", 0, fmt.Errorf("no Service with ClusterIP %s", clusterIP)
	}
	// ClusterIPs are unique cluster-wide, so at most one Service ever
	// matches (same invariant client.ts's getServiceByClusterIP doc
	// comment records).
	svc := &svcList.Items[0]

	var svcPort *corev1.ServicePort
	for i := range svc.Spec.Ports {
		if int(svc.Spec.Ports[i].Port) == targetPort {
			svcPort = &svc.Spec.Ports[i]
			break
		}
	}
	if svcPort == nil {
		return "", 0, fmt.Errorf("service %s/%s has no port %d", svc.Namespace, svc.Name, targetPort)
	}

	epStore := endpointSlicesResourceStore(storage)
	// The standard EndpointSlice-to-Service label (the real
	// endpointslice controller sets it); the loop below handles a list,
	// same as podproxy.ts's handleVKubeProxy did.
	labelSelector := fmt.Sprintf("kubernetes.io/service-name=%s", svc.Name)
	epObj, err := epStore.List(ctx, svc.Namespace, "", labelSelector)
	if err != nil {
		return "", 0, fmt.Errorf("list endpointslices: %w", err)
	}
	epList, ok := epObj.(*discoveryv1.EndpointSliceList)
	if !ok {
		return "", 0, fmt.Errorf("unexpected endpointslice list type %T", epObj)
	}

	for _, slice := range epList.Items {
		var resolvedPort int32
		var portFound bool
		for _, p := range slice.Ports {
			if p.Port == nil {
				continue
			}
			// Match by name if the Service names its ports, otherwise
			// there is only one to pick -- same rule client.ts's
			// handleVKubeProxy used.
			if svcPort.Name != "" {
				if p.Name != nil && *p.Name == svcPort.Name {
					resolvedPort, portFound = *p.Port, true
					break
				}
				continue
			}
			resolvedPort, portFound = *p.Port, true
			break
		}
		if !portFound {
			continue
		}
		for _, ep := range slice.Endpoints {
			ready := ep.Conditions.Ready == nil || *ep.Conditions.Ready
			if ready && ep.TargetRef != nil && ep.TargetRef.UID != "" {
				return string(ep.TargetRef.UID), int(resolvedPort), nil
			}
		}
	}
	return "", 0, fmt.Errorf("no ready endpoint for service %s/%s:%d", svc.Namespace, svc.Name, targetPort)
}

// vkubeproxyResolveResponse is the wire shape nodes/podproxy.ts's
// handleVKubeProxy decodes to find the NodeVM to forward to.
type vkubeproxyResolveResponse struct {
	PodUID        string `json:"podUID"`
	ContainerPort int    `json:"containerPort"`
}

// handleVKubeProxyResolve serves GET /internal/vkubeproxy-resolve?ip=&port=,
// called by nodes/podproxy.ts's handleVKubeProxy in place of its own
// former two-round-trip client.ts resolution.
func handleVKubeProxyResolve(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		portStr := r.URL.Query().Get("port")
		port, err := strconv.Atoi(portStr)
		if ip == "" || err != nil {
			writeStatusError(w, http.StatusBadRequest, "BadRequest", "ip and a numeric port query parameter are required")
			return
		}
		podUID, containerPort, err := ResolveVKubeProxyTarget(r.Context(), storage, ip, port)
		if err != nil {
			writeStatusError(w, http.StatusBadGateway, "BadGateway", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vkubeproxyResolveResponse{PodUID: podUID, ContainerPort: containerPort})
	}
}
