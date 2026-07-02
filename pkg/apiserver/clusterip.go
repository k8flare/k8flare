package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
// 10.43.0.1 for the future "kubernetes.default" Service (the conventional
// first address in a Service range), 10.43.0.10 for "kube-dns". Neither is
// provisioned yet, but a freshly initialized range blocks them out up
// front so a real allocation can never collide once they are. Expressed as
// literal IPs, not raw bitmap offsets, specifically so they stay correct
// regardless of how the allocator's internal offset 0 is defined (see
// NewClusterIPAllocator's network-address exclusion below).
var addressesReservedForFutureServices = []net.IP{
	net.ParseIP("10.43.0.1"),
	net.ParseIP("10.43.0.10"),
}

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

// ReleaseClusterIP returns svc's ClusterIP to the pool, if it had a real
// allocated one (not empty, not "None", not an ExternalName Service).
// Called from handler.go's DELETE cases, after the Service is already gone
// from storage: best-effort, logged rather than surfaced as a
// client-visible error, since the delete itself already succeeded by the
// time this runs and there's nothing left to roll back to.
func ReleaseClusterIP(ctx context.Context, storage *Storage, svc *corev1.Service) {
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return
	}
	ip := net.ParseIP(svc.Spec.ClusterIP)
	if ip == nil { // "", "None", or unparsable -- nothing was ever allocated
		return
	}
	if err := ServiceIPAllocator(storage).Release(ctx, ip); err != nil {
		log.Printf("failed to release ClusterIP %s for %s/%s: %v", svc.Spec.ClusterIP, svc.Namespace, svc.Name, err)
	}
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
// cheap -- no I/O happens until AllocateNext/Release is called -- so
// building a fresh instance per call is simpler than threading a shared
// one through main.go.
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
// ipallocator.Range itself calls internally, PLUS ipallocator.Range.New's
// own network/broadcast-address exclusion logic (base+1, and -1 more for
// IPv4's broadcast address), copied verbatim in NewClusterIPAllocator below
// so this allocator can never hand out the network or broadcast address
// the way a bare bitmap-with-no-exclusions otherwise would.
//
// This talks to allocator.AllocationBitmap directly rather than going
// through ipallocator.Range: that package also pulls in client-go
// informers/listers/cache for its newer IPAddress/ServiceCIDR allocation
// mode. Measured directly (not assumed): swapping this file to use
// ipallocator.Range and rebuilding workers/apiserver's WASM binary moved
// its gzip size from 7,691,909 bytes to 10,455,831 bytes (+2,763,922 bytes,
// ~2.6MiB) -- which alone exceeds this project's 9.5MiB gzip budget gate,
// for an allocation mode this project never exercises. The bitmap and IP
// math -- the actual hard part -- are 100% upstream code; only this
// coordinating layer, which ipallocator.Range also has (just wired to a
// client-go/etcd storage stack this project doesn't use), is
// project-specific.
type ClusterIPAllocator struct {
	storage *Storage
	cidr    *net.IPNet
	// base is the first *allocatable* address as a big.Int (cidr.IP + 1,
	// not cidr.IP itself -- see NewClusterIPAllocator).
	base *big.Int
	// max is the count of allocatable addresses (RangeSize(cidr), minus
	// the network address and, for IPv4, the broadcast address).
	max int
}

// NewClusterIPAllocator creates a ClusterIPAllocator for cidr, persisting
// its allocation state through storage.
func NewClusterIPAllocator(storage *Storage, cidr *net.IPNet) *ClusterIPAllocator {
	// Match k8s.io/kubernetes/pkg/registry/core/service/ipallocator.Range's
	// New(): "Don't use the network's '.0' address, but don't just
	// Allocate() it - we don't ever want to be able to release it" (base+1,
	// max--), and for IPv4 specifically, "Don't use the IPv4 network's
	// broadcast address" (max-- again). Both addresses are permanently
	// outside the allocatable range here, the same way upstream keeps them
	// outside its bitmap entirely, rather than merely pre-allocated (which
	// -- unlike the addressesReservedForFutureServices placeholders above,
	// which really may become real Services one day -- would wrongly let a
	// future Release put them back into circulation).
	base := big.NewInt(0).Add(netutils.BigForIP(cidr.IP), big.NewInt(1))
	max := int(netutils.RangeSize(cidr)) - 1
	if !netutils.IsIPv6CIDR(cidr) {
		max--
	}
	if max < 0 {
		max = 0
	}
	return &ClusterIPAllocator{
		storage: storage,
		cidr:    cidr,
		base:    base,
		max:     max,
	}
}

// offsetFor returns ip's offset within a's allocatable range (offset 0 is
// a.base, i.e. cidr.IP + 1 -- not cidr.IP itself). Returns an error if ip
// falls outside the allocatable range (before a.base, or at/after the
// excluded broadcast address).
func (a *ClusterIPAllocator) offsetFor(ip net.IP) (int, error) {
	offset := big.NewInt(0).Sub(netutils.BigForIP(ip), a.base)
	if offset.Sign() < 0 || offset.Cmp(big.NewInt(int64(a.max))) >= 0 {
		return 0, fmt.Errorf("ip %s is not in the allocatable range of %s", ip, a.cidr)
	}
	return int(offset.Int64()), nil
}

// newBitmap creates an empty bitmap for a's range, with
// addressesReservedForFutureServices pre-allocated. Only used the very
// first time this range is ever allocated from (see load).
func (a *ClusterIPAllocator) newBitmap() *k8sallocator.AllocationBitmap {
	bitmap := k8sallocator.NewAllocationMap(a.max, a.cidr.String())
	for _, ip := range addressesReservedForFutureServices {
		offset, err := a.offsetFor(ip)
		if err != nil {
			continue // outside this range (e.g. a non-default ServiceCIDR in a test) -- nothing to reserve
		}
		bitmap.Allocate(offset) // fresh map, valid offset: cannot fail
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
// ResourceStore.Update -- AllocateNext/Release retry on either.
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

// Release returns ip to the pool so a future AllocateNext can reuse it.
// Same CAS-retry pattern as AllocateNext. A no-op (not an error) if ip
// wasn't actually allocated -- matching allocator.AllocationBitmap.Release's
// own idempotent behavior -- so releasing twice, or releasing an address
// from before this allocator was ever populated, is harmless.
func (a *ClusterIPAllocator) Release(ctx context.Context, ip net.IP) error {
	offset, err := a.offsetFor(ip)
	if err != nil {
		return err
	}

	const maxRetries = 5
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		bitmap, revision, err := a.load(ctx)
		if err != nil {
			return err
		}

		if err := bitmap.Release(offset); err != nil {
			return fmt.Errorf("release clusterip %s: %w", ip, err)
		}

		if err := a.save(ctx, bitmap, revision); err != nil {
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrKeyExists) {
				lastErr = err
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("release clusterip %s: too many concurrent conflicts: %w", ip, lastErr)
}
