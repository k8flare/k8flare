package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"

	corev1 "k8s.io/api/core/v1"
	k8sallocator "k8s.io/kubernetes/pkg/registry/core/service/allocator"
	netutils "k8s.io/utils/net"
)

// clusterIPRangeKey is the kine storage key the ClusterIP allocation bitmap
// is persisted under -- the same key real kube-apiserver uses in etcd for
// this (pkg/registry/core/rangeallocation), just against this project's
// kine-over-DO storage instead.
const clusterIPRangeKey = "/ranges/serviceips"

// addressesReservedForFutureServices mirrors the reservation
// workers/storage/src/serviceip.ts makes via FIRST_ALLOCATABLE_INDEX:
// offset 1 for the future "kubernetes.default" Service (10.43.0.1, the
// conventional first address in a Service range), offset 10 for "kube-dns"
// (10.43.0.10). Neither is provisioned yet, but a freshly initialized range
// blocks them out up front so a real allocation can never collide once
// they are.
var addressesReservedForFutureServices = []int{1, 10}

// AssignClusterIP allocates and sets spec.clusterIP (and spec.clusterIPs)
// on svc if it needs one, synchronously, before it's ever written to
// storage -- called from handler.go's POST case, the same place
// ApplyDefaults runs. serviceNeedsClusterIP mirrors
// workers/storage/src/serviceip.ts's serviceNeedsClusterIP check exactly:
// ExternalName Services and explicit "None" (headless) Services never get
// one.
//
// workers/storage/src/serviceip.ts's allocateClusterIPs -- an
// alarm-triggered pass over every Service, kept as-is per this project's
// "don't touch workers/storage" boundary for this change -- runs on every
// Cluster DO alarm and would otherwise double-allocate a second ClusterIP
// for the same Service. It doesn't: it skips any Service whose
// spec.clusterIP is already non-empty (same serviceNeedsClusterIP-shaped
// check, TS side), and by the time a Service created here reaches storage
// its ClusterIP is already set. Verified end-to-end against a running
// wrangler dev stack, not just read from source -- see clusterip_test.go.
func AssignClusterIP(ctx context.Context, storage *Storage, svc *corev1.Service) error {
	if !serviceNeedsClusterIP(svc) {
		return nil
	}
	ip, err := ServiceIPAllocator(storage).AllocateNext(ctx)
	if err != nil {
		return err
	}
	svc.Spec.ClusterIP = ip.String()
	svc.Spec.ClusterIPs = []string{ip.String()}
	return nil
}

// serviceNeedsClusterIP reports whether svc should get an allocated
// ClusterIP: not ExternalName, and no ClusterIP set yet (an explicit
// "None" -- headless -- already counts as "set", so it's left alone).
func serviceNeedsClusterIP(svc *corev1.Service) bool {
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return false
	}
	return svc.Spec.ClusterIP == ""
}

// ServiceIPAllocator returns the ClusterIPAllocator for the cluster's
// Service address range (ServiceCIDR, supervisor.go). Constructing one is
// cheap -- no I/O happens until AllocateNext is called -- so building a
// fresh instance per call is simpler than threading a shared one through
// main.go.
func ServiceIPAllocator(storage *Storage) *ClusterIPAllocator {
	return NewClusterIPAllocator(storage, ServiceCIDR)
}

// ClusterIPAllocator hands out unique addresses from a CIDR range,
// persisting the allocation state as a single kine object with
// optimistic-concurrency (CAS) retries -- the same design real
// kube-apiserver uses against etcd
// (pkg/registry/core/service/ipallocator/controller): load the current
// bitmap snapshot with its storage revision, mutate, write back
// conditioned on that revision, retry from scratch on conflict.
//
// The actual bit allocation is upstream's real
// k8s.io/kubernetes/pkg/registry/core/service/allocator.AllocationBitmap --
// the same engine k8s.io/kubernetes/.../ipallocator.Range wraps for
// etcd-backed clusters -- and the IP<->offset math is upstream's real
// k8s.io/utils/net helpers (BigForIP/AddIPOffset/RangeSize), the same ones
// ipallocator.Range itself calls internally. This talks to them directly
// rather than going through ipallocator.Range: that package also pulls in
// client-go informers/listers/cache for its newer IPAddress/ServiceCIDR
// allocation mode (measured as a multi-MB addition to workers/apiserver's
// WASM binary for a mode this allocator never exercises -- see this
// project's final report / docs/cost-model.md for the measured gzip delta).
// The bitmap and IP math -- the actual hard part -- are 100% upstream code;
// only this coordinating layer, which ipallocator.Range also has (just
// wired to a client-go/etcd storage stack this project doesn't use), is
// project-specific.
type ClusterIPAllocator struct {
	storage *Storage
	cidr    *net.IPNet
	base    *big.Int
	max     int
}

// NewClusterIPAllocator creates a ClusterIPAllocator for cidr, persisting
// its allocation state through storage.
func NewClusterIPAllocator(storage *Storage, cidr *net.IPNet) *ClusterIPAllocator {
	return &ClusterIPAllocator{
		storage: storage,
		cidr:    cidr,
		base:    netutils.BigForIP(cidr.IP),
		max:     int(netutils.RangeSize(cidr)),
	}
}

// newBitmap creates an empty bitmap for a's range, with
// addressesReservedForFutureServices pre-allocated. Only used the very
// first time this range is ever allocated from (see load).
func (a *ClusterIPAllocator) newBitmap() *k8sallocator.AllocationBitmap {
	bitmap := k8sallocator.NewAllocationMap(a.max, a.cidr.String())
	for _, offset := range addressesReservedForFutureServices {
		bitmap.Allocate(offset) // fresh map: cannot fail
	}
	return bitmap
}

// clusterIPRangeSnapshot is the JSON envelope AllocationBitmap.Snapshot's
// (rangeSpec, data) pair is stored as. Deliberately not
// k8s.io/kubernetes/pkg/apis/core's RangeAllocation type (which Range.Snapshot
// upstream uses): that type exists for etcd storage via a generic registry
// with scheme-based internal<->versioned conversion, machinery this
// project's kine-over-DO Storage doesn't have or need for an object no API
// client ever sees.
type clusterIPRangeSnapshot struct {
	Range string `json:"range"`
	Data  []byte `json:"data"`
}

// load fetches the persisted bitmap snapshot -- or seeds a fresh one, with
// the reserved addresses pre-allocated, if this range has never been
// allocated from before -- along with the storage revision needed for a
// CAS write-back.
func (a *ClusterIPAllocator) load(ctx context.Context) (*k8sallocator.AllocationBitmap, int64, error) {
	stored, err := a.storage.Get(ctx, clusterIPRangeKey)
	if errors.Is(err, ErrNotFound) {
		return a.newBitmap(), 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("load clusterip range: %w", err)
	}

	var snap clusterIPRangeSnapshot
	if err := json.Unmarshal(stored.Value, &snap); err != nil {
		return nil, 0, fmt.Errorf("decode clusterip range: %w", err)
	}
	bitmap := k8sallocator.NewAllocationMap(a.max, a.cidr.String())
	if err := bitmap.Restore(snap.Range, snap.Data); err != nil {
		return nil, 0, fmt.Errorf("restore clusterip range: %w", err)
	}
	return bitmap, stored.ModRevision, nil
}

// save snapshots bitmap and CAS-writes it back at revision (0 meaning
// "create", matching Storage.Create/Update semantics elsewhere in this
// package). Returns ErrConflict/ErrKeyExists on a losing race, exactly like
// ResourceStore.Update -- AllocateNext retries on either.
func (a *ClusterIPAllocator) save(ctx context.Context, bitmap *k8sallocator.AllocationBitmap, revision int64) error {
	rangeSpec, data := bitmap.Snapshot()
	encoded, err := json.Marshal(clusterIPRangeSnapshot{Range: rangeSpec, Data: data})
	if err != nil {
		return fmt.Errorf("encode clusterip range: %w", err)
	}
	if revision == 0 {
		_, err = a.storage.Create(ctx, clusterIPRangeKey, encoded)
	} else {
		_, _, err = a.storage.Update(ctx, clusterIPRangeKey, encoded, revision)
	}
	return err
}

// AllocateNext picks and durably persists a free address, retrying if a
// concurrent allocation raced it -- the DO's CAS write is the actual
// serialization point, exactly like etcd's resourceVersion CAS on a real
// cluster.
func (a *ClusterIPAllocator) AllocateNext(ctx context.Context) (net.IP, error) {
	const maxRetries = 5
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		bitmap, revision, err := a.load(ctx)
		if err != nil {
			return nil, err
		}

		offset, ok, err := bitmap.AllocateNext()
		if err != nil {
			return nil, fmt.Errorf("allocate clusterip: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("allocate clusterip: range %s is full", a.cidr)
		}

		if err := a.save(ctx, bitmap, revision); err != nil {
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrKeyExists) {
				lastErr = err
				continue // another allocation raced us; reload and retry
			}
			return nil, err
		}

		return netutils.AddIPOffset(a.base, offset), nil
	}
	return nil, fmt.Errorf("allocate clusterip: too many concurrent conflicts: %w", lastErr)
}
