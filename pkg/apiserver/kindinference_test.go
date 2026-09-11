package apiserver_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The real kube-controller-manager's event broadcaster POSTs an Event body
// with no apiVersion/kind and got 400 "Object 'Kind' is missing" back,
// losing the events that didn't get retried (TODO.md P2-3, S32). Upstream's
// create/update handlers hand the route's own GroupVersionKind to the
// decoder as the default, so a TypeMeta-less body is read as the kind the
// URL already names.
func TestCreateInfersKindFromPath(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "kindinfer-event"

	_ = client.CoreV1().Events(ns).Delete(ctx, name, metav1.DeleteOptions{})
	t.Cleanup(func() {
		_ = client.CoreV1().Events(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	body := fmt.Sprintf(`{"metadata":{"name":%q,"namespace":%q},`+
		`"involvedObject":{"kind":"Node","name":"kindinfer-node"},`+
		`"reason":"NodeNotReady","message":"kind inferred from the path","type":"Normal","count":1}`, name, ns)
	status, respBody := postRaw(t,
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/namespaces/%s/events", testPort, ns), body)
	if status != http.StatusCreated {
		t.Fatalf("POST events without apiVersion/kind: got %d %s, want 201", status, respBody)
	}

	got, err := client.CoreV1().Events(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get the created event: %v", err)
	}
	if got.Reason != "NodeNotReady" {
		t.Errorf("reason = %q, want NodeNotReady", got.Reason)
	}
}

// Same inference on the update path, which upstream defaults identically.
func TestUpdateInfersKindFromPath(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "kindinfer-cm"

	_ = client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if _, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string]string{"k": "before"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create configmap: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().ConfigMaps(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	body := fmt.Sprintf(`{"metadata":{"name":%q,"namespace":%q},"data":{"k":"after"}}`, name, ns)
	status, respBody := putRaw(t,
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/namespaces/%s/configmaps/%s", testPort, ns, name), body)
	if status != http.StatusOK {
		t.Fatalf("PUT configmap without apiVersion/kind: got %d %s, want 200", status, respBody)
	}

	got, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get the updated configmap: %v", err)
	}
	if got.Data["k"] != "after" {
		t.Errorf("data.k = %q, want after", got.Data["k"])
	}
}

// A body that DOES name its kind keeps being read as that kind: the
// route's kind is a default for an absent TypeMeta, never an override.
func TestCreateKeepsExplicitKind(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	const ns, name = "default", "kindinfer-explicit"

	_ = client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
	t.Cleanup(func() {
		_ = client.CoreV1().ConfigMaps(ns).Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	body := fmt.Sprintf(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q,"namespace":%q},"data":{"k":"v"}}`, name, ns)
	status, respBody := postRaw(t,
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/namespaces/%s/configmaps", testPort, ns), body)
	if status != http.StatusCreated {
		t.Fatalf("POST configmap with apiVersion/kind: got %d %s, want 201", status, respBody)
	}
	if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatalf("get the created configmap: %v", err)
	}
}

func postRaw(t *testing.T, url, body string) (int, string) {
	t.Helper()
	return sendRaw(t, http.MethodPost, url, body)
}

func putRaw(t *testing.T, url, body string) (int, string) {
	t.Helper()
	return sendRaw(t, http.MethodPut, url, body)
}

func sendRaw(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer k8flare-dev-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}
