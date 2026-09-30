package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	registry "github.com/k8flare/k8flare/packages/apiserver-registry"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestEncodeIPAddress(t *testing.T) {
	data, err := runtime.Encode(ipCodec, &networkingv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "10.43.0.5"},
		Spec:       networkingv1.IPAddressSpec{ParentRef: &networkingv1.ParentReference{Resource: "services", Namespace: "default", Name: "web"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "networking.k8s.io/v1") || !strings.Contains(string(data), "10.43.0.5") {
		t.Fatalf("encoded %s", data)
	}
}

func TestClaimServiceIPKeepsExistingOwner(t *testing.T) {
	data, err := runtime.Encode(ipCodec, &networkingv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "10.43.0.1"},
		Spec:       networkingv1.IPAddressSpec{ParentRef: &networkingv1.ParentReference{Resource: "services", Namespace: "default", Name: "kubernetes"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	var repaired bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": r.URL.Query().Get("key"), "value": encoded, "modRevision": 1}})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if rev, _ := body["revision"].(float64); rev == 1 {
			repaired = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2})
			return
		}
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1})
	}))
	t.Cleanup(srv.Close)
	deps := registry.Deps{Kine: &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"}, Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.1", ClusterIPs: []string{"10.43.0.1"}}}
	if err := claimServiceIPs(context.Background(), deps, svc, []string{"10.43.0.1"}); err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("existing IPAddress kept an empty identity")
	}
	other := svc.DeepCopy()
	other.Name = "other"
	if err := claimServiceIPs(context.Background(), deps, other, []string{"10.43.0.1"}); err == nil {
		t.Fatal("expected a conflict for a different service")
	}
}

func TestFinishKeepsIPWhenServiceAlreadyExists(t *testing.T) {
	var deleted bool
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gotKey = r.URL.Query().Get("key")
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "kv": map[string]any{"key": r.URL.Query().Get("key"), "value": base64.StdEncoding.EncodeToString([]byte(`{"spec":{"clusterIP":"10.43.0.1"}}`)), "modRevision": 1}})
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 2})
		case http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": 3})
		}
	}))
	t.Cleanup(srv.Close)
	deps := registry.Deps{Kine: &kine.Client{HTTP: &http.Client{Transport: rewrite{base: srv.URL, next: srv.Client().Transport}}}}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"}, Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.1", ClusterIPs: []string{"10.43.0.1"}}}
	finish, err := finishServiceIPs(context.Background(), deps, nil, svc)
	if err != nil {
		t.Fatal(err)
	}
	finish(context.Background(), false)
	if deleted {
		t.Fatal("failed create released an IP the stored service already uses")
	}
	if gotKey != "/registry/services/default/kubernetes" {
		t.Fatalf("service key %s", gotKey)
	}
}

func TestFinishServiceIPsWithoutStore(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}, Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.5"}}
	finish, err := finishServiceIPs(context.Background(), registry.Deps{}, nil, svc)
	if err != nil || finish == nil {
		t.Fatal(err)
	}
	finish(context.Background(), false)
}

func TestAddedIPs(t *testing.T) {
	got := addedIPs([]string{"10.43.0.1"}, []string{"10.43.0.1", "10.43.0.2"})
	if len(got) != 1 || got[0] != "10.43.0.2" {
		t.Fatal(got)
	}
}

func TestIPFromAddressKey(t *testing.T) {
	ip, ok := ipFromAddressKey("/registry/ipaddresses/10.43.0.5")
	if !ok || ip != "10.43.0.5" {
		t.Fatalf("v4 %s ok=%v", ip, ok)
	}
	ip, ok = ipFromAddressKey("/registry/ipaddresses/2001-db8--1")
	if !ok || ip != "2001:db8::1" {
		t.Fatalf("v6 %s ok=%v", ip, ok)
	}
	if _, ok := ipFromAddressKey("/registry/services/default/kubernetes"); ok {
		t.Fatal("service key")
	}
}

func TestPickClusterIPSkipsReservedAndTaken(t *testing.T) {
	taken := reservedClusterIPs()
	taken["10.43.0.2"] = true
	ip, err := pickClusterIP(taken)
	if err != nil {
		t.Fatal(err)
	}
	if ip != "10.43.0.3" {
		t.Fatalf("got %s", ip)
	}
}

func TestPickNodePortSkipsTaken(t *testing.T) {
	taken := map[int32]bool{30000: true, 30001: true}
	p, ok := pickNodePort(taken)
	if !ok || p != 30002 {
		t.Fatalf("got %d ok=%v", p, ok)
	}
}

func TestNeedsNodePorts(t *testing.T) {
	if needsNodePorts(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName}}) {
		t.Fatal("external name")
	}
	if needsNodePorts(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}) {
		t.Fatal("cluster ip")
	}
	if !needsNodePorts(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort}}) {
		t.Fatal("node port")
	}
	off := false
	if needsNodePorts(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, AllocateLoadBalancerNodePorts: &off}}) {
		t.Fatal("lb allocate off")
	}
}

func TestDropServiceTypeFieldsReleasesNodePorts(t *testing.T) {
	on := true
	class := "example.com/lb"
	old := &corev1.Service{Spec: corev1.ServiceSpec{
		Type:                          corev1.ServiceTypeLoadBalancer,
		AllocateLoadBalancerNodePorts: &on,
		LoadBalancerClass:             &class,
		ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyLocal,
		HealthCheckNodePort:           30001,
		Ports:                         []corev1.ServicePort{{Port: 80, NodePort: 30000}},
	}, Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}}}}
	next := old.DeepCopy()
	next.Spec.Type = corev1.ServiceTypeClusterIP
	dropServiceTypeFields(next, old)
	if next.Spec.Ports[0].NodePort != 0 || next.Spec.HealthCheckNodePort != 0 || next.Spec.AllocateLoadBalancerNodePorts != nil || next.Spec.LoadBalancerClass != nil || next.Spec.ExternalTrafficPolicy != "" {
		t.Fatalf("spec=%+v", next.Spec)
	}
	if len(next.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("status=%+v", next.Status)
	}
	policy := corev1.IPFamilyPolicySingleStack
	externalOld := &corev1.Service{Spec: corev1.ServiceSpec{
		Type:           corev1.ServiceTypeClusterIP,
		ClusterIP:      "10.43.1.1",
		ClusterIPs:     []string{"10.43.1.1"},
		IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol},
		IPFamilyPolicy: &policy,
	}}
	external := externalOld.DeepCopy()
	external.Spec.Type = corev1.ServiceTypeExternalName
	dropServiceTypeFields(external, externalOld)
	if external.Spec.IPFamilies != nil || external.Spec.IPFamilyPolicy != nil {
		t.Fatalf("external=%+v", external.Spec)
	}
}

func TestAllocateNodePortsRejectsOutOfRange(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type:  corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{{Port: 80, NodePort: 80}},
	}}
	if err := allocateNodePorts(svc, map[int32]bool{}); err == nil {
		t.Fatal("expected out-of-range nodePort to be rejected")
	}
}

func TestAllocateNodePortsRejectsTaken(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type:  corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{{Port: 80, NodePort: 30000}},
	}}
	if err := allocateNodePorts(svc, map[int32]bool{30000: true}); err == nil {
		t.Fatal("expected allocated port to be rejected")
	}
	svc.Spec.Ports[0].NodePort = 0
	if err := allocateNodePorts(svc, map[int32]bool{30000: true}); err != nil {
		t.Fatal(err)
	}
	if svc.Spec.Ports[0].NodePort != 30001 {
		t.Fatalf("nodePort=%d", svc.Spec.Ports[0].NodePort)
	}
}

func TestAssignNodePortsSharesServicePort(t *testing.T) {
	svc := &corev1.Service{Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{
			{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
			{Name: "http-udp", Port: 80, Protocol: corev1.ProtocolUDP},
		},
	}}
	assignNodePorts(context.Background(), svc, registry.Deps{})
	if svc.Spec.Ports[0].NodePort == 0 || svc.Spec.Ports[0].NodePort != svc.Spec.Ports[1].NodePort {
		t.Fatalf("ports: %#v", svc.Spec.Ports)
	}
}

func TestAllocateHealthCheckNodePort(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type:                  corev1.ServiceTypeLoadBalancer,
		ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
		Ports:                 []corev1.ServicePort{{Port: 80, NodePort: 30000}},
	}}
	if err := allocateNodePorts(svc, map[int32]bool{}); err != nil {
		t.Fatal(err)
	}
	if err := allocateHealthCheckNodePort(svc, map[int32]bool{30000: true}); err != nil {
		t.Fatal(err)
	}
	if svc.Spec.HealthCheckNodePort != 30001 {
		t.Fatalf("healthCheckNodePort=%d", svc.Spec.HealthCheckNodePort)
	}
	out := svc.DeepCopy()
	out.Spec.HealthCheckNodePort = 80
	if err := allocateHealthCheckNodePort(out, map[int32]bool{}); err == nil {
		t.Fatal("expected out-of-range healthCheckNodePort")
	}
	taken := out.DeepCopy()
	taken.Spec.HealthCheckNodePort = 30000
	if err := allocateHealthCheckNodePort(taken, map[int32]bool{30000: true}); err == nil {
		t.Fatal("expected allocated healthCheckNodePort")
	}
	cluster := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, HealthCheckNodePort: 30000}}
	if err := healthCheckNodePortAllowed(cluster, nil); err == nil {
		t.Fatal("expected healthCheckNodePort on ClusterIP to be rejected")
	}
	local := svc.DeepCopy()
	changed := local.DeepCopy()
	changed.Spec.HealthCheckNodePort = 30002
	if err := immutableHealthCheckNodePort(changed, local); err == nil {
		t.Fatal("expected immutable healthCheckNodePort")
	}
}

func TestImmutableClusterIPs(t *testing.T) {
	old := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.43.1.1", ClusterIPs: []string{"10.43.1.1", "2001:db8::1"},
	}}
	changed := old.DeepCopy()
	changed.Spec.ClusterIPs = []string{"10.43.1.1", "2001:db8::2"}
	if err := immutableClusterIPs(changed, old); err == nil {
		t.Fatal("expected secondary clusterIP to be immutable")
	}
	single := old.DeepCopy()
	single.Spec.ClusterIPs = []string{"10.43.1.1"}
	if err := immutableClusterIPs(single, old); err == nil {
		t.Fatal("expected secondary release to require SingleStack")
	}
	policy := corev1.IPFamilyPolicySingleStack
	single.Spec.IPFamilyPolicy = &policy
	if err := immutableClusterIPs(single, old); err != nil {
		t.Fatal(err)
	}
}

func TestServiceSelectorAllowed(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"BAD KEY": "web"}}}
	if err := serviceSelectorAllowed(svc); err == nil {
		t.Fatal("expected invalid selector")
	}
	svc.Spec.Selector = map[string]string{"app": "web"}
	if err := serviceSelectorAllowed(svc); err != nil {
		t.Fatal(err)
	}
}

func TestServiceTypeAllowed(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{Type: "Ingress"}}
	if err := serviceTypeAllowed(svc); err == nil {
		t.Fatal("expected unsupported service type")
	}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	if err := serviceTypeAllowed(svc); err != nil {
		t.Fatal(err)
	}
}

func TestExternalIPsAllowed(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{ExternalIPs: []string{"127.0.0.1"}}}
	if err := externalIPsAllowed(svc, nil); err == nil {
		t.Fatal("expected loopback externalIP")
	}
	svc.Spec.ExternalIPs = []string{"203.0.113.10"}
	if err := externalIPsAllowed(svc, nil); err != nil {
		t.Fatal(err)
	}
	old := &corev1.Service{Spec: corev1.ServiceSpec{ExternalIPs: []string{"127.0.0.1"}}}
	kept := &corev1.Service{Spec: corev1.ServiceSpec{ExternalIPs: []string{"127.0.0.1"}}}
	if err := externalIPsAllowed(kept, old); err != nil {
		t.Fatal(err)
	}
}

func TestInternalTrafficPolicyAllowed(t *testing.T) {
	bad := corev1.ServiceInternalTrafficPolicy("Everywhere")
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{InternalTrafficPolicy: &bad}}
	if err := internalTrafficPolicyAllowed(svc); err == nil {
		t.Fatal("expected unsupported internalTrafficPolicy")
	}
	ok := corev1.ServiceInternalTrafficPolicyLocal
	svc.Spec.InternalTrafficPolicy = &ok
	if err := internalTrafficPolicyAllowed(svc); err != nil {
		t.Fatal(err)
	}
}

func TestExternalTrafficPolicyAllowed(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP, ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyCluster,
	}}
	if err := externalTrafficPolicyAllowed(svc, nil); err == nil {
		t.Fatal("expected externalTrafficPolicy on ClusterIP to be rejected")
	}
	nodePort := svc.DeepCopy()
	nodePort.Spec.Type = corev1.ServiceTypeNodePort
	nodePort.Spec.ExternalTrafficPolicy = "LocalOnly"
	if err := externalTrafficPolicyAllowed(nodePort, nil); err == nil {
		t.Fatal("expected unsupported externalTrafficPolicy")
	}
	nodePort.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
	if err := externalTrafficPolicyAllowed(svc, nodePort); err != nil {
		t.Fatal(err)
	}
}

func TestServicePortValues(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Ports: []corev1.ServicePort{{Port: 0}, {Port: 80, Protocol: "ICMP"}},
	}}
	if err := servicePortValues(svc); err == nil {
		t.Fatal("expected invalid ports")
	}
	ok := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
		{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
		{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP},
	}}}
	if err := servicePortValues(ok); err != nil {
		t.Fatal(err)
	}
	badName := ok.DeepCopy()
	badName.Spec.Ports[0].Name = "HTTP"
	if err := servicePortValues(badName); err == nil {
		t.Fatal("expected invalid port name")
	}
	badTarget := ok.DeepCopy()
	badTarget.Spec.Ports[0].TargetPort = intstr.FromString("HTTP")
	if err := servicePortValues(badTarget); err == nil {
		t.Fatal("expected invalid targetPort")
	}
	proto := "Not A Protocol"
	badProto := ok.DeepCopy()
	badProto.Spec.Ports[0].AppProtocol = &proto
	if err := servicePortValues(badProto); err == nil {
		t.Fatal("expected invalid appProtocol")
	}
}

func TestExternalNameAllowed(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName}}
	if err := externalNameAllowed(svc, nil); err == nil {
		t.Fatal("expected empty externalName")
	}
	svc.Spec.ExternalName = "example.com"
	policy := corev1.IPFamilyPolicySingleStack
	svc.Spec.IPFamilyPolicy = &policy
	if err := externalNameAllowed(svc, nil); err == nil {
		t.Fatal("expected ipFamilyPolicy to be rejected")
	}
	old := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, IPFamilyPolicy: &policy}}
	if err := externalNameAllowed(svc, old); err != nil {
		t.Fatal(err)
	}
	svc.Spec.ExternalName = "NOT_A_HOST"
	svc.Spec.IPFamilyPolicy = nil
	if err := externalNameAllowed(svc, nil); err == nil {
		t.Fatal("expected invalid externalName")
	}
}

func TestServicePortsRequired(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}
	if err := servicePortsRequired(svc); err == nil {
		t.Fatal("expected empty ports to be rejected")
	}
	headless := svc.DeepCopy()
	headless.Spec.ClusterIP = corev1.ClusterIPNone
	if err := servicePortsRequired(headless); err != nil {
		t.Fatal(err)
	}
	external := svc.DeepCopy()
	external.Spec.Type = corev1.ServiceTypeExternalName
	if err := servicePortsRequired(external); err != nil {
		t.Fatal(err)
	}
}

func TestSessionAffinityAllowed(t *testing.T) {
	bad := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{SessionAffinity: "sticky"}}
	if err := sessionAffinityAllowed(bad); err == nil {
		t.Fatal("expected unsupported session affinity")
	}
	timeout := int32(0)
	clientIP := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		SessionAffinity:       corev1.ServiceAffinityClientIP,
		SessionAffinityConfig: &corev1.SessionAffinityConfig{ClientIP: &corev1.ClientIPConfig{TimeoutSeconds: &timeout}},
	}}
	if err := sessionAffinityAllowed(clientIP); err == nil {
		t.Fatal("expected zero timeout to be rejected")
	}
	timeout = 10800
	if err := sessionAffinityAllowed(clientIP); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBalancerSourceRangesOnlyOnLoadBalancer(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP, LoadBalancerSourceRanges: []string{"10.0.0.0/8"},
	}}
	if err := loadBalancerSourceRangesAllowed(svc, nil); err == nil {
		t.Fatal("expected source ranges on ClusterIP to be rejected")
	}
	lb := svc.DeepCopy()
	lb.Spec.Type = corev1.ServiceTypeLoadBalancer
	if err := loadBalancerSourceRangesAllowed(lb, nil); err != nil {
		t.Fatal(err)
	}
	lb.Spec.LoadBalancerSourceRanges = []string{"not-a-cidr"}
	if err := loadBalancerSourceRangesAllowed(lb, nil); err == nil {
		t.Fatal("expected invalid source range")
	}
}

func TestDuplicateServicePorts(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{
			{Port: 80, Protocol: corev1.ProtocolTCP, NodePort: 30000},
			{Port: 80, Protocol: corev1.ProtocolTCP, NodePort: 30000},
		},
	}}
	if err := duplicateServicePorts(svc); err == nil {
		t.Fatal("expected duplicate ports")
	}
	shared := svc.DeepCopy()
	shared.Spec.Ports[1].Protocol = corev1.ProtocolUDP
	if err := duplicateServicePorts(shared); err != nil {
		t.Fatal(err)
	}
}

func TestClusterIPRejectsNodePort(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP, Ports: []corev1.ServicePort{{Port: 80, NodePort: 30000}},
	}}
	if err := clusterIPNodePortAllowed(svc, nil); err == nil {
		t.Fatal("expected nodePort on ClusterIP to be rejected")
	}
	old := svc.DeepCopy()
	old.Spec.Type = corev1.ServiceTypeNodePort
	if err := clusterIPNodePortAllowed(svc, old); err != nil {
		t.Fatal(err)
	}
}

func TestClusterIPNoneRejectedForNodePort(t *testing.T) {
	nodePort := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeNodePort, ClusterIP: corev1.ClusterIPNone,
	}}
	if err := clusterIPNoneAllowed(nodePort); err == nil {
		t.Fatal("expected NodePort None to be rejected")
	}
	lb := nodePort.DeepCopy()
	lb.Spec.Type = corev1.ServiceTypeLoadBalancer
	if err := clusterIPNoneAllowed(lb); err == nil {
		t.Fatal("expected LoadBalancer None to be rejected")
	}
	cluster := nodePort.DeepCopy()
	cluster.Spec.Type = corev1.ServiceTypeClusterIP
	if err := clusterIPNoneAllowed(cluster); err != nil {
		t.Fatal(err)
	}
}

func TestClusterIPSliceAgrees(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		ClusterIP: "10.43.1.1", ClusterIPs: []string{"10.43.1.2"},
	}}
	if err := clusterIPSliceAgrees(svc); err == nil {
		t.Fatal("expected clusterIPs[0] to match clusterIP")
	}
	headless := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		ClusterIP: corev1.ClusterIPNone, ClusterIPs: []string{corev1.ClusterIPNone, "10.43.1.2"},
	}}
	if err := clusterIPSliceAgrees(headless); err == nil {
		t.Fatal("expected None to be the only clusterIP")
	}
	ok := svc.DeepCopy()
	ok.Spec.ClusterIPs = []string{"10.43.1.1"}
	if err := clusterIPSliceAgrees(ok); err != nil {
		t.Fatal(err)
	}
	onlySlice := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{ClusterIPs: []string{"10.43.1.1"}}}
	if err := clusterIPSliceAgrees(onlySlice); err == nil {
		t.Fatal("expected clusterIPs without clusterIP to be rejected")
	}
}

func TestImmutableIPFamilies(t *testing.T) {
	old := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.43.1.1", ClusterIPs: []string{"10.43.1.1", "2001:db8::1"},
		IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
	}}
	changed := old.DeepCopy()
	changed.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv6Protocol, corev1.IPv4Protocol}
	if err := immutableIPFamilies(changed, old); err == nil {
		t.Fatal("expected ipFamilies to be immutable")
	}
	single := old.DeepCopy()
	single.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
	if err := immutableIPFamilies(single, old); err == nil {
		t.Fatal("expected secondary family release to require SingleStack")
	}
	policy := corev1.IPFamilyPolicySingleStack
	single.Spec.IPFamilyPolicy = &policy
	if err := immutableIPFamilies(single, old); err != nil {
		t.Fatal(err)
	}
}

func TestImmutableClusterIP(t *testing.T) {
	old := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.43.1.1"}}
	changed := old.DeepCopy()
	changed.Spec.ClusterIP = "10.43.1.2"
	if err := immutableClusterIP(changed, old); err == nil {
		t.Fatal("expected immutable clusterIP")
	}
	if err := immutableClusterIP(old.DeepCopy(), old); err != nil {
		t.Fatal(err)
	}
	external := old.DeepCopy()
	external.Spec.Type = corev1.ServiceTypeExternalName
	external.Spec.ClusterIP = ""
	if err := immutableClusterIP(external, old); err != nil {
		t.Fatal(err)
	}
}

func TestExternalNameDropsClusterIP(t *testing.T) {
	svc := &corev1.Service{Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeExternalName, ClusterIP: "10.43.0.2", ClusterIPs: []string{"10.43.0.2"}, ExternalName: "example.com",
		HealthCheckNodePort: 30001,
		Ports:               []corev1.ServicePort{{Port: 80, NodePort: 30000}},
	}}
	if !releaseExternalName(svc) || svc.Spec.ClusterIP != "" || svc.Spec.ClusterIPs != nil || svc.Spec.Ports[0].NodePort != 0 || svc.Spec.HealthCheckNodePort != 0 {
		t.Fatalf("%#v", svc.Spec)
	}
}

func TestHeadlessServiceGetsNoneClusterIPs(t *testing.T) {
	svc := &corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone}}
	defaultHeadlessClusterIPs(svc)
	if len(svc.Spec.ClusterIPs) != 1 || svc.Spec.ClusterIPs[0] != corev1.ClusterIPNone {
		t.Fatalf("clusterIPs=%v", svc.Spec.ClusterIPs)
	}
	if err := clusterIPSliceAgrees(svc); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultServiceIPFamily(t *testing.T) {
	svc := &corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.2"}}
	defaultServiceIPFamily(svc)
	if svc.Spec.IPFamilyPolicy == nil || *svc.Spec.IPFamilyPolicy != corev1.IPFamilyPolicySingleStack {
		t.Fatalf("policy=%v", svc.Spec.IPFamilyPolicy)
	}
	if len(svc.Spec.IPFamilies) != 1 || svc.Spec.IPFamilies[0] != corev1.IPv4Protocol {
		t.Fatalf("families=%v", svc.Spec.IPFamilies)
	}
}

func TestSpecifiedClusterIPAllowed(t *testing.T) {
	outside := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: corev1.ServiceSpec{ClusterIP: "192.0.2.1"}}
	if err := specifiedClusterIPAllowed(outside); err == nil {
		t.Fatal("expected clusterIP outside the service CIDR")
	}
	network := outside.DeepCopy()
	network.Spec.ClusterIP = "10.43.0.0"
	if err := specifiedClusterIPAllowed(network); err == nil {
		t.Fatal("expected network address to be rejected")
	}
	ok := outside.DeepCopy()
	ok.Spec.ClusterIP = "10.43.1.20"
	if err := specifiedClusterIPAllowed(ok); err != nil {
		t.Fatal(err)
	}
}

func TestNeedsClusterIP(t *testing.T) {
	if needsClusterIP(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName}}) {
		t.Fatal("external name")
	}
	if needsClusterIP(&corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone}}) {
		t.Fatal("headless")
	}
	if needsClusterIP(&corev1.Service{Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.20"}}) {
		t.Fatal("already set")
	}
	if !needsClusterIP(&corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}) {
		t.Fatal("load balancer")
	}
}
