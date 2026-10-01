import assert from "node:assert/strict";
import { test } from "node:test";
import { parseServiceHost, resolveClusterTarget, type ClusterLookups, type EndpointSlice, type Service } from "../src/podkubelet/cluster.ts";

function service(overrides: Partial<Service["spec"]> = {}): Service {
  return { metadata: { name: "web", namespace: "shop" }, spec: { clusterIP: "10.43.0.10", ports: [{ name: "http", port: 80, targetPort: 8080 }], ...overrides } };
}

function slice(endpoints: EndpointSlice["endpoints"], ports: EndpointSlice["ports"] = [{ name: "http", port: 8080 }]): EndpointSlice {
  return { metadata: { name: "web-abc", namespace: "shop", labels: { "kubernetes.io/service-name": "web" } }, addressType: "IPv4", endpoints, ports };
}

function lookups(overrides: Partial<ClusterLookups> & { svc?: Service | null; slices?: EndpointSlice[] } = {}): ClusterLookups & { calls: string[] } {
  const calls: string[] = [];
  const svc = overrides.svc === undefined ? service() : overrides.svc;
  return {
    calls,
    service: async (ns, name) => {
      calls.push(`service ${ns}/${name}`);
      return svc && svc.metadata.namespace === ns && svc.metadata.name === name ? svc : null;
    },
    serviceByClusterIP: async (ip) => {
      calls.push(`clusterip ${ip}`);
      return svc && svc.spec.clusterIP === ip ? svc : null;
    },
    endpointSlices: async (ns, name) => {
      calls.push(`slices ${ns}/${name}`);
      return overrides.slices ?? [];
    },
    podByIP: async (ip) => {
      calls.push(`pod ${ip}`);
      return ip === "10.42.255.7" ? { uid: "pod-7" } : null;
    },
    ...overrides,
  };
}

test("service hosts resolve in every form the cluster search path would", () => {
  assert.deepEqual(parseServiceHost("web", "shop"), { namespace: "shop", name: "web" });
  assert.deepEqual(parseServiceHost("web.shop", "default"), { namespace: "shop", name: "web" });
  assert.deepEqual(parseServiceHost("web.shop.svc", "default"), { namespace: "shop", name: "web" });
  assert.deepEqual(parseServiceHost("web.shop.svc.cluster.local.", "default"), { namespace: "shop", name: "web" });
  assert.equal(parseServiceHost("web.shop.pod.cluster.local", "default"), null);
  assert.equal(parseServiceHost("www.example.com", "default"), null);
  assert.equal(parseServiceHost("10.43.0.10", "default"), null);
});

test("a ClusterIP resolves through the Service to a ready endpoint Pod on cloudflare with the target port", async () => {
  const l = lookups({ slices: [slice([{ addresses: ["10.42.255.9"], conditions: { ready: true }, nodeName: "cloudflare", targetRef: { kind: "Pod", namespace: "shop", name: "web-1", uid: "pod-9" } }])] });
  assert.deepEqual(await resolveClusterTarget("10.43.0.10", 80, "default", l), { kind: "pod", uid: "pod-9", port: 8080 });
  assert.deepEqual(l.calls, ["pod 10.43.0.10", "clusterip 10.43.0.10", "slices shop/web"]);
});

test("a service name resolves the same way and skips endpoints that are not ready", async () => {
  const l = lookups({
    slices: [
      slice([
        { addresses: ["10.42.255.8"], conditions: { ready: false }, nodeName: "cloudflare", targetRef: { kind: "Pod", namespace: "shop", name: "web-0", uid: "pod-8" } },
        { addresses: ["10.42.255.9"], conditions: { ready: true }, nodeName: "cloudflare", targetRef: { kind: "Pod", namespace: "shop", name: "web-1", uid: "pod-9" } },
      ]),
    ],
  });
  assert.deepEqual(await resolveClusterTarget("web.shop.svc.cluster.local", 80, "default", l), { kind: "pod", uid: "pod-9", port: 8080 });
  assert.deepEqual(await resolveClusterTarget("web", 80, "shop", l), { kind: "pod", uid: "pod-9", port: 8080 });
});

test("endpoints on real nodes are reported as unreachable with the node names", async () => {
  const l = lookups({ slices: [slice([{ addresses: ["10.42.0.5"], conditions: { ready: true }, nodeName: "node-a", targetRef: { kind: "Pod", namespace: "shop", name: "web-2", uid: "pod-5" } }])] });
  const target = await resolveClusterTarget("web.shop.svc", 80, "default", l);
  assert.equal(target?.kind, "unreachable");
  assert.equal(target?.kind === "unreachable" && target.status, 502);
  assert.match(target?.kind === "unreachable" ? target.reason : "", /on node-a; Pods on cloudflare can only reach Pods on cloudflare/);
});

test("a service without ready endpoints is 503 and a wrong port is 502", async () => {
  const none = await resolveClusterTarget("web.shop.svc", 80, "default", lookups({ slices: [slice([{ addresses: ["10.42.255.9"], conditions: { ready: false }, nodeName: "cloudflare" }])] }));
  assert.deepEqual(none, { kind: "unreachable", status: 503, reason: "service shop/web has no ready endpoints" });
  const port = await resolveClusterTarget("web.shop.svc", 9090, "default", lookups());
  assert.deepEqual(port, { kind: "unreachable", status: 502, reason: "service shop/web has no TCP port 9090" });
});

test("a headless service matches the requested port against the endpoint ports directly", async () => {
  const l = lookups({ svc: service({ clusterIP: "None", ports: [] }), slices: [slice([{ addresses: ["10.42.255.9"], conditions: { ready: true }, nodeName: "cloudflare", targetRef: { kind: "Pod", uid: "pod-9" } }], [{ name: "", port: 5432 }])] });
  assert.deepEqual(await resolveClusterTarget("web.shop.svc", 5432, "default", l), { kind: "pod", uid: "pod-9", port: 5432 });
  const missing = await resolveClusterTarget("web.shop.svc", 80, "default", l);
  assert.deepEqual(missing, { kind: "unreachable", status: 502, reason: "service shop/web has no endpoint serving port 80" });
});

test("a Pod IP of a cloudflare Pod goes straight to that Pod, and unknown hosts pass through", async () => {
  const l = lookups();
  assert.deepEqual(await resolveClusterTarget("10.42.255.7", 8080, "default", l), { kind: "pod", uid: "pod-7", port: 8080 });
  assert.equal(await resolveClusterTarget("10.42.255.99", 80, "default", l), null);
  assert.equal(await resolveClusterTarget("www.example.com", 80, "default", l), null);
  assert.equal(await resolveClusterTarget("missing.shop.svc", 80, "default", l), null);
  assert.equal(l.calls.includes("service shop/missing"), true);
});

test("ExternalName services are refused rather than followed", async () => {
  const target = await resolveClusterTarget("web.shop.svc", 80, "default", lookups({ svc: service({ type: "ExternalName", clusterIP: "", externalName: "example.com" }) }));
  assert.equal(target?.kind, "unreachable");
  assert.match(target?.kind === "unreachable" ? target.reason : "", /ExternalName/);
});
