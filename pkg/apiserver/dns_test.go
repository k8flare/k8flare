package apiserver_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/miekg/dns"
)

// dohQuery sends one DoH (RFC 8484) A-record query to /dns-query using
// the cluster token, mirroring exactly what pkg/dnsshim's forwardDoH does.
func dohQuery(t *testing.T, name string) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	packed, err := m.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/dns-query", testPort), bytes.NewReader(packed))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Authorization", "Bearer k8flare-dev-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DoH request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DoH request: HTTP %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	var reply dns.Msg
	if err := reply.Unpack(buf.Bytes()); err != nil {
		t.Fatalf("unpack reply: %v (raw %x)", err, buf.Bytes())
	}
	return &reply
}

// TestClusterDNS drives the DoH synthesis endpoint (pkg/apiserver/dns.go)
// against real Service/Pod/EndpointSlice objects -- the other half of
// cluster DNS is pkg/dnsshim (cmd/agent), out of scope for this
// wrangler-dev-backed suite.
func TestClusterDNS(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	t.Run("NoAuth", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost,
			fmt.Sprintf("http://127.0.0.1:%d/dns-query", testPort), bytes.NewReader(nil))
		req.Header.Set("Content-Type", "application/dns-message")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", resp.StatusCode)
		}
	})

	t.Run("NXDOMAIN", func(t *testing.T) {
		reply := dohQuery(t, "no-such-service.default.svc.cluster.local")
		if reply.Rcode != dns.RcodeNameError {
			t.Fatalf("rcode = %v, want NXDOMAIN", dns.RcodeToString[reply.Rcode])
		}
	})

	t.Run("ClusterIPService", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "dns-test-clusterip"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "dns-test-clusterip"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		}
		created, err := client.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create service: %v", err)
		}
		t.Cleanup(func() {
			_ = client.CoreV1().Services("default").Delete(ctx, svc.Name, metav1.DeleteOptions{})
		})

		reply := dohQuery(t, "dns-test-clusterip.default.svc.cluster.local")
		if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 1 {
			t.Fatalf("rcode=%v answers=%d, want NOERROR/1: %v", dns.RcodeToString[reply.Rcode], len(reply.Answer), reply)
		}
		a, ok := reply.Answer[0].(*dns.A)
		if !ok {
			t.Fatalf("answer is not an A record: %v", reply.Answer[0])
		}
		if a.A.String() != created.Spec.ClusterIP {
			t.Fatalf("resolved %s, want ClusterIP %s", a.A.String(), created.Spec.ClusterIP)
		}
	})

	t.Run("HeadlessServiceResolvesToPodIP", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "dns-test-headless"},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector:  map[string]string{"app": "dns-test-headless"},
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		}
		if _, err := client.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create headless service: %v", err)
		}
		t.Cleanup(func() {
			_ = client.CoreV1().Services("default").Delete(ctx, svc.Name, metav1.DeleteOptions{})
		})

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "dns-test-headless-pod", Labels: map[string]string{"app": "dns-test-headless"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}
		created, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create pod: %v", err)
		}
		t.Cleanup(func() {
			_ = client.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
		})
		created.Status = corev1.PodStatus{
			PodIP: "10.42.0.123",
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		}
		if _, err := client.CoreV1().Pods("default").UpdateStatus(ctx, created, metav1.UpdateOptions{}); err != nil {
			t.Fatalf("update pod status: %v", err)
		}

		// EndpointSlice synthesis (from the Pod status update above) is
		// asynchronous, so poll rather than assuming it landed instantly.
		var reply *dns.Msg
		for i := 0; i < 10; i++ {
			reply = dohQuery(t, "dns-test-headless.default.svc.cluster.local")
			if reply.Rcode == dns.RcodeSuccess && len(reply.Answer) > 0 {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 1 {
			t.Fatalf("rcode=%v answers=%d, want NOERROR/1: %v", dns.RcodeToString[reply.Rcode], len(reply.Answer), reply)
		}
		a, ok := reply.Answer[0].(*dns.A)
		if !ok || a.A.String() != "10.42.0.123" {
			t.Fatalf("answer = %v, want A 10.42.0.123", reply.Answer[0])
		}
	})
}
