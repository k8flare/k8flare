package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// RegisterR2Handlers registers R2 PVC-credential routes meant only for
// other Workers in this project to call via a Cloudflare service binding
// (not for kubectl or any other public client) -- the same
// no-AuthMiddleware-because-the-binding-itself-is-the-trust-boundary
// reasoning as RegisterInternalHandlers (nodelifecycle.go): a service
// binding is only reachable by a Worker this account explicitly wired a
// "services" binding to. The caller here is workers/nodes (see
// workers/nodes/src/client.ts's mintR2Credentials), reached over its own
// APISERVER service binding, the same way workers/storage already reaches
// /internal/reconcile-node-lifecycle over its CONTROLLERS binding.
//
// Kept in its own registration function (not folded into
// RegisterInternalHandlers itself) so nodelifecycle.go doesn't gain a
// PVC/PV-specific dependency (coreStores) it has no other reason to need.
func RegisterR2Handlers(mux *http.ServeMux, coreStores map[string]*ResourceStore) {
	mux.HandleFunc("POST /internal/mint-r2-credentials", func(w http.ResponseWriter, r *http.Request) {
		handleMintR2Credentials(w, r, coreStores)
	})
}

// mintR2CredentialsRequest is this endpoint's request body.
// workers/nodes sends the Pod's PVC reference (namespace + claim name), not
// a bucket/prefix directly -- this endpoint alone owns the mapping from
// "which PVC" to "which R2 prefix" (via the bound PersistentVolume's
// spec.csi.volumeAttributes), so workers/nodes never needs to understand
// that CSI attribute schema at all.
type mintR2CredentialsRequest struct {
	Namespace  string `json:"namespace"`
	ClaimName  string `json:"claimName"`
	TTLSeconds int64  `json:"ttlSeconds"` // 0 means DefaultCredentialTTL
}

// mintR2CredentialsResponse mirrors R2Credential's fields; kept as its own
// type (rather than reusing R2Credential's json tags directly) so this
// HTTP contract can evolve independently of that internal struct's shape.
type mintR2CredentialsResponse struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey string    `json:"secretAccessKey"`
	SessionToken    string    `json:"sessionToken"`
	Bucket          string    `json:"bucket"`
	Endpoint        string    `json:"endpoint"`
	Prefix          string    `json:"prefix"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

func handleMintR2Credentials(w http.ResponseWriter, r *http.Request, coreStores map[string]*ResourceStore) {
	var req mintR2CredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" || req.ClaimName == "" {
		http.Error(w, "namespace and claimName are required", http.StatusBadRequest)
		return
	}
	ttl := DefaultCredentialTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	cfg := currentR2Config()

	ctx := r.Context()
	pvcStore := coreStores["persistentvolumeclaims"]
	pvStore := coreStores["persistentvolumes"]
	if pvcStore == nil || pvStore == nil {
		writeInternalError(w, fmt.Errorf("persistentvolumeclaims/persistentvolumes stores are not registered"))
		return
	}

	pvcObj, err := pvcStore.Get(ctx, req.Namespace, req.ClaimName)
	if err != nil {
		writeResourceError(w, err, "persistentvolumeclaims", req.ClaimName)
		return
	}
	pvc, ok := pvcObj.(*corev1.PersistentVolumeClaim)
	if !ok {
		writeInternalError(w, fmt.Errorf("persistentvolumeclaims store returned %T, expected *corev1.PersistentVolumeClaim", pvcObj))
		return
	}
	if pvc.Spec.VolumeName == "" {
		http.Error(w, fmt.Sprintf("PersistentVolumeClaim %s/%s is not yet bound", req.Namespace, req.ClaimName), http.StatusConflict)
		return
	}

	pvObj, err := pvStore.Get(ctx, "", pvc.Spec.VolumeName)
	if err != nil {
		writeResourceError(w, err, "persistentvolumes", pvc.Spec.VolumeName)
		return
	}
	pv, ok := pvObj.(*corev1.PersistentVolume)
	if !ok {
		writeInternalError(w, fmt.Errorf("persistentvolumes store returned %T, expected *corev1.PersistentVolume", pvObj))
		return
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != R2CSIDriverName {
		http.Error(w, fmt.Sprintf("PersistentVolume %s is not an R2-backed volume", pv.Name), http.StatusBadRequest)
		return
	}
	prefix := pv.Spec.CSI.VolumeAttributes[r2AttrPrefix]

	cred, err := MintCredential(cfg, prefix, ttl)
	if err != nil {
		writeInternalError(w, fmt.Errorf("mint r2 credential: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, mintR2CredentialsResponse{
		AccessKeyID:     cred.AccessKeyID,
		SecretAccessKey: cred.SecretAccessKey,
		SessionToken:    cred.SessionToken,
		Bucket:          cred.Bucket,
		Endpoint:        cred.Endpoint,
		Prefix:          cred.Prefix,
		ExpiresAt:       cred.ExpiresAt,
	})
}
