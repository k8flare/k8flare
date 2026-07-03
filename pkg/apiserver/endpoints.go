package apiserver

import (
	"context"
	"errors"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	endpointsliceutil "k8s.io/endpointslice/util"
)

// legacyEndpointsManagedBy and the endpoint*ControllerName constants below
// are hand-copied, not imported, from
// k8s.io/kubernetes/pkg/controller/endpoint.LabelManagedBy/ControllerName
// and k8s.io/kubernetes/pkg/controller/endpointslice.ControllerName -- see
// findPort's doc comment for why those packages can't be imported just for
// one small piece of them.
const (
	legacyEndpointsManagedBy    = "endpoints.kubernetes.io/managed-by"
	endpointControllerName      = "endpoint-controller"
	endpointSliceControllerName = "endpointslice-controller.k8s.io"
)

func endpointsResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, "endpoints", true,
		func() runtime.Object { return &corev1.Endpoints{} },
		func() runtime.Object { return &corev1.EndpointsList{} },
	)
}

func endpointSlicesResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, "endpointslices", true,
		func() runtime.Object { return &discoveryv1.EndpointSlice{} },
		func() runtime.Object { return &discoveryv1.EndpointSliceList{} },
	)
}

func servicesResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, "services", true,
		func() runtime.Object { return &corev1.Service{} },
		func() runtime.Object { return &corev1.ServiceList{} },
	)
}

func podsResourceStore(storage *Storage) *ResourceStore {
	return NewResourceStore(storage, "pods", true,
		func() runtime.Object { return &corev1.Pod{} },
		func() runtime.Object { return &corev1.PodList{} },
	)
}

// ReconcileNamespaceEndpoints recomputes Endpoints and EndpointSlice objects
// for every selector-having Service in namespace, based on the Pods
// currently in that namespace. Called synchronously from handler.go's and
// subresource.go's Service/Pod write paths (create, update, status update,
// delete) -- restores what the deleted workers/storage/src/endpoints.ts did
// from Cluster DO's alarm loop (see docs/platform-verification.md's Phase 5
// findings for why the real kube-controller-manager's endpoint/endpointslice
// controllers can't fit Workers' size budget as a replacement), but scoped
// to one namespace per call (each call site already knows which namespace
// changed) and driven synchronously by the write itself instead of a timer.
//
// A Service is managed here when it has a selector and isn't ExternalName --
// same scope endpoints.ts had. Headless Services (spec.clusterIP == "None")
// with a selector are still managed (only ClusterIP allocation skips them).
// Services without a selector are owned externally and left untouched.
//
// IPv4-only, matching this project's existing ServiceCIDR/clusterCIDR scope
// (supervisor.go, nodecidr.go) -- addressType is always IPv4 and only
// pod.Status.PodIP (not the dual-stack PodIPs list) is read, same
// simplification endpoints.ts made.
func ReconcileNamespaceEndpoints(ctx context.Context, storage *Storage, namespace string) error {
	svcListObj, err := servicesResourceStore(storage).List(ctx, namespace, "", "")
	if err != nil {
		return fmt.Errorf("reconcile endpoints: list services: %w", err)
	}
	svcList, ok := svcListObj.(*corev1.ServiceList)
	if !ok {
		return fmt.Errorf("reconcile endpoints: unexpected list type %T for services", svcListObj)
	}

	podListObj, err := podsResourceStore(storage).List(ctx, namespace, "", "")
	if err != nil {
		return fmt.Errorf("reconcile endpoints: list pods: %w", err)
	}
	podList, ok := podListObj.(*corev1.PodList)
	if !ok {
		return fmt.Errorf("reconcile endpoints: unexpected list type %T for pods", podListObj)
	}
	pods := make([]*corev1.Pod, len(podList.Items))
	for i := range podList.Items {
		pods[i] = &podList.Items[i]
	}

	for i := range svcList.Items {
		svc := &svcList.Items[i]
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			continue
		}
		if len(svc.Spec.Selector) == 0 {
			continue // no selector: externally managed, left untouched
		}
		if err := reconcileServiceEndpoints(ctx, storage, svc, pods); err != nil {
			log.Printf("endpoints reconciliation error for %s/%s: %v", namespace, svc.Name, err)
		}
	}
	return nil
}

// DeleteServiceEndpoints removes the Endpoints and EndpointSlice objects
// owned by the given (now-deleted) Service, if any. Called from handler.go's
// Service DELETE paths. Best-effort and idempotent -- NotFound is expected
// and ignored, matching ReleaseClusterIP/ReleasePodCIDR's style -- since
// namespace-cascading delete (DeleteNamespaceDependents) already sweeps
// Endpoints/EndpointSlice generically as ordinary namespaced resources when
// a whole Namespace is deleted, this only needs to handle the direct
// single/collection Service delete paths.
func DeleteServiceEndpoints(ctx context.Context, storage *Storage, namespace, name string) {
	if _, err := endpointsResourceStore(storage).Delete(ctx, namespace, name); err != nil && !isNotFoundErr(err) {
		log.Printf("failed to delete Endpoints for %s/%s: %v", namespace, name, err)
	}
	if _, err := endpointSlicesResourceStore(storage).Delete(ctx, namespace, name); err != nil && !isNotFoundErr(err) {
		log.Printf("failed to delete EndpointSlice for %s/%s: %v", namespace, name, err)
	}
}

func isNotFoundErr(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonNotFound
}

// reconcileServiceEndpoints computes and upserts the Endpoints and
// EndpointSlice for one Service, from the Pods matching its selector among
// candidatePods (every Pod in the Service's namespace -- selector matching
// happens here).
func reconcileServiceEndpoints(ctx context.Context, storage *Storage, svc *corev1.Service, candidatePods []*corev1.Pod) error {
	selector := labels.SelectorFromValidatedSet(svc.Spec.Selector)
	var matching []*corev1.Pod
	for _, pod := range candidatePods {
		if selector.Matches(labels.Set(pod.Labels)) {
			matching = append(matching, pod)
		}
	}

	slice := buildEndpointSlice(svc, matching)
	if err := upsertEndpointSlice(ctx, storage, slice); err != nil {
		return fmt.Errorf("upsert EndpointSlice: %w", err)
	}

	eps := buildLegacyEndpoints(svc, matching)
	if err := upsertLegacyEndpoints(ctx, storage, eps); err != nil {
		return fmt.Errorf("upsert Endpoints: %w", err)
	}
	return nil
}

// buildEndpointSlice builds the desired discovery.k8s.io/v1 EndpointSlice
// for svc from its matching Pods. A single IPv4 slice per Service is a
// deliberate simplification inherited from the deleted endpoints.ts: real
// kube-controller-manager hash-suffixes the name and shards a Service across
// multiple EndpointSlices (by address type and by a max-per-slice cap);
// neither is needed at this project's scale.
func buildEndpointSlice(svc *corev1.Service, pods []*corev1.Pod) *discoveryv1.EndpointSlice {
	ports := resolveEndpointPorts(svc.Spec.Ports, pods)

	endpoints := make([]discoveryv1.Endpoint, 0, len(pods))
	for _, pod := range pods {
		if pod.Status.PodIP == "" {
			continue // no address yet -- nothing to publish for this Pod
		}
		endpoints = append(endpoints, podToDiscoveryEndpoint(svc, pod))
	}

	ownerRef := metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service"))
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svc.Name,
			Namespace:       svc.Namespace,
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
			Labels: map[string]string{
				discoveryv1.LabelServiceName: svc.Name,
				discoveryv1.LabelManagedBy:   endpointSliceControllerName,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   endpoints,
		Ports:       ports,
	}
}

// podToDiscoveryEndpoint mirrors k8s.io/endpointslice's unexported
// podToEndpoint (utils.go) -- same hand-port reasoning as findPort. Omits
// the Zone/topology field (this project has no Node topology labels) and
// Hostname (ShouldSetHostname's Hostname-subdomain-matching gate, reused
// directly below since it's cheap -- see findPort's doc comment).
func podToDiscoveryEndpoint(svc *corev1.Service, pod *corev1.Pod) discoveryv1.Endpoint {
	serving := endpointsliceutil.IsPodReady(pod)
	terminating := pod.DeletionTimestamp != nil
	ready := svc.Spec.PublishNotReadyAddresses || (serving && !terminating)

	ep := discoveryv1.Endpoint{
		Addresses: []string{pod.Status.PodIP},
		Conditions: discoveryv1.EndpointConditions{
			Ready:       &ready,
			Serving:     &serving,
			Terminating: &terminating,
		},
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
	}
	if pod.Spec.NodeName != "" {
		ep.NodeName = &pod.Spec.NodeName
	}
	if endpointsliceutil.ShouldSetHostname(pod, svc) {
		ep.Hostname = &pod.Spec.Hostname
	}
	return ep
}

// buildLegacyEndpoints builds the desired core/v1 Endpoints for svc from its
// matching Pods, sharing its name/namespace by convention. A single subset
// covers the homogeneous-Pod case this project targets; an empty subsets
// list is written when no Pods match yet (matches upstream).
func buildLegacyEndpoints(svc *corev1.Service, pods []*corev1.Pod) *corev1.Endpoints {
	corePorts := resolveLegacyEndpointPorts(svc.Spec.Ports, pods)

	var ready, notReady []corev1.EndpointAddress
	for _, pod := range pods {
		if pod.Status.PodIP == "" {
			continue
		}
		addr := corev1.EndpointAddress{
			IP: pod.Status.PodIP,
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: pod.Namespace,
				Name:      pod.Name,
				UID:       pod.UID,
			},
		}
		if pod.Spec.NodeName != "" {
			addr.NodeName = &pod.Spec.NodeName
		}
		if svc.Spec.PublishNotReadyAddresses || endpointsliceutil.IsPodReady(pod) {
			ready = append(ready, addr)
		} else {
			notReady = append(notReady, addr)
		}
	}

	var subsets []corev1.EndpointSubset
	if len(ready) > 0 || len(notReady) > 0 {
		subsets = []corev1.EndpointSubset{{
			Addresses:         ready,
			NotReadyAddresses: notReady,
			Ports:             corePorts,
		}}
	}

	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels:    map[string]string{legacyEndpointsManagedBy: endpointControllerName},
		},
		Subsets: subsets,
	}
}

// resolveEndpointPorts resolves svc's ports against pods into
// discovery.k8s.io/v1 EndpointPorts, for the single combined EndpointSlice
// this project builds per Service (see buildEndpointSlice's doc comment on
// the homogeneous-Pod assumption this makes: each named targetPort is
// resolved against the first matching Pod that exposes it, not per-Pod as
// real upstream's per-slice-grouping design does).
func resolveEndpointPorts(svcPorts []corev1.ServicePort, pods []*corev1.Pod) []discoveryv1.EndpointPort {
	if len(svcPorts) == 0 && len(pods) == 0 {
		return nil
	}
	ports := make([]discoveryv1.EndpointPort, 0, len(svcPorts))
	for i := range svcPorts {
		svcPort := &svcPorts[i]
		portNum, ok := resolveTargetPort(svcPort, pods)
		if !ok {
			continue // no matching Pod exposes this named port -- omitted, matches upstream
		}
		name := svcPort.Name
		protocol := svcPort.Protocol
		port32 := int32(portNum)
		ports = append(ports, discoveryv1.EndpointPort{
			Name:        &name,
			Port:        &port32,
			Protocol:    &protocol,
			AppProtocol: svcPort.AppProtocol,
		})
	}
	return ports
}

// resolveLegacyEndpointPorts is resolveEndpointPorts's core/v1 twin (the
// legacy Endpoints API has its own, near-identical EndpointPort type).
func resolveLegacyEndpointPorts(svcPorts []corev1.ServicePort, pods []*corev1.Pod) []corev1.EndpointPort {
	ports := make([]corev1.EndpointPort, 0, len(svcPorts))
	for i := range svcPorts {
		svcPort := &svcPorts[i]
		portNum, ok := resolveTargetPort(svcPort, pods)
		if !ok {
			continue
		}
		ports = append(ports, corev1.EndpointPort{
			Name:        svcPort.Name,
			Port:        int32(portNum),
			Protocol:    svcPort.Protocol,
			AppProtocol: svcPort.AppProtocol,
		})
	}
	return ports
}

// resolveTargetPort resolves one Service port's effective container port
// number against the first of pods that exposes it, via findPort below.
// ApplyDefaults (handler.go) already runs the real upstream Service
// defaulter before a Service is ever stored, which fills in an unset
// TargetPort from Port (k8s.io/kubernetes/pkg/apis/core/v1/defaults.go's
// SetDefaults_Service) -- so unlike endpoints.ts (which never ran real
// upstream defaulting and had to special-case "unset"), this can trust
// svcPort.TargetPort is always a concrete Int or String by the time it gets
// here.
func resolveTargetPort(svcPort *corev1.ServicePort, pods []*corev1.Pod) (int, bool) {
	if svcPort.TargetPort.Type == intstr.Int {
		return svcPort.TargetPort.IntValue(), true
	}
	for _, pod := range pods {
		if port, err := findPort(pod, svcPort); err == nil {
			return port, true
		}
	}
	return 0, false
}

// findPort mirrors k8s.io/endpointslice's exported FindPort (utils.go) --
// hand-ported rather than imported because FindPort lives in
// k8s.io/endpointslice's root package, which also contains reconciler.go's
// NewReconciler (requiring a full k8s.io/client-go/kubernetes.Interface via
// its clientset.Interface parameter). Go compiles a package as a whole, so
// importing FindPort would still pay for client-go's typed Clientset even
// though FindPort itself never touches it -- measured directly: adding
// `import "k8s.io/endpointslice"` to this binary moved its gzip size from
// 7,699,133 to 10,469,635 bytes (+2,770,502 bytes, ~2.64MiB), which alone
// blows this project's 9.5MiB CI budget (see docs/cost-model.md). By
// contrast k8s.io/endpointslice/util (a separate, genuinely lightweight
// package -- no client-go import) is imported directly above for
// IsPodReady/ShouldSetHostname, at a measured +34KB -- the split happens to
// fall on the right side of the package boundary for those two.
func findPort(pod *corev1.Pod, svcPort *corev1.ServicePort) (int, error) {
	portName := svcPort.TargetPort
	switch portName.Type {
	case intstr.String:
		name := portName.StrVal
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				if port.Name == name && port.Protocol == svcPort.Protocol {
					return int(port.ContainerPort), nil
				}
			}
		}
		// Sidecar containers (initContainers with restartPolicy=Always) can
		// also expose named ports, matching real FindPort.
		for _, container := range pod.Spec.InitContainers {
			if container.RestartPolicy == nil || *container.RestartPolicy != corev1.ContainerRestartPolicyAlways {
				continue
			}
			for _, port := range container.Ports {
				if port.Name == name && port.Protocol == svcPort.Protocol {
					return int(port.ContainerPort), nil
				}
			}
		}
	case intstr.Int:
		return portName.IntValue(), nil
	}
	return 0, fmt.Errorf("no suitable port for pod: %s", pod.UID)
}

// upsertEndpointSlice writes desired unless the stored EndpointSlice's
// meaningful fields (Endpoints/Ports/Labels) are already identical --
// skipping an unchanged write avoids resourceVersion churn and watch-event
// noise on every reconcile pass, mirroring endpoints.ts's upsertKey.
func upsertEndpointSlice(ctx context.Context, storage *Storage, desired *discoveryv1.EndpointSlice) error {
	store := endpointSlicesResourceStore(storage)
	current, err := store.Get(ctx, desired.Namespace, desired.Name)
	if err != nil {
		if isNotFoundErr(err) {
			_, err := store.Create(ctx, desired.Namespace, desired)
			return err
		}
		return err
	}
	currentSlice, ok := current.(*discoveryv1.EndpointSlice)
	if ok &&
		apiequality.Semantic.DeepEqual(currentSlice.Endpoints, desired.Endpoints) &&
		apiequality.Semantic.DeepEqual(currentSlice.Ports, desired.Ports) &&
		apiequality.Semantic.DeepEqual(currentSlice.Labels, desired.Labels) {
		return nil
	}
	_, err = store.Update(ctx, desired.Namespace, desired.Name, desired)
	return err
}

// upsertLegacyEndpoints is upsertEndpointSlice's core/v1 twin.
func upsertLegacyEndpoints(ctx context.Context, storage *Storage, desired *corev1.Endpoints) error {
	store := endpointsResourceStore(storage)
	current, err := store.Get(ctx, desired.Namespace, desired.Name)
	if err != nil {
		if isNotFoundErr(err) {
			_, err := store.Create(ctx, desired.Namespace, desired)
			return err
		}
		return err
	}
	currentEps, ok := current.(*corev1.Endpoints)
	if ok && apiequality.Semantic.DeepEqual(currentEps.Subsets, desired.Subsets) {
		return nil
	}
	_, err = store.Update(ctx, desired.Namespace, desired.Name, desired)
	return err
}

// TriggerEndpointsReconcile recomputes Endpoints/EndpointSlice for
// namespace whenever a write is to a Service or Pod -- the single call
// site handler.go and subresource.go's write paths use, so each of them is
// one line instead of a repeated type-switch. Errors are logged, not
// surfaced: Endpoints/EndpointSlice are a derived, best-effort projection
// (the same role a real controller's own reconcile loop would play), not
// something a client write should fail over.
func TriggerEndpointsReconcile(ctx context.Context, storage *Storage, namespace string, obj runtime.Object) {
	switch obj.(type) {
	case *corev1.Service, *corev1.Pod:
	default:
		return
	}
	if err := ReconcileNamespaceEndpoints(ctx, storage, namespace); err != nil {
		log.Printf("endpoints reconciliation error for namespace %s: %v", namespace, err)
	}
}
