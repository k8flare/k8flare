// Image x size-tier allowlist (Phase 7 v1). See workers/nodes/README.md and
// spikes/s3-containers/FINDINGS.md items 4 and the "instance sizing" addendum
// for why this shape is forced by the platform, not a design choice this
// project made freely:
//
//   - Cloudflare Containers picks the container image from a
//     `containers[].image` entry in wrangler.jsonc, fixed at deploy time --
//     there is no API for a Worker/DO to choose an arbitrary image at
//     runtime, so `kubectl run --image=<anything>` cannot work here. A Pod's
//     `spec.containers[0].image` must name one of ALLOWED_IMAGES below.
//   - `instance_type` (vCPU/memory/disk) is likewise a `containers[]` entry
//     field fixed at deploy time (only envVars/entrypoint/enableInternet/
//     labels can be overridden per-start). So honoring a Pod's
//     resources.requests means picking, per Pod, one of a small fixed set of
//     pre-declared (image, instance_type) pairs -- a size *tier* -- rather
//     than an arbitrary size. This file rounds a Pod's requests up to the
//     nearest tier below.
//
// Each tier maps 1:1 to a `containers[]` entry + Container subclass in
// podcontainer.ts + a durable_objects binding in wrangler.jsonc -- adding a
// new base image means adding one more (image x tier) entry to
// ALLOWED_IMAGES, one Dockerfile under images/, one Container subclass, one
// wrangler.jsonc containers[]/durable_objects entry, and one case in
// env.ts/index.ts's binding lookup. Mechanical, but not automatic -- there is
// no dynamic registration mechanism (matches the platform constraint above).

export type SizeTier = "small" | "medium" | "large";

export const SIZE_TIERS: Record<
  SizeTier,
  {
    /** Cloudflare Containers named instance_type (wrangler.jsonc, deploy-time-fixed). */
    instanceType: "lite" | "basic" | "standard-1";
    /** Allocatable CPU for this tier, in millicores -- matches instanceType's real vCPU fraction. */
    milliCPU: number;
    /** Allocatable memory for this tier, in bytes -- matches instanceType's real memory_mib. */
    memoryBytes: number;
  }
> = {
  // lite: 1/16 vCPU, 256 MiB, 2 GB disk (Cloudflare Containers named instance type)
  small: { instanceType: "lite", milliCPU: 63, memoryBytes: 256 * 1024 * 1024 },
  // basic: 1/4 vCPU, 1 GiB, 4 GB disk
  medium: { instanceType: "basic", milliCPU: 250, memoryBytes: 1 * 1024 * 1024 * 1024 },
  // standard-1: 1/2 vCPU, 4 GiB, 8 GB disk
  large: { instanceType: "standard-1", milliCPU: 500, memoryBytes: 4 * 1024 * 1024 * 1024 },
};

// Ordered smallest-to-largest so resolveSizeTier can pick the first tier that
// fits.
const TIER_ORDER: SizeTier[] = ["small", "medium", "large"];

// v1's only allowlisted image -- see workers/nodes/images/demo. A real
// deployment would list its own curated images here instead/in addition.
export const ALLOWED_IMAGES: ReadonlySet<string> = new Set(["k8flare/demo:latest"]);

export function isAllowedImage(image: string): boolean {
  return ALLOWED_IMAGES.has(image);
}

/** Parses a Kubernetes CPU quantity ("100m", "0.5", "2") into millicores. */
export function parseCPUQuantity(qty: string | undefined): number {
  if (!qty) return 0;
  if (qty.endsWith("m")) return Number.parseFloat(qty.slice(0, -1));
  return Number.parseFloat(qty) * 1000;
}

const MEMORY_SUFFIXES: Record<string, number> = {
  Ki: 1024,
  Mi: 1024 ** 2,
  Gi: 1024 ** 3,
  Ti: 1024 ** 4,
  K: 1000,
  M: 1000 ** 2,
  G: 1000 ** 3,
  T: 1000 ** 4,
};

/** Parses a Kubernetes memory quantity ("256Mi", "1Gi", "512000000") into bytes. */
export function parseMemoryQuantity(qty: string | undefined): number {
  if (!qty) return 0;
  for (const [suffix, multiplier] of Object.entries(MEMORY_SUFFIXES)) {
    if (qty.endsWith(suffix)) {
      return Number.parseFloat(qty.slice(0, -suffix.length)) * multiplier;
    }
  }
  return Number.parseFloat(qty); // plain byte count
}

export interface PodResourceRequest {
  milliCPU: number;
  memoryBytes: number;
}

/** Minimal shape this module needs from a real corev1.Pod -- see client.ts for the full wire type. */
export interface PodLike {
  spec?: {
    containers?: Array<{
      resources?: { requests?: Record<string, string> };
    }>;
  };
}

/** Sums resources.requests across every container in the Pod (Pod-level total, matching how a real Node's allocatable is consumed). */
export function podResourceRequests(pod: PodLike): PodResourceRequest {
  let milliCPU = 0;
  let memoryBytes = 0;
  for (const c of pod.spec?.containers ?? []) {
    const requests = c.resources?.requests ?? {};
    milliCPU += parseCPUQuantity(requests.cpu);
    memoryBytes += parseMemoryQuantity(requests.memory);
  }
  return { milliCPU, memoryBytes };
}

/**
 * Rounds a Pod's total resource requests up to the nearest size tier that
 * fits both its CPU and memory ask, per this file's header comment. Returns
 * "small" for a Pod with no requests at all (matches a real cluster's
 * behavior of scheduling best-effort Pods onto any node). Returns undefined
 * if the Pod's request exceeds even the largest tier -- the caller should
 * reject/fail the Pod rather than silently under-provisioning it.
 */
export function resolveSizeTier(pod: PodLike): SizeTier | undefined {
  const { milliCPU, memoryBytes } = podResourceRequests(pod);
  for (const tier of TIER_ORDER) {
    const capacity = SIZE_TIERS[tier];
    if (milliCPU <= capacity.milliCPU && memoryBytes <= capacity.memoryBytes) {
      return tier;
    }
  }
  return undefined;
}
