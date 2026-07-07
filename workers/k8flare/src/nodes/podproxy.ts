// Task #13: HTTP ingress to Pod-on-Containers Pods, two entry points that
// share one underlying primitive -- "reach this Pod's own container port,
// via its NodeVM" -- because `hostNetwork: true`
// (pkg/apiserver/computeclass.go) makes every container port directly
// reachable on the VM's own network stack (verified over SSH,
// docs/platform-verification.md S16: `curl 127.0.0.1:80` returned the
// Pod's own nginx page).
//
//   1. `handlePodProxy`: the standard `pods/{name}/proxy/{path}`
//      subresource (gateway/index.ts already routes it here as
//      `/podproxy/{ns}/{pod}[:port]/{path}`) -- an operator/kubectl
//      reaching INTO a Pod from outside the cluster.
//   2. `handleVKubeProxy`: the virtual kube-proxy backend half -- a Pod's
//      own outbound ClusterIP traffic, intercepted node-side by
//      pkg/vkubeproxy's tun/netstack forwarder and re-issued here as a
//      plain HTTP request carrying its original target ClusterIP:port in
//      headers, resolved via Service+EndpointSlice to a backing Pod.
//
// Both are v1, HTTP/1.x request-response only (matches the design's
// explicit deferral of raw TCP and secure/TokenReview'd kubelet
// exec/attach to later work).
import type { Env } from "./env.ts";
import {
  getPod,
  getServiceByClusterIP,
  listEndpointSlicesForService,
  type EndpointSliceEndpoint,
} from "./client.ts";
import { vmBinding } from "./scheduler.ts";

// Headers only meaningful between k8flare's own components (the cluster
// bearer token, our own routing headers) -- stripped before the request
// reaches an arbitrary Pod's application, which has no relationship to
// either.
const INTERNAL_REQUEST_HEADERS = ["authorization", "x-k8flare-target-ip", "x-k8flare-target-port"];

function stripInternalHeaders(req: Request): Headers {
  const headers = new Headers(req.headers);
  for (const h of INTERNAL_REQUEST_HEADERS) headers.delete(h);
  return headers;
}

/**
 * Resolves podUID's live NodeVM (nodes/scheduler.ts is the single source
 * of truth for which VM a Pod is currently bound to) and forwards req to
 * `port` on it, via nodevm.ts's generic port bridge. path is the
 * downstream request path+query (NOT including the /podproxy or
 * /vkubeproxy prefix -- callers below resolve that).
 */
export async function forwardToPod(
  env: Env,
  podUID: string,
  port: number,
  path: string,
  req: Request,
): Promise<Response> {
  const scheduler = env.SCHEDULER.get(env.SCHEDULER.idFromName("default"));
  const vm = await scheduler.lookupVM(podUID);
  if (!vm) {
    return new Response(`no live NodeVM for pod ${podUID}`, { status: 404 });
  }
  const ns = vmBinding(env, vm.tier);
  const target = new URL(req.url);
  target.pathname = `/${port}${path}`;
  return ns.get(ns.idFromName(vm.podUID)).fetch(
    new Request(target.toString(), {
      method: req.method,
      headers: stripInternalHeaders(req),
      body: req.body,
      // Required by the Fetch spec whenever a RequestInit's body is a
      // ReadableStream (rather than passing a Request object through
      // directly, which is what nodevm.ts's own bridge does when it
      // doesn't need to touch headers first).
      duplex: req.body ? "half" : undefined,
    } as RequestInit),
  );
}

const PODPROXY_PATH = /^\/podproxy\/([^/]+)\/([^/]+)(\/.*)?$/;

/**
 * Handles `/podproxy/{namespace}/{pod}[:port]{rest}` (the pods/proxy
 * subresource, routed here by gateway/index.ts). `pod` may carry an
 * explicit `:port` suffix (the standard Kubernetes `pods/{name}:{port}/
 * proxy/{path}` addressing) -- without one, this defaults to the Pod's
 * first declared container port, matching typical single-port Pods
 * (named ports are not resolved here; use the numeric form for those).
 */
export async function handlePodProxy(req: Request, env: Env, pathname: string): Promise<Response> {
  const match = pathname.match(PODPROXY_PATH);
  if (!match) {
    return new Response("expected /podproxy/{namespace}/{pod}[:port]/{path}", { status: 400 });
  }
  const [, namespace, podParam, rest] = match;
  const [podName, portStr] = podParam.split(":", 2);

  const pod = await getPod(env, namespace, podName);
  if (!pod || !pod.metadata.uid) {
    return new Response(`pod ${namespace}/${podName} not found`, { status: 404 });
  }

  let port: number | undefined = portStr ? Number(portStr) : undefined;
  if (port === undefined) {
    port = pod.spec?.containers?.[0]?.ports?.[0]?.containerPort;
  }
  if (!port || Number.isNaN(port)) {
    return new Response(
      `pod ${namespace}/${podName} declares no container port -- use pods/${podName}:{port}/proxy`,
      { status: 400 },
    );
  }

  return forwardToPod(env, pod.metadata.uid, port, rest || "/", req);
}

/**
 * Handles the virtual-kube-proxy backend resolution: given the intercepted
 * connection's original destination (X-K8flare-Target-IP/-Port, set by
 * pkg/vkubeproxy) and the app's own request (method/path/headers/body,
 * forwarded as-is), resolves the owning Service by ClusterIP
 * (`spec.clusterIP` field selector, pkg/apiserver/store.go), then its
 * ready backing Pod via EndpointSlice (pkg/apiserver/endpoints.go builds
 * exactly one combined slice per Service), then forwards like
 * handlePodProxy above. appPath is the app's own request path+query
 * (nodes/index.ts strips the /vkubeproxy prefix before calling this).
 */
export async function handleVKubeProxy(req: Request, env: Env, appPath: string): Promise<Response> {
  const targetIP = req.headers.get("X-K8flare-Target-IP");
  const targetPortStr = req.headers.get("X-K8flare-Target-Port");
  if (!targetIP || !targetPortStr) {
    return new Response("missing X-K8flare-Target-IP/X-K8flare-Target-Port headers", {
      status: 400,
    });
  }
  const targetPort = Number(targetPortStr);
  if (Number.isNaN(targetPort)) {
    return new Response("X-K8flare-Target-Port must be numeric", { status: 400 });
  }

  const svc = await getServiceByClusterIP(env, targetIP);
  if (!svc) {
    return new Response(`no Service with ClusterIP ${targetIP}`, { status: 502 });
  }
  const svcPort = svc.spec?.ports?.find((p) => p.port === targetPort);
  if (!svcPort) {
    return new Response(
      `Service ${svc.metadata.namespace}/${svc.metadata.name} has no port ${targetPort}`,
      { status: 502 },
    );
  }

  const slices = await listEndpointSlicesForService(env, svc.metadata.namespace, svc.metadata.name);
  let containerPort: number | undefined;
  let endpoint: EndpointSliceEndpoint | undefined;
  for (const slice of slices) {
    // This project builds one combined EndpointSlice per Service
    // (endpoints.go's buildEndpointSlice), whose Ports already carry the
    // resolved container port number -- match by name if the Service
    // names its ports, otherwise there is only one to pick.
    const portEntry = svcPort.name
      ? slice.ports?.find((p) => p.name === svcPort.name)
      : slice.ports?.[0];
    if (!portEntry?.port) continue;
    const ready = slice.endpoints?.find((e) => e.conditions?.ready !== false && e.targetRef?.uid);
    if (ready) {
      containerPort = portEntry.port;
      endpoint = ready;
      break;
    }
  }
  if (!containerPort || !endpoint?.targetRef?.uid) {
    return new Response(
      `no ready endpoint for Service ${svc.metadata.namespace}/${svc.metadata.name}:${targetPort}`,
      { status: 503 },
    );
  }

  return forwardToPod(env, endpoint.targetRef.uid, containerPort, appPath, req);
}
