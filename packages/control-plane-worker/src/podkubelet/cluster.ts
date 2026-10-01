import { VIRTUAL_NODE } from "./spec.ts";

export const SERVICE_NAME_LABEL = "kubernetes.io/service-name";

export interface ServicePort {
  name?: string;
  port: number;
  targetPort?: number | string;
  protocol?: string;
}

export interface Service {
  metadata: { name: string; namespace: string };
  spec: { type?: string; clusterIP?: string; clusterIPs?: string[]; ports?: ServicePort[]; externalName?: string };
}

export interface EndpointSlice {
  metadata: { name: string; namespace: string; labels?: Record<string, string> };
  addressType?: string;
  endpoints?: Array<{
    addresses?: string[];
    conditions?: { ready?: boolean; serving?: boolean; terminating?: boolean };
    nodeName?: string;
    targetRef?: { kind?: string; namespace?: string; name?: string; uid?: string };
  }>;
  ports?: Array<{ name?: string; port?: number; protocol?: string }>;
}

export interface ClusterLookups {
  service(namespace: string, name: string): Promise<Service | null>;
  serviceByClusterIP(ip: string): Promise<Service | null>;
  endpointSlices(namespace: string, service: string): Promise<EndpointSlice[]>;
  podByIP(ip: string): Promise<{ uid: string } | null>;
}

export type ClusterTarget =
  | { kind: "pod"; uid: string; port: number }
  | { kind: "unreachable"; status: number; reason: string }
  | null;

export interface ServiceRef {
  namespace: string;
  name: string;
}

const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

export function isIPv4(host: string): boolean {
  const m = IPV4.exec(host);
  return m !== null && m.slice(1).every((part) => Number(part) <= 255);
}

export function parseServiceHost(host: string, podNamespace: string): ServiceRef | null {
  const name = host.toLowerCase().replace(/\.$/, "");
  if (name === "" || isIPv4(name)) return null;
  const labels = name.split(".");
  if (labels.some((l) => l === "")) return null;
  if (labels.length === 1) return { namespace: podNamespace, name: labels[0] };
  if (labels.length === 2) return { namespace: labels[1], name: labels[0] };
  if (labels.length === 3 && labels[2] === "svc") return { namespace: labels[1], name: labels[0] };
  if (labels.length === 5 && labels[2] === "svc" && labels[3] === "cluster" && labels[4] === "local") return { namespace: labels[1], name: labels[0] };
  return null;
}

function portName(ports: ServicePort[] | undefined, port: number): string | undefined {
  return ports?.find((p) => p.port === port && (p.protocol ?? "TCP") === "TCP")?.name;
}

function slicePort(slice: EndpointSlice, name: string | undefined, headlessPort: number | null): number | null {
  const ports = (slice.ports ?? []).filter((p) => (p.protocol ?? "TCP") === "TCP" && typeof p.port === "number" && p.port > 0);
  if (headlessPort !== null) return ports.some((p) => p.port === headlessPort) ? headlessPort : null;
  const match = ports.find((p) => (p.name ?? "") === (name ?? ""));
  if (match) return match.port!;
  if (ports.length === 1 && name === undefined) return ports[0].port!;
  return null;
}

function pick<T>(items: T[]): T {
  return items[Math.floor(Math.random() * items.length)];
}

export async function resolveServiceTarget(svc: Service, port: number, lookups: ClusterLookups): Promise<ClusterTarget> {
  const ref = `${svc.metadata.namespace}/${svc.metadata.name}`;
  if (svc.spec.type === "ExternalName") {
    return { kind: "unreachable", status: 502, reason: `service ${ref} is an ExternalName Service, which Pods on ${VIRTUAL_NODE} cannot follow` };
  }
  const headless = svc.spec.clusterIP === "None";
  let name: string | undefined;
  if (!headless) {
    const servicePort = svc.spec.ports?.find((p) => p.port === port && (p.protocol ?? "TCP") === "TCP");
    if (!servicePort) return { kind: "unreachable", status: 502, reason: `service ${ref} has no TCP port ${port}` };
    name = portName(svc.spec.ports, port);
  }
  const slices = await lookups.endpointSlices(svc.metadata.namespace, svc.metadata.name);
  const candidates: Array<{ uid: string; port: number }> = [];
  const elsewhere = new Set<string>();
  let anyReady = false;
  for (const slice of slices) {
    if (slice.addressType && slice.addressType !== "IPv4") continue;
    const targetPort = slicePort(slice, name, headless ? port : null);
    for (const ep of slice.endpoints ?? []) {
      if (ep.conditions?.ready === false) continue;
      anyReady = true;
      if (targetPort === null) continue;
      if (ep.nodeName === VIRTUAL_NODE && ep.targetRef?.kind === "Pod" && ep.targetRef.uid) {
        candidates.push({ uid: ep.targetRef.uid, port: targetPort });
      } else {
        elsewhere.add(ep.nodeName || "an unknown node");
      }
    }
  }
  if (candidates.length > 0) {
    const chosen = pick(candidates);
    return { kind: "pod", uid: chosen.uid, port: chosen.port };
  }
  if (!anyReady) return { kind: "unreachable", status: 503, reason: `service ${ref} has no ready endpoints` };
  if (elsewhere.size > 0) {
    return { kind: "unreachable", status: 502, reason: `the ready endpoints of service ${ref} are on ${[...elsewhere].sort().join(", ")}; Pods on ${VIRTUAL_NODE} can only reach Pods on ${VIRTUAL_NODE}` };
  }
  return { kind: "unreachable", status: 502, reason: `service ${ref} has no endpoint serving port ${port}` };
}

export async function resolveClusterTarget(host: string, port: number, podNamespace: string, lookups: ClusterLookups): Promise<ClusterTarget> {
  if (isIPv4(host)) {
    const pod = await lookups.podByIP(host);
    if (pod) return { kind: "pod", uid: pod.uid, port };
    const svc = await lookups.serviceByClusterIP(host);
    if (svc) return resolveServiceTarget(svc, port, lookups);
    return null;
  }
  const ref = parseServiceHost(host, podNamespace);
  if (!ref) return null;
  const svc = await lookups.service(ref.namespace, ref.name);
  if (!svc) return null;
  return resolveServiceTarget(svc, port, lookups);
}
