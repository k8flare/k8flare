package apiserver_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Mirrors of pkg/controllers/clusterop's constants. That package is
// //go:build js && wasm (it only ever runs inside a dynamic worker), so a
// host test cannot import it -- and duplicating three string literals is
// cheaper than a shared build-tag-free package would be. A drift here
// shows up immediately as this test failing.
const (
	clusteropSecretNamespace  = "k8flare-system"
	clusteropRotateAnnotation = "k8flare.com/rotate-token"
	clusteropDefaultCluster   = "default"
)

// TestClusterOperatorLifecycle drives the cluster operator
// (pkg/controllers/clusterop) end to end against a REAL clusterop dynamic
// worker: create a Cluster object, watch it become Ready with a usable
// token, rotate that token, then delete it and watch the finalizer take
// the whole Durable Object tree down with it.
//
// Opt-in via K8FLARE_CLUSTEROP_TEST=1 (`make test-clusterop`), for the
// same reason as TestKCMDynamicWorkerControlPlane: it boots its own
// wrangler dev on a fresh port with a throwaway state dir, and the first
// poke compiles a ~32MB WASM module inside workerd. The workload
// controllers and the scheduler are switched off (CM_DISABLED/
// SCHED_DISABLED) -- this test provisions whole clusters and has no pods
// for them to reconcile, so loading them would only compete for the same
// isolate budget.
func TestClusterOperatorLifecycle(t *testing.T) {
	if os.Getenv("K8FLARE_CLUSTEROP_TEST") != "1" {
		t.Skip("cluster-operator lifecycle is opt-in: set K8FLARE_CLUSTEROP_TEST=1 (make test-clusterop)")
	}

	port := findFreePort(t)
	projectRoot := findProjectRoot(t)

	// The operator's own console output (pkg/controllers/clusterop's logf,
	// which reaches here through wasm_exec.js's globalThis.fs shim) is the
	// only view into why a reconcile stalled, and this harness discarded it
	// twice over: to os.DevNull, and via "--log-level error", which
	// suppresses wrangler's console forwarding. Opt in with
	// K8FLARE_DEV_LOG=<path> to get both back; the default stays quiet so a
	// passing run is not noisy.
	//
	// The level must be "log", not "info": wrangler orders its levels
	// debug > log > info > warn > error, and a Worker's console.log lands
	// at "log" -- so "info" still drops every console line while keeping
	// the request log, which is exactly what it looked like at first
	// (measured 2026-07-28: request lines present, zero console lines).
	logLevel := "error"
	sink := io.Discard
	if path := os.Getenv("K8FLARE_DEV_LOG"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create K8FLARE_DEV_LOG %s: %v", path, err)
		}
		t.Cleanup(func() { f.Close() })
		t.Logf("wrangler dev output -> %s", path)
		sink = f
		logLevel = "log"
	}

	cmd := exec.Command("npx", "wrangler", "dev",
		"-c", "packages/k8flare-worker/wrangler.jsonc",
		"--enable-containers=false",
		"--local",
		"--port", fmt.Sprintf("%d", port),
		"--persist-to", t.TempDir(),
		"--var", "CM_DISABLED:1",
		"--var", "SCHED_DISABLED:1",
		"--log-level", logLevel,
	)
	cmd.Dir = projectRoot
	cmd.Stdout = sink
	cmd.Stderr = sink
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start wrangler dev: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	})

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			time.Sleep(2 * time.Second)
			break
		}
		time.Sleep(time.Second)
	}

	host := fmt.Sprintf("http://127.0.0.1:%d", port)
	cfg := &rest.Config{Host: host, BearerToken: "k8flare-dev-token"}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("dynamic client: %v", err)
	}
	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("typed client: %v", err)
	}
	clusters := dc.Resource(clustersGVR)
	ctx := context.Background()

	name := fmt.Sprintf("op-%d", time.Now().Unix())
	t.Cleanup(func() {
		_ = clusters.Delete(context.Background(), name, metav1.DeleteOptions{})
	})

	if _, err := clusters.Create(ctx, newCluster(name, "operator lifecycle"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	// Provisioning. The first poke includes the in-workerd WASM compile,
	// so the budget is generous.
	var doName string
	waitFor(t, 5*time.Minute, "cluster reaches phase Ready", func() bool {
		got, err := clusters.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		phase, _, _ := unstructured.NestedString(got.Object, "status", "phase")
		doName, _, _ = unstructured.NestedString(got.Object, "status", "doName")
		return phase == "Ready" && doName != ""
	})
	if want := name + "@"; len(doName) <= len(want) || doName[:len(want)] != want {
		t.Errorf("doName %q is not <name>@<uid>", doName)
	}

	secretName := "cluster-" + name
	var token string
	waitFor(t, 2*time.Minute, "credential Secret published", func() bool {
		s, err := core.CoreV1().Secrets(clusteropSecretNamespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		token = string(s.Data["token"])
		return token != "" && len(s.Data["kubeconfig"]) > 0
	})

	// The minted token must actually work against the new cluster's own
	// URL space, and only there.
	waitFor(t, time.Minute, "minted token authenticates against /c/"+name, func() bool {
		return apiStatus(t, host+"/c/"+name+"/api/v1/namespaces", token) == http.StatusOK
	})
	if code := apiStatus(t, host+"/c/"+name+"/version", ""); code != http.StatusOK {
		t.Errorf("GET /c/%s/version = %d, want 200", name, code)
	}
	if code := apiStatus(t, host+"/c/"+name+"/api/v1/namespaces", "not-this-clusters-token"); code != http.StatusUnauthorized {
		t.Errorf("bogus token against /c/%s = %d, want 401", name, code)
	}

	// Diff guard (docs/cluster-api-design.md's "poke feedback
	// prevention"): a converged Cluster must stop being written. Without
	// the observedGeneration/phase check the operator's own status write
	// re-pokes the pump and the object's resourceVersion climbs forever.
	before, err := clusters.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	rv := before.GetResourceVersion()
	time.Sleep(45 * time.Second)
	after, err := clusters.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if after.GetResourceVersion() != rv {
		t.Errorf("converged cluster was rewritten: resourceVersion %s -> %s (reconcile loop)",
			rv, after.GetResourceVersion())
	}

	// Rotation: annotate, and the operator must mint a replacement,
	// republish the Secret, revoke the old token, and clear the
	// annotation.
	rotating := after.DeepCopy()
	annotations := rotating.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	rotateValue := fmt.Sprintf("%d", time.Now().UnixNano())
	annotations[clusteropRotateAnnotation] = rotateValue
	rotating.SetAnnotations(annotations)
	if _, err := clusters.Update(ctx, rotating, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("annotate for rotation: %v", err)
	}

	var rotated string
	waitFor(t, 3*time.Minute, "token rotated in the Secret", func() bool {
		s, err := core.CoreV1().Secrets(clusteropSecretNamespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		rotated = string(s.Data["token"])
		return rotated != "" && rotated != token
	})
	waitFor(t, 2*time.Minute, "rotate annotation cleared", func() bool {
		got, err := clusters.Get(ctx, name, metav1.GetOptions{})
		return err == nil && got.GetAnnotations()[clusteropRotateAnnotation] == ""
	})
	waitFor(t, 2*time.Minute, "rotated token authenticates", func() bool {
		return apiStatus(t, host+"/c/"+name+"/api/v1/namespaces", rotated) == http.StatusOK
	})
	// Revocation propagates within the verifier's isolate cache TTL (60s,
	// clusters/tokens.ts) -- a documented tradeoff, not an instant.
	waitFor(t, 3*time.Minute, "superseded token stops working", func() bool {
		return apiStatus(t, host+"/c/"+name+"/api/v1/namespaces", token) == http.StatusUnauthorized
	})

	// Replay: the SAME annotation value must be a no-op, not a second
	// rotation. The operator derives the vault token id deterministically
	// from the value (clusterop.rotationTokenID), so replaying it returns
	// the token already minted instead of stranding another permanently
	// valid one -- which is exactly what a rotation that failed partway
	// and got retried looks like.
	replaying, err := clusters.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cluster for rotation replay: %v", err)
	}
	replayAnnotations := replaying.GetAnnotations()
	if replayAnnotations == nil {
		replayAnnotations = map[string]string{}
	}
	replayAnnotations[clusteropRotateAnnotation] = rotateValue
	replaying.SetAnnotations(replayAnnotations)
	if _, err := clusters.Update(ctx, replaying, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("re-annotate with the same rotation value: %v", err)
	}
	waitFor(t, 2*time.Minute, "replayed rotate annotation cleared", func() bool {
		got, err := clusters.Get(ctx, name, metav1.GetOptions{})
		return err == nil && got.GetAnnotations()[clusteropRotateAnnotation] == ""
	})
	s, err := core.CoreV1().Secrets(clusteropSecretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret after rotation replay: %v", err)
	}
	if got := string(s.Data["token"]); got != rotated {
		t.Errorf("rotation replay minted a NEW token (secret token changed) -- rotation is not idempotent")
	}
	// And only that one token is valid: the replay must not have
	// resurrected the superseded one.
	if code := apiStatus(t, host+"/c/"+name+"/api/v1/namespaces", rotated); code != http.StatusOK {
		t.Errorf("rotated token after replay = %d, want 200", code)
	}
	if code := apiStatus(t, host+"/c/"+name+"/api/v1/namespaces", token); code != http.StatusUnauthorized {
		t.Errorf("superseded token after replay = %d, want 401", code)
	}

	// Teardown: the finalizer holds the object until the DO cascade and
	// the Secret are actually gone.
	if err := clusters.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}
	waitFor(t, 3*time.Minute, "cluster object fully deleted", func() bool {
		_, err := clusters.Get(ctx, name, metav1.GetOptions{})
		return err != nil
	})
	waitFor(t, 2*time.Minute, "cluster no longer resolves", func() bool {
		return apiStatus(t, host+"/c/"+name+"/version", "") == http.StatusNotFound
	})
	if _, err := core.CoreV1().Secrets(clusteropSecretNamespace).Get(ctx, secretName, metav1.GetOptions{}); err == nil {
		t.Errorf("credential Secret %s survived cluster deletion", secretName)
	}

	// The management cluster's own Cluster object is seeded by the
	// operator at startup, so it must exist by now -- and it must NOT have
	// been provisioned as a tenant (its DO tree is the pre-existing
	// "default", with no uid suffix).
	def, err := clusters.Get(ctx, clusteropDefaultCluster, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("default Cluster was not seeded: %v", err)
	}
	if got, _, _ := unstructured.NestedString(def.Object, "status", "doName"); got != "" && got != "default" {
		t.Errorf("default cluster doName = %q, want \"default\"", got)
	}

	// Rotating the MANAGEMENT cluster is a terminal no-op, not a retry
	// loop: its credential is the K3S_TOKEN root secret, which the vault
	// endpoint refuses to rotate (409). The operator must clear the
	// annotation and say why in a condition rather than requeue forever.
	rotatingDefault := def.DeepCopy()
	defAnnotations := rotatingDefault.GetAnnotations()
	if defAnnotations == nil {
		defAnnotations = map[string]string{}
	}
	defAnnotations[clusteropRotateAnnotation] = fmt.Sprintf("%d", time.Now().UnixNano())
	rotatingDefault.SetAnnotations(defAnnotations)
	if _, err := clusters.Update(ctx, rotatingDefault, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("annotate default cluster for rotation: %v", err)
	}
	waitFor(t, 2*time.Minute, "default cluster rotate annotation cleared", func() bool {
		got, err := clusters.Get(ctx, clusteropDefaultCluster, metav1.GetOptions{})
		return err == nil && got.GetAnnotations()[clusteropRotateAnnotation] == ""
	})
	waitFor(t, 2*time.Minute, "default cluster reports RotateUnsupported", func() bool {
		got, err := clusters.Get(ctx, clusteropDefaultCluster, metav1.GetOptions{})
		if err != nil {
			return false
		}
		conds, _, _ := unstructured.NestedSlice(got.Object, "status", "conditions")
		for _, raw := range conds {
			c, ok := raw.(map[string]interface{})
			if ok && c["type"] == "RotateUnsupported" && c["status"] == "True" {
				return true
			}
		}
		return false
	})
}

// apiStatus issues a GET and returns the status code, treating a
// transport failure as 0 so pollers can retry rather than fail.
func apiStatus(t *testing.T, url, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
