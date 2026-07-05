package apiserver

import (
	"context"
	"errors"
	"log"
	"sync"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This file is Phase 8's PV/PVC bind mechanism: the R2-backed equivalent of
// clusterip.go's AssignClusterIP/ReleaseClusterIP and nodecidr.go's
// AssignPodCIDR/ReleasePodCIDR "synchronous allocation" pattern, adapted to
// a two-object (PersistentVolume + PersistentVolumeClaim) relationship that
// pattern doesn't quite fit unmodified -- see bindPersistentVolumeClaim's
// doc comment for exactly where and why this deviates.
//
// # Why this hooks ApplyPostCreateEffects, not the pre-Create mutation slot
//
// AssignClusterIP/AssignPodCIDR run in handler.go's POST case BEFORE
// store.Create, because all they do is mutate fields on the very object
// being created (svc.Spec.ClusterIP, node.Spec.PodCIDR) from an
// independent, already-existing address pool. Binding a PVC is different:
// it requires *creating a second, separate resource* (a PersistentVolume)
// that references the PVC's own namespace/name/uid -- which don't exist
// until the PVC itself has actually been persisted (uid in particular is
// assigned inside store.Create, via uuid.NewUUID()). serviceaccount.go's
// ApplyPostCreateEffects hook (originally written for
// Namespace -> default ServiceAccount + kube-root-ca.crt ConfigMap, which is
// the exact same shape: "create other resources as a side effect of this
// one's creation") already exists for precisely this kind of dependency, so
// PVC bind is added as another case in its switch rather than forcing PV
// creation into a slot designed for same-object mutation.
//
// One useful side effect of this choice: ApplyPostCreateEffects runs before
// handler.go's writeRuntimeObject(w, http.StatusCreated, obj) -- and since
// bindPersistentVolumeClaim mutates the same *corev1.PersistentVolumeClaim
// pointer handler.go already holds (and additionally persists those
// mutations via its own store.Update call), a client's PVC create response
// reflects the bound state in the same HTTP round trip, the same
// "synchronous" UX AssignClusterIP/AssignPodCIDR give Services/Nodes --
// just reached through a different hook.
//
// # What's deliberately NOT implemented in v1
//
//   - Only one StorageClass ("r2") is ever dynamically provisioned; a PVC
//     naming any other class (or a class that doesn't exist) is left
//     Pending, matching a real cluster's behavior when no provisioner
//     matches -- see bindPersistentVolumeClaim.
//   - VolumeBindingMode is always Immediate (bind on PVC create); there is
//     no WaitForFirstConsumer support (no scheduler-driven topology
//     decision could change which R2 prefix a Pod should use -- there is
//     only ever one, unconditional prefix per PVC either way).
//   - ReleasePersistentVolume deletes the PV object when its PVC is
//     deleted (matching "reclaimPolicy=Delete"'s end state), but does NOT
//     delete the underlying R2 objects under the freed prefix. Real
//     dynamic provisioning's actual data deletion is always done by an
//     out-of-tree external provisioner's DeleteVolume call -- there's no
//     upstream Go package for "delete objects in a CSI backend" to reuse
//     (CLAUDE.md rule 3), and implementing it here would mean hand-rolling
//     AWS SigV4 request signing plus S3 XML response parsing purely for
//     this one cleanup path. Left as documented future work (see
//     docs/cost-model.md's R2 section); R2 objects under a released
//     prefix are simply orphaned (billed as storage until manually
//     deleted) until that lands.

// R2StorageClassName is the one StorageClass this project dynamically
// provisions against in v1 -- see BootstrapStorageClasses.
const R2StorageClassName = "r2"

// R2CSIDriverName is a synthetic CSI driver name stashed on every R2-backed
// PersistentVolume's spec.csi.driver. No real CSI sidecar/plugin registers
// under this name anywhere in this project (binding happens synchronously
// in Go below, not through the real CSI protocol) -- it exists so the
// field holds a real, discoverable value (`kubectl get pv -o yaml` shows
// something meaningful) rather than an empty string, and so
// workers/nodes/src/virtualnode.ts has a stable driver name to assert on
// before trusting a PV's volumeAttributes.
const R2CSIDriverName = "k8flare.com/r2"

// r2AttrBucket and r2AttrPrefix are the spec.csi.volumeAttributes keys a
// bound PersistentVolume carries its R2 location under. workers/nodes reads
// these (indirectly -- see the /internal/mint-r2-credentials handler below,
// which is the only reader in v1) to know which bucket/prefix a Pod's
// mounted PVC maps to.
const (
	r2AttrBucket = "bucket"
	r2AttrPrefix = "prefix"
)

var storageClassBootstrapOnce sync.Once

// BootstrapStorageClasses creates the "r2" StorageClass, marked as this
// cluster's default (the same `storageclass.kubernetes.io/is-default-class`
// annotation a real cluster's admission controller sets), if it doesn't
// already exist. Mirrors BootstrapCluster's (bootstrap.go) idempotent,
// once-per-Worker-instance pattern, kept as a separate function+sync.Once
// rather than folded into BootstrapCluster itself: BootstrapCluster is only
// ever called from the core/v1 registration branch in
// workers/apiserver/main.go, and storage.k8s.io/v1 (where StorageClass
// lives) is a different GroupVersion with its own registration branch --
// see main.go's isStorage branch.
func BootstrapStorageClasses(ctx context.Context, stores map[string]*ResourceStore) {
	storageClassBootstrapOnce.Do(func() {
		scStore := stores["storageclasses"]
		if scStore == nil {
			return // defensive; always present once apidef.Table lists storageclasses
		}
		reclaim := corev1.PersistentVolumeReclaimDelete
		binding := storagev1.VolumeBindingImmediate
		_, err := scStore.Create(ctx, "", &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:        R2StorageClassName,
				Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"},
			},
			Provisioner:       R2CSIDriverName,
			ReclaimPolicy:     &reclaim,
			VolumeBindingMode: &binding,
		})
		if err != nil {
			var se *StatusError
			if errors.As(err, &se) && se.Status.Reason == metav1.StatusReasonAlreadyExists {
				return
			}
			log.Printf("failed to bootstrap %q StorageClass: %v", R2StorageClassName, err)
		}
	})
}

// bindPersistentVolumeClaim provisions and binds a PersistentVolume for pvc
// if pvc's (possibly defaulted) StorageClass is "r2" and it doesn't already
// have one. Called from ApplyPostCreateEffects (serviceaccount.go), i.e.
// after pvc has already been persisted once (so it has a real UID) --
// see this file's package doc comment for why.
//
// The R2 prefix a PV gets ("pvc-<uid>/") is derived from the PVC's UID, not
// its namespace/name: UID is assigned fresh by every Create
// (uuid.NewUUID(), store.go), so a deleted-and-recreated PVC of the same
// name can never be handed a prefix an earlier, unrelated PVC (possibly
// belonging to a different tenant, if namespaces are ever reused across
// tenants) already wrote data under.
func bindPersistentVolumeClaim(ctx context.Context, stores map[string]*ResourceStore, pvc *corev1.PersistentVolumeClaim) {
	if pvc.Spec.VolumeName != "" {
		return // already bound -- shouldn't happen on a fresh create, but idempotent-safe
	}

	className := R2StorageClassName
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		className = *pvc.Spec.StorageClassName
	} else {
		// No class requested: default to "r2", the only class this project
		// dynamically provisions, and record the default onto the PVC
		// itself -- matching a real cluster's default-storage-class
		// admission plugin, which fills this field in rather than leaving
		// it nil.
		pvc.Spec.StorageClassName = &className
	}
	if className != R2StorageClassName {
		// Not this project's class: leave Pending, same as a real cluster
		// when no provisioner matches -- see this file's package doc
		// comment.
		return
	}

	pvStore := stores["persistentvolumes"]
	pvcStore := stores["persistentvolumeclaims"]
	if pvStore == nil || pvcStore == nil {
		return // defensive; both always present once apidef.Table lists them
	}

	cfg := currentR2Config()

	prefix := "pvc-" + string(pvc.UID) + "/"
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-" + string(pvc.UID)},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: pvc.Spec.Resources.Requests[corev1.ResourceStorage]},
			AccessModes:                   accessModesOrDefault(pvc.Spec.AccessModes),
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			StorageClassName:              R2StorageClassName,
			ClaimRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Namespace:  pvc.Namespace,
				Name:       pvc.Name,
				UID:        pvc.UID,
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       R2CSIDriverName,
					VolumeHandle: prefix,
					VolumeAttributes: map[string]string{
						r2AttrBucket: cfg.Bucket,
						r2AttrPrefix: prefix,
					},
				},
			},
		},
	}

	createdObj, err := pvStore.Create(ctx, "", pv)
	if err != nil {
		log.Printf("bind PVC %s/%s: create PersistentVolume: %v", pvc.Namespace, pvc.Name, err)
		return
	}
	createdPV := createdObj.(*corev1.PersistentVolume)

	// Re-GET before the follow-up status Update, rather than reusing
	// createdPV (the pointer Create just mutated in place) directly: Create
	// stamped it with metav1.Now(), which carries sub-second precision, but
	// metav1.Time's JSON encoding truncates to whole seconds -- so the
	// in-memory CreationTimestamp and the value that actually round-tripped
	// through kine storage disagree below the second, and Update's
	// immutable-field check (store.go) rejects the update as a (spurious)
	// attempt to change creationTimestamp. handleStatusSubresource
	// (subresource.go) and workers/nodes/src/virtualnode.ts's
	// ensureNodeRegistered both already avoid this the same way: fetch
	// fresh, mutate the fresh copy, Update that -- found here by actually
	// running this against wrangler dev (CLAUDE.md rule 2), not by reading
	// either side's code first: the Update call below returned no error
	// before this fix either, because failures here are deliberately
	// logged, not surfaced -- the symptom was PersistentVolume.status.phase
	// silently staying empty forever.
	freshPVObj, err := pvStore.Get(ctx, "", createdPV.Name)
	if err != nil {
		log.Printf("bind PVC %s/%s: re-fetch PersistentVolume %s: %v", pvc.Namespace, pvc.Name, createdPV.Name, err)
	} else {
		freshPV := freshPVObj.(*corev1.PersistentVolume)
		freshPV.Status.Phase = corev1.VolumeBound
		updatedObj, err := pvStore.Update(ctx, "", freshPV.Name, freshPV)
		if err != nil {
			// Non-fatal: the PV exists and is already ClaimRef'd, which is
			// what actually matters for the PVC bind below -- .status.phase
			// is cosmetic (nothing in this project's read paths branches on
			// it).
			log.Printf("bind PVC %s/%s: set PersistentVolume %s status: %v", pvc.Namespace, pvc.Name, createdPV.Name, err)
		} else {
			createdPV = updatedObj.(*corev1.PersistentVolume)
		}
	}

	// Same re-GET-before-Update reasoning as above: pvc is the exact
	// pointer handler.go's POST case got back from store.Create and will
	// serialize as this request's HTTP response, so its fields are mutated
	// directly (giving the client the bound state in the same round trip --
	// see this file's package doc comment) -- but the Update call itself
	// targets a freshly-fetched copy, whose CreationTimestamp actually
	// matches what's in storage.
	freshPVCObj, err := pvcStore.Get(ctx, pvc.Namespace, pvc.Name)
	if err != nil {
		log.Printf("bind PVC %s/%s: re-fetch claim: %v", pvc.Namespace, pvc.Name, err)
		return
	}
	freshPVC := freshPVCObj.(*corev1.PersistentVolumeClaim)
	freshPVC.Spec.StorageClassName = pvc.Spec.StorageClassName
	freshPVC.Spec.VolumeName = createdPV.Name
	freshPVC.Status.Phase = corev1.ClaimBound
	freshPVC.Status.AccessModes = createdPV.Spec.AccessModes
	freshPVC.Status.Capacity = createdPV.Spec.Capacity
	updatedPVCObj, err := pvcStore.Update(ctx, pvc.Namespace, pvc.Name, freshPVC)
	if err != nil {
		log.Printf("bind PVC %s/%s: update claim: %v", pvc.Namespace, pvc.Name, err)
		return
	}
	updatedPVC := updatedPVCObj.(*corev1.PersistentVolumeClaim)

	// Reflect the persisted result back onto the caller's pvc pointer so
	// handler.go's HTTP response (which serializes pvc, not freshPVC/
	// updatedPVC) shows the bound state synchronously too.
	pvc.Spec.VolumeName = updatedPVC.Spec.VolumeName
	pvc.Spec.StorageClassName = updatedPVC.Spec.StorageClassName
	pvc.Status = updatedPVC.Status
	pvc.ResourceVersion = updatedPVC.ResourceVersion
}

// accessModesOrDefault returns modes unchanged if non-empty, or
// ReadWriteMany if empty. Real kube-apiserver requires spec.accessModes to
// be non-empty (API validation this project's admission-free apiserver
// doesn't enforce -- see scheme.go's comment on having no admission chain),
// so an empty value reaching here is a client that skipped that
// validation, not a real "no preference" case; ReadWriteMany is the
// closest true statement about R2 either way (object storage has no
// single-writer/block-device exclusivity to model -- unlike a real block
// or file CSI volume, any number of Pods can concurrently read and write
// distinct keys under the same prefix).
func accessModesOrDefault(modes []corev1.PersistentVolumeAccessMode) []corev1.PersistentVolumeAccessMode {
	if len(modes) > 0 {
		return modes
	}
	return []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
}

// ReleasePersistentVolume deletes pvc's bound PersistentVolume, if it has
// one. Called from handler.go's DELETE cases, mirroring
// ReleaseClusterIP/ReleasePodCIDR's placement and best-effort/logged (not
// client-visible) error handling -- by the time this runs the PVC delete
// itself has already succeeded, so there's nothing left for a client retry
// to roll back to.
//
// This does NOT delete the underlying R2 objects under the freed prefix --
// see this file's package doc comment for why that's out of scope for v1.
func ReleasePersistentVolume(ctx context.Context, stores map[string]*ResourceStore, pvc *corev1.PersistentVolumeClaim) {
	if pvc.Spec.VolumeName == "" {
		return
	}
	pvStore := stores["persistentvolumes"]
	if pvStore == nil {
		return
	}
	if _, err := pvStore.Delete(ctx, "", pvc.Spec.VolumeName); err != nil {
		log.Printf("release PVC %s/%s: delete PersistentVolume %s: %v", pvc.Namespace, pvc.Name, pvc.Spec.VolumeName, err)
	}
}
