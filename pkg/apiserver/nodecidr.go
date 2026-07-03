package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset"
)

// podCIDRRangeKey is the kine storage key the set of already-allocated Node
// PodCIDRs is persisted under -- same CAS-snapshot design as
// clusterIPRangeKey in clusterip.go, just for CIDR blocks instead of single
// IPs.
const podCIDRRangeKey = "/ranges/podcidrs"

// clusterCIDR and nodeCIDRMaskSize must match cmd/controller-manager/main.go's
// real kube-controller-manager flags (--cluster-cidr=10.42.0.0/16,
// --node-cidr-mask-size unset -- upstream's own default for IPv4 is 24, see
// k8s.io/kubernetes/cmd/kube-controller-manager/app/options/nodeipamcontroller.go),
// so a Node allocated a PodCIDR here and a Node allocated one by the real
// nodeipam controller (BYO VM / host-process deployments, see CLAUDE.md) draw
// from the exact same address space and never collide.
var clusterCIDR = mustParseCIDR("10.42.0.0/16")

const nodeCIDRMaskSize = 24

// maxPodCIDRAllocatorRetries mirrors maxAllocatorRetries in clusterip.go --
// same CAS-retry reasoning.
const maxPodCIDRAllocatorRetries = 16

// AssignPodCIDR allocates and sets spec.podCIDR (and spec.podCIDRs) on node
// if it needs one, synchronously, before it's ever written to storage --
// called from handler.go's POST case, the same place AssignClusterIP runs
// for Services. Mirrors the real nodeipam (node IPAM) controller's
// allocate-on-registration behavior, minus the informer/workqueue framework
// (see docs/platform-verification.md's Phase 5 findings for why that
// framework can't fit Workers' size budget).
func AssignPodCIDR(ctx context.Context, storage *Storage, node *corev1.Node) error {
	if !nodeNeedsPodCIDR(node) {
		return nil
	}
	cidr, err := NodeCIDRAllocator(storage).AllocateNext(ctx)
	if err != nil {
		return err
	}
	cidrStr := cidr.String()
	node.Spec.PodCIDR = cidrStr
	node.Spec.PodCIDRs = []string{cidrStr}
	return nil
}

// ReleasePodCIDR returns node's PodCIDR to the pool, if it had one.
// Called from handler.go's DELETE cases, after the Node is already gone
// from storage: best-effort, logged rather than surfaced as a
// client-visible error, matching ReleaseClusterIP's behavior.
func ReleasePodCIDR(ctx context.Context, storage *Storage, node *corev1.Node) {
	if node.Spec.PodCIDR == "" {
		return
	}
	if err := NodeCIDRAllocator(storage).Release(ctx, node.Spec.PodCIDR); err != nil {
		log.Printf("failed to release PodCIDR %s for node %s: %v", node.Spec.PodCIDR, node.Name, err)
	}
}

// nodeNeedsPodCIDR reports whether node should get an allocated PodCIDR:
// only if it doesn't already have one (mirrors serviceNeedsClusterIP's
// shape; unlike Services there's no "None"/ExternalName-equivalent opt-out
// for Nodes).
func nodeNeedsPodCIDR(node *corev1.Node) bool {
	return node.Spec.PodCIDR == ""
}

// NodeCIDRAllocator returns the PodCIDRAllocator for the cluster's Pod
// address range (clusterCIDR above). Cheap to construct fresh per call, same
// reasoning as ServiceIPAllocator.
func NodeCIDRAllocator(storage *Storage) *PodCIDRAllocator {
	return NewPodCIDRAllocator(storage, clusterCIDR, nodeCIDRMaskSize)
}

// NewPodCIDRAllocator creates a PodCIDRAllocator for cidr, subdivided into
// maskSize-bit blocks, persisting its allocation state through storage. A
// separate constructor from the package-level NodeCIDRAllocator (which
// always uses the real clusterCIDR/nodeCIDRMaskSize) so tests can exercise
// allocation/exhaustion/release against a small range, the same way
// clusterip_allocator_test.go does for NewClusterIPAllocator.
func NewPodCIDRAllocator(storage *Storage, cidr *net.IPNet, maskSize int) *PodCIDRAllocator {
	return &PodCIDRAllocator{storage: storage, cidr: cidr, maskSize: maskSize}
}

// PodCIDRAllocator hands out unique /24 blocks from clusterCIDR, persisting
// the set of already-allocated blocks as a single kine object with
// optimistic-concurrency (CAS) retries -- the same design ClusterIPAllocator
// uses (clusterip.go), adapted for CIDR blocks instead of single IPs.
//
// The actual bit allocation is upstream's real
// k8s.io/kubernetes/pkg/controller/nodeipam/ipam/cidrset.CidrSet -- the same
// engine the real nodeipam (node IPAM) controller uses -- which is a leaf
// package with no client-go dependency (measured: +11KB gzip to add to this
// binary, see docs/cost-model.md). Unlike allocator.AllocationBitmap
// (clusterip.go), CidrSet has no exported Snapshot()/Restore(), so instead
// of persisting its internal bitmap directly, this allocator persists the
// list of already-allocated CIDR strings and replays them through
// CidrSet.Occupy() on every load -- functionally equivalent (CidrSet's own
// startup path, in the real nodeipam controller, does the same thing: it
// occupies every already-assigned Node.Spec.PodCIDR it finds on the
// informer's initial List before serving new allocations).
type PodCIDRAllocator struct {
	storage  *Storage
	cidr     *net.IPNet
	maskSize int
}

// podCIDRRangeSnapshot is the JSON envelope the allocated-CIDR list is
// stored as.
type podCIDRRangeSnapshot struct {
	Allocated []string `json:"allocated"`
}

// load fetches the persisted list of allocated CIDRs -- or an empty one if
// this range has never been allocated from before -- and replays it into a
// fresh CidrSet, along with the storage revision needed for a CAS
// write-back.
func (a *PodCIDRAllocator) load(ctx context.Context) (*cidrset.CidrSet, podCIDRRangeSnapshot, int64, error) {
	set, err := cidrset.NewCIDRSet(a.cidr, a.maskSize)
	if err != nil {
		return nil, podCIDRRangeSnapshot{}, 0, fmt.Errorf("create cidr set: %w", err)
	}

	stored, err := a.storage.Get(ctx, podCIDRRangeKey)
	if errors.Is(err, ErrNotFound) {
		return set, podCIDRRangeSnapshot{}, 0, nil
	}
	if err != nil {
		return nil, podCIDRRangeSnapshot{}, 0, fmt.Errorf("load podcidr range: %w", err)
	}

	var snap podCIDRRangeSnapshot
	if err := json.Unmarshal(stored.Value, &snap); err != nil {
		return nil, podCIDRRangeSnapshot{}, 0, fmt.Errorf("decode podcidr range: %w", err)
	}
	for _, cidrStr := range snap.Allocated {
		_, ipnet, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue // defensive: skip a corrupt entry rather than fail the whole load
		}
		if err := set.Occupy(ipnet); err != nil {
			continue // defensive: same reasoning
		}
	}
	return set, snap, stored.ModRevision, nil
}

// save CAS-writes snap back at revision (0 meaning "create", matching
// Storage.Create/Update semantics elsewhere in this package).
func (a *PodCIDRAllocator) save(ctx context.Context, snap podCIDRRangeSnapshot, revision int64) error {
	encoded, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encode podcidr range: %w", err)
	}
	if revision == 0 {
		_, err = a.storage.Create(ctx, podCIDRRangeKey, encoded)
	} else {
		_, _, err = a.storage.Update(ctx, podCIDRRangeKey, encoded, revision)
	}
	return err
}

// AllocateNext picks and durably persists a free /24 block, retrying if a
// concurrent allocation raced it -- same CAS-retry shape as
// ClusterIPAllocator.AllocateNext.
func (a *PodCIDRAllocator) AllocateNext(ctx context.Context) (*net.IPNet, error) {
	var lastErr error
	for i := 0; i < maxPodCIDRAllocatorRetries; i++ {
		set, snap, revision, err := a.load(ctx)
		if err != nil {
			return nil, err
		}

		cidr, err := set.AllocateNext()
		if err != nil {
			return nil, fmt.Errorf("allocate podcidr: %w", err)
		}

		snap.Allocated = append(snap.Allocated, cidr.String())
		if err := a.save(ctx, snap, revision); err != nil {
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrKeyExists) {
				lastErr = err
				continue // another allocation raced us; reload and retry
			}
			return nil, err
		}

		return cidr, nil
	}
	return nil, fmt.Errorf("allocate podcidr: too many concurrent conflicts: %w", lastErr)
}

// Release returns cidrStr to the pool so a future AllocateNext can reuse it.
// A no-op (not an error) if cidrStr wasn't actually allocated, matching
// ClusterIPAllocator.Release's idempotent behavior.
func (a *PodCIDRAllocator) Release(ctx context.Context, cidrStr string) error {
	var lastErr error
	for i := 0; i < maxPodCIDRAllocatorRetries; i++ {
		_, snap, revision, err := a.load(ctx)
		if err != nil {
			return err
		}

		kept := snap.Allocated[:0]
		found := false
		for _, s := range snap.Allocated {
			if s == cidrStr {
				found = true
				continue
			}
			kept = append(kept, s)
		}
		if !found {
			return nil // never allocated -- nothing to do
		}
		snap.Allocated = kept

		if err := a.save(ctx, snap, revision); err != nil {
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrKeyExists) {
				lastErr = err
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("release podcidr %s: too many concurrent conflicts: %w", cidrStr, lastErr)
}
