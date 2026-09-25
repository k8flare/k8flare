package admission

import (
	"net/http/httptest"
	"strings"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	psapi "k8s.io/pod-security-admission/api"
)

func TestPodSecurityDeniesPrivilegedOnBaseline(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/namespaces/psa": mustJSON(t, corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "psa",
				Labels: map[string]string{psapi.EnforceLevelLabel: string(psapi.LevelBaseline)},
			},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, psaPodReq("psa", true))
	if out.Allowed {
		t.Fatal("expected privileged pod deny")
	}
	if !strings.Contains(out.Message, `violates PodSecurity "baseline:latest"`) {
		t.Fatalf("deny message: %q", out.Message)
	}
}

func TestPodSecurityAllowsPrivilegedWhenUnlabeled(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, psaPodReq("default", true))
	if !out.Allowed {
		t.Fatalf("unlabeled ns should stay privileged: %+v", out)
	}
}

func TestPodSecurityAllowsBaselinePod(t *testing.T) {
	kineSrv := httptest.NewServer(memStore{data: map[string][]byte{
		"/registry/namespaces/psa": mustJSON(t, corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "psa",
				Labels: map[string]string{psapi.EnforceLevelLabel: string(psapi.LevelBaseline)},
			},
		}),
	}})
	defer kineSrv.Close()
	h := NewHandler(Config{Kine: rewriteClient(kineSrv)})
	out := postAdmit(t, h, psaPodReq("psa", false))
	if !out.Allowed {
		t.Fatalf("baseline-compliant pod denied: %+v", out)
	}
}

func psaPodReq(ns string, privileged bool) admit.Request {
	container := map[string]any{"name": "c", "image": "img"}
	if privileged {
		container["securityContext"] = map[string]any{"privileged": true}
	}
	return admit.Request{
		Phase:     "validate",
		Name:      "p",
		Namespace: ns,
		Resource:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Kind:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		Operation: "CREATE",
		Object: map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "p", "namespace": ns},
			"spec":     map[string]any{"containers": []any{container}},
		},
	}
}
