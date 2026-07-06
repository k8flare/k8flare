package apiserver_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestPVCBinding_SynchronousAndDefaultStorageClass verifies Phase 8's PV/PVC
// bind boundary condition end-to-end against a real wrangler dev stack (per
// CLAUDE.md rule 2, "actually run it" -- not just read from source): a
// PersistentVolumeClaim that omits spec.storageClassName is defaulted to
// "r2" and bound to a freshly created PersistentVolume synchronously, in
// the same HTTP round trip as its Create -- mirroring
// TestClusterIPAllocation_SynchronousAndNoDoubleAllocation's shape for
// Services (clusterip_test.go).
func TestPVCBinding_SynchronousAndDefaultStorageClass(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, "pvc-bind-test", metav1.DeleteOptions{})

	pvc, err := client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-bind-test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create pvc-bind-test: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(context.Background(), "pvc-bind-test", metav1.DeleteOptions{})
	})

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "r2" {
		t.Errorf("spec.storageClassName = %v, want \"r2\" (default)", pvc.Spec.StorageClassName)
	}
	if pvc.Spec.VolumeName == "" {
		t.Fatal("spec.volumeName is empty in the Create response -- binding is not synchronous")
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		t.Errorf("status.phase = %q, want %q", pvc.Status.Phase, corev1.ClaimBound)
	}

	pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get PersistentVolume %s: %v", pvc.Spec.VolumeName, err)
	}
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.Name != pvc.Name || pv.Spec.ClaimRef.Namespace != ns {
		t.Errorf("PV claimRef = %+v, want a reference back to %s/%s", pv.Spec.ClaimRef, ns, pvc.Name)
	}
	if pv.Spec.ClaimRef.UID != pvc.UID {
		t.Errorf("PV claimRef.uid = %s, want pvc.uid %s", pv.Spec.ClaimRef.UID, pvc.UID)
	}
	if pv.Status.Phase != corev1.VolumeBound {
		t.Errorf("PV status.phase = %q, want %q", pv.Status.Phase, corev1.VolumeBound)
	}
	if pv.Spec.CSI == nil {
		t.Fatal("PV spec.csi is nil, want an R2-backed CSI source")
	}
	if pv.Spec.CSI.Driver != "k8flare.com/r2" {
		t.Errorf("PV spec.csi.driver = %q, want \"k8flare.com/r2\"", pv.Spec.CSI.Driver)
	}
	prefix := pv.Spec.CSI.VolumeAttributes["prefix"]
	if !strings.HasPrefix(prefix, "pvc-"+string(pvc.UID)) {
		t.Errorf("PV spec.csi.volumeAttributes[prefix] = %q, want it derived from pvc uid %s", prefix, pvc.UID)
	}
}

// TestPVCBinding_ExplicitOtherClassStaysPending verifies a PVC naming a
// StorageClass other than "r2" is left unbound, matching a real cluster's
// behavior when no provisioner matches (pvcbind.go's package doc comment) --
// not silently forced onto R2 regardless of what the client asked for.
func TestPVCBinding_ExplicitOtherClassStaysPending(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, "pvc-other-class", metav1.DeleteOptions{})
	other := "some-other-class"

	pvc, err := client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-other-class"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &other,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create pvc-other-class: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(context.Background(), "pvc-other-class", metav1.DeleteOptions{})
	})

	if pvc.Spec.VolumeName != "" {
		t.Errorf("spec.volumeName = %q, want empty (no provisioner for class %q)", pvc.Spec.VolumeName, other)
	}
}

// TestPVCBinding_DeleteReleasesPersistentVolume verifies ReleasePersistentVolume:
// deleting a bound PVC deletes its PersistentVolume too (matching
// reclaimPolicy=Delete's end state -- pvcbind.go's package doc comment on
// what's deliberately NOT done: the underlying R2 objects are not purged,
// only the PV object itself).
func TestPVCBinding_DeleteReleasesPersistentVolume(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, "pvc-release-test", metav1.DeleteOptions{})

	pvc, err := client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-release-test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create pvc-release-test: %v", err)
	}
	volumeName := pvc.Spec.VolumeName
	if volumeName == "" {
		t.Fatal("pvc-release-test never got bound -- can't test release")
	}

	if err := client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, "pvc-release-test", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete pvc-release-test: %v", err)
	}

	_, err = client.CoreV1().PersistentVolumes().Get(ctx, volumeName, metav1.GetOptions{})
	if err == nil {
		t.Errorf("PersistentVolume %s still exists after its claim was deleted", volumeName)
	} else if !errors.IsNotFound(err) {
		t.Errorf("Get PersistentVolume %s after claim delete: want NotFound, got %v", volumeName, err)
	}
}

// TestStorageClassBootstrap_R2ExistsAndIsDefault verifies
// BootstrapStorageClasses ran: kubectl (or any client) can discover a real
// "r2" StorageClass object, marked as this cluster's default, not just an
// implied name PVCs happen to default to (pvcbind.go's bindPersistentVolumeClaim
// never actually reads this object -- see its doc comment -- so this test
// exists specifically to catch the bootstrap wiring in
// workers/apiserver/main.go regressing independently of bind behavior).
func TestStorageClassBootstrap_R2ExistsAndIsDefault(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()

	sc, err := client.StorageV1().StorageClasses().Get(ctx, "r2", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get StorageClass r2: %v", err)
	}
	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] != "true" {
		t.Errorf("r2 StorageClass annotations = %v, want is-default-class=true", sc.Annotations)
	}
	if sc.Provisioner != "k8flare.com/r2" {
		t.Errorf("r2 StorageClass provisioner = %q, want \"k8flare.com/r2\"", sc.Provisioner)
	}
}

// TestMintR2Credentials_ReturnsWellFormedScopedCredential exercises
// /internal/mint-r2-credentials end to end against the real running
// Worker/DO stack: bind a PVC, ask for credentials by namespace+claimName
// (not by bucket/prefix -- workers/nodes never needs to understand the PV's
// CSI attribute schema, see r2handlers.go's doc comment), and check the
// returned session token decodes to a JWT scoped to exactly this PVC's
// prefix. This is deliberately a second, independent check of the JWT
// shape beyond r2_test.go's fixed-vector unit test: that one proves
// signR2JWT itself is spec-correct in isolation, this one proves the
// bind -> lookup -> mint wiring through real HTTP/DO storage produces the
// same result.
func TestMintR2Credentials_ReturnsWellFormedScopedCredential(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, "pvc-mint-test", metav1.DeleteOptions{})
	pvc, err := client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-mint-test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create pvc-mint-test: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(context.Background(), "pvc-mint-test", metav1.DeleteOptions{})
	})
	if pvc.Spec.VolumeName == "" {
		t.Fatal("pvc-mint-test never got bound -- can't test credential minting")
	}

	reqBody, _ := json.Marshal(map[string]any{
		"namespace": ns,
		"claimName": "pvc-mint-test",
	})
	// The consolidated Worker exposes /internal/* to the outside only
	// behind the cluster token (in-Worker callers bypass the public
	// handler entirely) -- so this external-client test authenticates.
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/internal/mint-r2-credentials", testPort), bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer k8flare-dev-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /internal/mint-r2-credentials: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /internal/mint-r2-credentials: status %d", resp.StatusCode)
	}

	var cred struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		SessionToken    string `json:"sessionToken"`
		Bucket          string `json:"bucket"`
		Endpoint        string `json:"endpoint"`
		Prefix          string `json:"prefix"`
		ExpiresAt       string `json:"expiresAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cred); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for name, v := range map[string]string{
		"accessKeyId": cred.AccessKeyID, "secretAccessKey": cred.SecretAccessKey,
		"sessionToken": cred.SessionToken, "bucket": cred.Bucket,
		"endpoint": cred.Endpoint, "prefix": cred.Prefix, "expiresAt": cred.ExpiresAt,
	} {
		if v == "" {
			t.Errorf("response field %q is empty", name)
		}
	}
	if !strings.HasPrefix(cred.Prefix, "pvc-"+string(pvc.UID)) {
		t.Errorf("prefix = %q, want it derived from pvc uid %s", cred.Prefix, pvc.UID)
	}
	if len(cred.SecretAccessKey) != 64 { // lowercase hex SHA-256 digest
		t.Errorf("secretAccessKey has length %d, want 64 (hex SHA-256)", len(cred.SecretAccessKey))
	}

	// sessionToken is base64("jwt/" + <jwt>) -- decode it and check the
	// payload's "paths.prefixPaths" names exactly this PVC's prefix, not
	// the whole bucket and not some other PVC's prefix.
	rawToken, err := base64.StdEncoding.DecodeString(cred.SessionToken)
	if err != nil {
		t.Fatalf("decode sessionToken as base64: %v", err)
	}
	jwt := strings.TrimPrefix(string(rawToken), "jwt/")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("decoded sessionToken %q is not a 3-part JWT", jwt)
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload segment: %v", err)
	}
	var payload struct {
		Bucket string `json:"bucket"`
		Scope  string `json:"scope"`
		Paths  struct {
			PrefixPaths []string `json:"prefixPaths"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("unmarshal JWT payload: %v", err)
	}
	if payload.Bucket != cred.Bucket {
		t.Errorf("JWT payload bucket = %q, want %q", payload.Bucket, cred.Bucket)
	}
	if payload.Scope != "object-read-write" {
		t.Errorf("JWT payload scope = %q, want \"object-read-write\"", payload.Scope)
	}
	if len(payload.Paths.PrefixPaths) != 1 || payload.Paths.PrefixPaths[0] != cred.Prefix {
		t.Errorf("JWT payload paths.prefixPaths = %v, want exactly [%q]", payload.Paths.PrefixPaths, cred.Prefix)
	}
}

// TestMintR2Credentials_UnboundClaimIsRejected verifies the endpoint
// refuses to mint a credential for a claim with no bound volume yet,
// instead of minting a credential scoped to an empty ("" -- whole bucket)
// prefix, which would be a much larger blast radius than the caller asked
// for.
func TestMintR2Credentials_UnboundClaimIsRejected(t *testing.T) {
	client := setupWranglerDev(t)
	ctx := context.Background()
	ns := "default"

	_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, "pvc-unbound-test", metav1.DeleteOptions{})
	other := "some-other-class" // never dynamically provisioned -- see TestPVCBinding_ExplicitOtherClassStaysPending
	_, err := client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-unbound-test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &other,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create pvc-unbound-test: %v", err)
	}
	t.Cleanup(func() {
		_ = client.CoreV1().PersistentVolumeClaims(ns).Delete(context.Background(), "pvc-unbound-test", metav1.DeleteOptions{})
	})

	reqBody, _ := json.Marshal(map[string]any{"namespace": ns, "claimName": "pvc-unbound-test"})
	// The consolidated Worker exposes /internal/* to the outside only
	// behind the cluster token (in-Worker callers bypass the public
	// handler entirely) -- so this external-client test authenticates.
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/internal/mint-r2-credentials", testPort), bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer k8flare-dev-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /internal/mint-r2-credentials: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("minted a credential for an unbound claim, want a non-200 rejection (got 200)")
	}
}
