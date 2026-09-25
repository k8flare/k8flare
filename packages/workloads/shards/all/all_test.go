package all

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/k8flare/k8flare/packages/workloads"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	"k8s.io/utils/ptr"
)

func creates(client *fake.Clientset, resource string) int {
	n := 0
	for _, a := range client.Actions() {
		if a.GetVerb() == "create" && a.GetResource().Resource == resource {
			n++
		}
	}
	return n
}

func assignGeneratedNames(client *fake.Clientset) {
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil {
			if obj.GetUID() == "" {
				uids++
				obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			}
			if obj.GetName() == "" && obj.GetGenerateName() != "" {
				uids++
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
}

func TestSyncCreatesReplicaSetThenPods(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil && obj.GetUID() == "" {
			uids++
			obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			if obj.GetName() == "" {
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
	rsCreates, podCreates := 0, 0
	for i := 0; i < 4; i++ {
		client.ClearActions()
		if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		rsCreates += creates(client, "replicasets")
		podCreates += creates(client, "pods")
	}
	if rsCreates != 1 || podCreates != 2 {
		t.Fatalf("replicaset creates = %d, pod creates = %d", rsCreates, podCreates)
	}
	client.ClearActions()
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := creates(client, "pods") + creates(client, "replicasets"); got != 0 {
		t.Fatalf("creates on settled state = %d", got)
	}
}

func TestSyncDeploymentBatchCreatesPods(t *testing.T) {
	labels := map[string]string{"app": "web"}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: "d1"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}}},
		},
	}
	client := fake.NewSimpleClientset(d)
	uids := 0
	client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if obj, err := meta.Accessor(action.(k8stesting.CreateAction).GetObject()); err == nil && obj.GetUID() == "" {
			uids++
			obj.SetUID(types.UID(fmt.Sprintf("uid-%d", uids)))
			if obj.GetName() == "" {
				obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), uids))
			}
		}
		return false, nil, nil
	})
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"deployments"}); err != nil {
		t.Fatal(err)
	}
	if gotRS, gotPods := creates(client, "replicasets"), creates(client, "pods"); gotRS != 1 || gotPods != 2 {
		t.Fatalf("replicaset creates = %d, pod creates = %d", gotRS, gotPods)
	}
}

func TestSyncDisruptionUpdatesPDBStatus(t *testing.T) {
	labels := map[string]string{"foo": "bar"}
	min := intstr.FromInt32(1)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", Generation: 1},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &min,
			Selector:     &metav1.LabelSelector{MatchLabels: labels},
		},
	}
	var pods []runtime.Object
	for i := 0; i < 3; i++ {
		pods = append(pods, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("p%d", i), Namespace: "default", Labels: labels},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
				Conditions: []v1.PodCondition{{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				}},
			},
		})
	}
	objs := append([]runtime.Object{pdb}, pods...)
	client := fake.NewSimpleClientset(objs...)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"poddisruptionbudgets"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.PolicyV1().PodDisruptionBudgets("default").Get(context.Background(), "foo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.ObservedGeneration != 1 {
		t.Fatalf("observedGeneration=%d", got.Status.ObservedGeneration)
	}
	if got.Status.DisruptionsAllowed < 1 {
		t.Fatalf("disruptionsAllowed=%d currentHealthy=%d", got.Status.DisruptionsAllowed, got.Status.CurrentHealthy)
	}
}

func TestSyncStatefulSetReadyReplicasFollowsPodReady(t *testing.T) {
	labels := map[string]string{"app": "ss"}
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss", Namespace: "default", UID: "ss1", Generation: 1},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To[int32](1),
			ServiceName: "ss",
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}},
			},
		},
		Status: appsv1.StatefulSetStatus{Replicas: 1, ReadyReplicas: 1, ObservedGeneration: 1},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ss-0",
			Namespace: "default",
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Name:       "ss",
				UID:        "ss1",
				Controller: ptr.To(true),
			}},
		},
		Spec: v1.PodSpec{Containers: []v1.Container{{Name: "c", Image: "i"}}},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			Conditions: []v1.PodCondition{{
				Type:   v1.PodReady,
				Status: v1.ConditionFalse,
			}},
		},
	}
	client := fake.NewSimpleClientset(ss, pod)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"pods"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.AppsV1().StatefulSets("default").Get(context.Background(), "ss", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.ReadyReplicas != 0 {
		t.Fatalf("readyReplicas=%d", got.Status.ReadyReplicas)
	}
}

func TestSyncCreatesEndpointsForSelectorService(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-sel", Namespace: "default", UID: "s1"},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "none"},
			Ports:    []v1.ServicePort{{Name: "example", Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}
	client := fake.NewSimpleClientset(svc)
	assignGeneratedNames(client)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services"}); err != nil {
		t.Fatal(err)
	}
	if creates(client, "endpoints") == 0 {
		t.Fatal("expected endpoints create")
	}
	if creates(client, "endpointslices") == 0 {
		t.Fatal("expected endpointslice create")
	}
}

func TestSyncCreatesEndpointsForMultiportService(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "default", UID: "m1"},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "none"},
			Ports: []v1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt32(80)},
				{Name: "https", Port: 443, TargetPort: intstr.FromInt32(443)},
			},
		},
	}
	client := fake.NewSimpleClientset(svc)
	assignGeneratedNames(client)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services"}); err != nil {
		t.Fatal(err)
	}
	if creates(client, "endpoints") == 0 {
		t.Fatal("expected endpoints create")
	}
	if creates(client, "endpointslices") == 0 {
		t.Fatal("expected endpointslice create")
	}
}

func TestSyncMirrorsCustomEndpoints(t *testing.T) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-ep", Namespace: "default", UID: "s2"},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{Name: "example", Port: 80, Protocol: v1.ProtocolTCP}},
		},
	}
	ep := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-ep", Namespace: "default", UID: "e1"},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{IP: "10.1.2.3"}},
			Ports:     []v1.EndpointPort{{Port: 80, Protocol: v1.ProtocolTCP}},
		}},
	}
	client := fake.NewSimpleClientset(svc, ep)
	assignGeneratedNames(client)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"services", "endpoints"}); err != nil {
		t.Fatal(err)
	}
	if creates(client, "endpointslices") == 0 {
		t.Fatal("expected endpointslice create")
	}
}

func TestSyncDeletesExpiredBootstrapToken(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-token-prb001", Namespace: metav1.NamespaceSystem, UID: "tok1"},
		Type:       bootstrapapi.SecretTypeBootstrapToken,
		Data: map[string][]byte{
			bootstrapapi.BootstrapTokenIDKey:         []byte("prb001"),
			bootstrapapi.BootstrapTokenSecretKey:     []byte("notasecret0000"),
			bootstrapapi.BootstrapTokenExpirationKey: []byte("2000-01-01T00:00:00Z"),
		},
	}
	client := fake.NewSimpleClientset(secret)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"secrets"}); err != nil {
		t.Fatal(err)
	}
	_, err := client.CoreV1().Secrets(metav1.NamespaceSystem).Get(context.Background(), secret.Name, metav1.GetOptions{})
	if err == nil {
		t.Fatal("expired bootstrap token still present")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

func TestSyncAssignsNodePodCIDR(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewSimpleClientset(node)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"nodes"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.PodCIDR == "" || !strings.HasPrefix(got.Spec.PodCIDR, "10.42.") {
		t.Fatalf("podCIDR = %q", got.Spec.PodCIDR)
	}
}

func TestSyncDeletesExpiredIssuedCSR(t *testing.T) {
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "expired-issued", UID: "csr-old", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    []byte("unused"),
			SignerName: certificatesv1.KubeAPIServerClientSignerName,
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{{
				Type:           certificatesv1.CertificateApproved,
				Status:         v1.ConditionTrue,
				LastUpdateTime: metav1.Now(),
			}},
			Certificate: expiredCertPEM(t),
		},
	}
	client := fake.NewSimpleClientset(csr)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	_, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), csr.Name, metav1.GetOptions{})
	if err == nil {
		t.Fatal("expired issued CSR still present")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

func expiredCertPEM(t *testing.T) []byte {
	t.Helper()
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired"},
		NotBefore:    time.Now().Add(-2 * time.Hour),
		NotAfter:     time.Now().Add(-time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &pk.PublicKey, pk)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestSyncApprovesKubeletClientCSR(t *testing.T) {
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "system:node:foo", Organization: []string{"system:nodes"}},
	}, pk)
	if err != nil {
		t.Fatal(err)
	}
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node-foo", UID: "c1", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
			SignerName: certificatesv1.KubeAPIServerClientKubeletSignerName,
			Username:   "system:node:foo",
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
	client := fake.NewSimpleClientset(csr)
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}}, nil
	})
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, nil, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "node-foo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	approved := false
	for _, c := range got.Status.Conditions {
		if c.Type == certificatesv1.CertificateApproved && c.Status == v1.ConditionTrue {
			approved = true
		}
	}
	if !approved {
		t.Fatalf("conditions = %+v", got.Status.Conditions)
	}
}

func TestSyncIssuesKubeletClientCSR(t *testing.T) {
	signingCA := testSigningCA(t)
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "system:node:foo", Organization: []string{"system:nodes"}},
	}, pk)
	if err != nil {
		t.Fatal(err)
	}
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node-foo", UID: "c1", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
			SignerName: certificatesv1.KubeAPIServerClientKubeletSignerName,
			Username:   "system:node:foo",
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
	client := fake.NewSimpleClientset(csr)
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}}, nil
	})
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), signingCA, nil, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "node-foo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Certificate) == 0 {
		t.Fatalf("certificate missing; conditions = %+v", got.Status.Conditions)
	}
}

func TestSyncIssuesKubeletServingCSR(t *testing.T) {
	servingCA := testSigningCA(t)
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: "system:node:foo", Organization: []string{"system:nodes"}},
		DNSNames:    []string{"foo"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}, pk)
	if err != nil {
		t.Fatal(err)
	}
	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "node-serve", UID: "c2", ResourceVersion: "1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}),
			SignerName: certificatesv1.KubeletServingSignerName,
			Username:   "system:node:foo",
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageServerAuth},
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{{
				Type:   certificatesv1.CertificateApproved,
				Status: v1.ConditionTrue,
				Reason: "Approved",
			}},
		},
	}
	client := fake.NewSimpleClientset(csr)
	if _, err := workloads.Sync(context.Background(), client, []byte("ca"), nil, servingCA, []string{"certificatesigningrequests"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "node-serve", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Certificate) == 0 {
		t.Fatalf("certificate missing; conditions = %+v", got.Status.Conditions)
	}
}

func testSigningCA(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
}
