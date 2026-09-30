export const VIRTUAL_NODE = "cloudflare";
export const DEFAULT_POD_CIDR = "10.42.255.0/24";
export const INSTANCE_ANNOTATION = "containers.k8flare.com/instance";
export const IMAGE_ANNOTATION = "containers.k8flare.com/image";
export const DECLARED_IMAGE_PREFIX = "images/";
export const CONTAINERS_CA_PATH = "/etc/cloudflare/certs/cloudflare-containers-ca.crt";
export const API_HOST = "kubernetes.default.svc";

export const INITIAL_CRASH_LOOP_BACKOFF_MS = 10_000;
export const MAX_CRASH_LOOP_BACKOFF_MS = 300_000;
export const INITIAL_START_BACKOFF_MS = 5_000;
export const MAX_START_BACKOFF_MS = 300_000;
export const DEFAULT_TERMINATION_GRACE_S = 30;

export interface KeyRef {
  name: string;
  key: string;
  optional?: boolean;
}

export interface EnvVarSource {
  configMapKeyRef?: KeyRef;
  secretKeyRef?: KeyRef;
  fieldRef?: { apiVersion?: string; fieldPath: string };
  resourceFieldRef?: { containerName?: string; resource: string; divisor?: string };
}

export interface EnvVar {
  name: string;
  value?: string;
  valueFrom?: EnvVarSource;
}

export interface EnvFromSource {
  prefix?: string;
  configMapRef?: { name: string; optional?: boolean };
  secretRef?: { name: string; optional?: boolean };
}

export interface HTTPGetAction {
  path?: string;
  port: number | string;
  host?: string;
  scheme?: string;
  httpHeaders?: Array<{ name: string; value: string }>;
}

export interface Probe {
  httpGet?: HTTPGetAction;
  tcpSocket?: { port: number | string; host?: string };
  exec?: { command: string[] };
  initialDelaySeconds?: number;
  periodSeconds?: number;
  timeoutSeconds?: number;
  successThreshold?: number;
  failureThreshold?: number;
}

export interface ContainerPort {
  name?: string;
  containerPort: number;
  protocol?: string;
}

export interface PodContainer {
  name: string;
  image: string;
  command?: string[];
  args?: string[];
  env?: EnvVar[];
  envFrom?: EnvFromSource[];
  ports?: ContainerPort[];
  resources?: { requests?: Record<string, string>; limits?: Record<string, string> };
  livenessProbe?: Probe;
  readinessProbe?: Probe;
  startupProbe?: Probe;
}

export interface PodCondition {
  type: string;
  status: "True" | "False" | "Unknown";
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
  lastProbeTime?: string | null;
}

export interface ContainerStateWaiting {
  reason?: string;
  message?: string;
}

export interface ContainerStateRunning {
  startedAt?: string;
}

export interface ContainerStateTerminated {
  exitCode: number;
  reason?: string;
  message?: string;
  startedAt?: string;
  finishedAt?: string;
  containerID?: string;
}

export interface ContainerState {
  waiting?: ContainerStateWaiting;
  running?: ContainerStateRunning;
  terminated?: ContainerStateTerminated;
}

export interface ContainerStatus {
  name: string;
  image: string;
  imageID: string;
  containerID?: string;
  ready: boolean;
  started?: boolean;
  restartCount: number;
  state: ContainerState;
  lastState: ContainerState;
}

export interface PodStatus {
  phase?: string;
  reason?: string;
  message?: string;
  conditions?: PodCondition[];
  containerStatuses?: ContainerStatus[];
  podIP?: string;
  podIPs?: Array<{ ip: string }>;
  hostIP?: string;
  hostIPs?: Array<{ ip: string }>;
  startTime?: string;
}

export interface Pod {
  apiVersion?: string;
  kind?: string;
  metadata: {
    name: string;
    namespace: string;
    uid: string;
    resourceVersion?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    deletionTimestamp?: string;
    deletionGracePeriodSeconds?: number;
  };
  spec: {
    containers: PodContainer[];
    restartPolicy?: "Always" | "OnFailure" | "Never";
    nodeName?: string;
    serviceAccountName?: string;
    terminationGracePeriodSeconds?: number;
  };
  status?: PodStatus;
}

export interface ImageMeta {
  entrypoint: string[];
  cmd: string[];
  workingDir: string;
  user: string;
  env: string[];
}

export interface ContainerInstance {
  vcpu: number;
  memoryMib: number;
  diskMb: number;
}

export type InstanceSpec = string | ContainerInstance;

export function expand(input: string, mapping: (name: string) => string): string {
  let out = "";
  let checkpoint = 0;
  for (let cursor = 0; cursor < input.length; cursor++) {
    if (input[cursor] !== "$" || cursor + 1 >= input.length) continue;
    out += input.slice(checkpoint, cursor);
    const rest = input.slice(cursor + 1);
    let read: string;
    let isVar = false;
    let advance: number;
    if (rest[0] === "$") {
      read = "$";
      advance = 1;
    } else if (rest[0] === "(") {
      const close = rest.indexOf(")", 1);
      if (close >= 0) {
        read = rest.slice(1, close);
        isVar = true;
        advance = close + 1;
      } else {
        read = "$(";
        advance = 1;
      }
    } else {
      const first = String.fromCodePoint(rest.codePointAt(0)!);
      read = "$" + first;
      advance = first.length;
    }
    out += isVar ? mapping(read) : read;
    cursor += advance;
    checkpoint = cursor + 1;
  }
  return out + input.slice(checkpoint);
}

export function mappingFor(...contexts: Array<Record<string, string>>): (name: string) => string {
  return (name) => {
    for (const vars of contexts) {
      if (Object.prototype.hasOwnProperty.call(vars, name)) return vars[name];
    }
    return `$(${name})`;
  };
}

export function composeEntrypoint(container: PodContainer, image: ImageMeta | undefined, env: Record<string, string>): string[] {
  const mapping = mappingFor(env);
  const command = (container.command ?? []).map((c) => expand(c, mapping));
  const args = (container.args ?? []).map((a) => expand(a, mapping));
  if (command.length > 0) return [...command, ...args];
  if (!image) throw new Error(`image ${container.image} has no recorded ENTRYPOINT/CMD; set spec.containers[].command`);
  if (args.length > 0) return [...image.entrypoint, ...args];
  const entrypoint = [...image.entrypoint, ...image.cmd];
  if (entrypoint.length === 0) throw new Error(`image ${container.image} declares neither ENTRYPOINT nor CMD; set spec.containers[].command`);
  return entrypoint;
}

export function imageEnv(image: ImageMeta | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const entry of image?.env ?? []) {
    const eq = entry.indexOf("=");
    if (eq <= 0) continue;
    out[entry.slice(0, eq)] = entry.slice(eq + 1);
  }
  return out;
}

export interface EnvLookups {
  configMap(name: string): Promise<Record<string, string> | null>;
  secret(name: string): Promise<Record<string, string> | null>;
}

export interface EnvContext {
  pod: Pod;
  container: PodContainer;
  podIP: string;
  hostIP: string;
}

const envNamePattern = /^[-._a-zA-Z][-._a-zA-Z0-9]*$/;

function fieldRefValue(ctx: EnvContext, fieldPath: string): string {
  const { pod } = ctx;
  const keyed = /^metadata\.(labels|annotations)\['([^']*)'\]$/.exec(fieldPath);
  if (keyed) return pod.metadata[keyed[1] as "labels" | "annotations"]?.[keyed[2]] ?? "";
  switch (fieldPath) {
    case "metadata.name":
      return pod.metadata.name;
    case "metadata.namespace":
      return pod.metadata.namespace;
    case "metadata.uid":
      return pod.metadata.uid;
    case "spec.nodeName":
      return pod.spec.nodeName ?? VIRTUAL_NODE;
    case "spec.serviceAccountName":
      return pod.spec.serviceAccountName ?? "default";
    case "status.hostIP":
      return ctx.hostIP;
    case "status.hostIPs":
      return ctx.hostIP;
    case "status.podIP":
    case "status.podIPs":
      return ctx.podIP;
  }
  throw new Error(`unsupported fieldRef ${fieldPath}`);
}

const binarySuffixes: Record<string, number> = { Ki: 2 ** 10, Mi: 2 ** 20, Gi: 2 ** 30, Ti: 2 ** 40, Pi: 2 ** 50, Ei: 2 ** 60 };
const decimalSuffixes: Record<string, number> = { n: 1e-9, u: 1e-6, m: 1e-3, "": 1, k: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18 };

export function parseQuantity(text: string): number {
  const m = /^([0-9.]+)(Ki|Mi|Gi|Ti|Pi|Ei|[numkMGTPE]?)$/.exec(text.trim());
  if (!m) throw new Error(`invalid quantity ${text}`);
  const scale = binarySuffixes[m[2]] ?? decimalSuffixes[m[2]];
  return Number(m[1]) * scale;
}

function resourceFieldValue(ctx: EnvContext, ref: NonNullable<EnvVarSource["resourceFieldRef"]>): string {
  const container = ref.containerName ? ctx.pod.spec.containers.find((c) => c.name === ref.containerName) : ctx.container;
  if (!container) throw new Error(`resourceFieldRef container ${ref.containerName} not found`);
  const [bucket, resource] = ref.resource.split(".", 2);
  if ((bucket !== "limits" && bucket !== "requests") || !resource) throw new Error(`unsupported resourceFieldRef ${ref.resource}`);
  const limits = container.resources?.limits ?? {};
  const requests = container.resources?.requests ?? {};
  const raw = bucket === "requests" ? requests[resource] ?? limits[resource] : limits[resource] ?? requests[resource];
  const value = raw ? parseQuantity(raw) : 0;
  const divisor = parseQuantity(ref.divisor || "1");
  return String(Math.ceil(value / divisor));
}

export async function resolveEnv(ctx: EnvContext, lookups: EnvLookups): Promise<Record<string, string>> {
  const { container } = ctx;
  const out: Record<string, string> = {};
  for (const source of container.envFrom ?? []) {
    let data: Record<string, string> | null = null;
    if (source.configMapRef) {
      data = await lookups.configMap(source.configMapRef.name);
      if (!data && !source.configMapRef.optional) throw new Error(`configmap "${source.configMapRef.name}" not found`);
    } else if (source.secretRef) {
      data = await lookups.secret(source.secretRef.name);
      if (!data && !source.secretRef.optional) throw new Error(`secret "${source.secretRef.name}" not found`);
    }
    for (const [k, v] of Object.entries(data ?? {})) {
      const name = (source.prefix ?? "") + k;
      if (!envNamePattern.test(name)) continue;
      out[name] = v;
    }
  }
  for (const envVar of container.env ?? []) {
    let value = "";
    const from = envVar.valueFrom;
    if (envVar.value !== undefined) {
      value = expand(envVar.value, mappingFor(out));
    } else if (from?.configMapKeyRef) {
      const ref = from.configMapKeyRef;
      const data = await lookups.configMap(ref.name);
      if (!data) {
        if (ref.optional) continue;
        throw new Error(`configmap "${ref.name}" not found`);
      }
      if (!(ref.key in data)) {
        if (ref.optional) continue;
        throw new Error(`couldn't find key ${ref.key} in ConfigMap ${ctx.pod.metadata.namespace}/${ref.name}`);
      }
      value = data[ref.key];
    } else if (from?.secretKeyRef) {
      const ref = from.secretKeyRef;
      const data = await lookups.secret(ref.name);
      if (!data) {
        if (ref.optional) continue;
        throw new Error(`secret "${ref.name}" not found`);
      }
      if (!(ref.key in data)) {
        if (ref.optional) continue;
        throw new Error(`couldn't find key ${ref.key} in Secret ${ctx.pod.metadata.namespace}/${ref.name}`);
      }
      value = data[ref.key];
    } else if (from?.fieldRef) {
      value = fieldRefValue(ctx, from.fieldRef.fieldPath);
    } else if (from?.resourceFieldRef) {
      value = resourceFieldValue(ctx, from.resourceFieldRef);
    }
    out[envVar.name] = value;
  }
  return out;
}

export function platformEnv(): Record<string, string> {
  return {
    KUBERNETES_SERVICE_HOST: API_HOST,
    KUBERNETES_SERVICE_PORT: "443",
    KUBERNETES_SERVICE_PORT_HTTPS: "443",
    KUBERNETES_PORT: "tcp://kubernetes.default.svc:443",
    SSL_CERT_FILE: CONTAINERS_CA_PATH,
    NODE_EXTRA_CA_CERTS: CONTAINERS_CA_PATH,
  };
}

export function parseInstance(annotation: string | undefined): InstanceSpec | undefined {
  if (!annotation) return undefined;
  const text = annotation.trim();
  if (!text.startsWith("{")) return text;
  const parsed = JSON.parse(text) as Partial<ContainerInstance>;
  if (typeof parsed.vcpu !== "number" || typeof parsed.memoryMib !== "number" || typeof parsed.diskMb !== "number") {
    throw new Error(`invalid ${INSTANCE_ANNOTATION}: ${annotation}`);
  }
  return { vcpu: parsed.vcpu, memoryMib: parsed.memoryMib, diskMb: parsed.diskMb };
}

export interface ImageRef {
  declared?: string;
  literal?: string;
}

export function parseImageRef(annotation: string | undefined, fallback: string): ImageRef {
  const text = (annotation ?? fallback).trim();
  if (text.startsWith(DECLARED_IMAGE_PREFIX)) return { declared: text.slice(DECLARED_IMAGE_PREFIX.length) };
  return { literal: text };
}

export interface BackoffEntry {
  delayMs: number;
  lastUpdateMs: number;
}

function backoffExpired(entry: BackoffEntry, eventMs: number, maxMs: number): boolean {
  return eventMs - entry.lastUpdateMs > maxMs * 2;
}

export function nextBackoff(entry: BackoffEntry | undefined, eventMs: number, nowMs: number, initialMs: number, maxMs: number): BackoffEntry {
  if (!entry || backoffExpired(entry, eventMs, maxMs)) return { delayMs: initialMs, lastUpdateMs: nowMs };
  return { delayMs: Math.min(entry.delayMs * 2, maxMs), lastUpdateMs: nowMs };
}

export interface RestartPlan {
  startAtMs: number;
  entry: BackoffEntry;
  backoffMs: number;
}

export function planRestart(entry: BackoffEntry | undefined, finishedAtMs: number, nowMs: number): RestartPlan {
  const inBackoff = entry && !backoffExpired(entry, finishedAtMs, MAX_CRASH_LOOP_BACKOFF_MS) && nowMs - finishedAtMs < entry.delayMs;
  const startAtMs = inBackoff ? finishedAtMs + entry.delayMs : nowMs;
  return {
    startAtMs,
    backoffMs: inBackoff ? entry.delayMs : 0,
    entry: nextBackoff(entry, finishedAtMs, startAtMs, INITIAL_CRASH_LOOP_BACKOFF_MS, MAX_CRASH_LOOP_BACKOFF_MS),
  };
}

export function shouldRestart(policy: Pod["spec"]["restartPolicy"], exitCode: number): boolean {
  if (policy === "Never") return false;
  if (policy === "OnFailure") return exitCode !== 0;
  return true;
}

export type StartFailureKind = "transient" | "terminal";

export function classifyStartFailure(message: string): StartFailureKind {
  if (/no container instance that can be provided|temporarily unavailable|try again/i.test(message)) return "transient";
  return "terminal";
}

export interface ExitInfo {
  exitCode: number;
  message: string;
}

export function parseExit(error: unknown): ExitInfo {
  if (error === undefined || error === null) return { exitCode: 0, message: "" };
  const message = error instanceof Error ? error.message : String(error);
  const m = /(?:exit(?:ed)?(?: with)?(?: code)?|code|status)[^0-9-]*(-?\d+)/i.exec(message);
  return { exitCode: m ? Number(m[1]) : 1, message };
}

export function terminatedReason(exitCode: number): string {
  return exitCode === 0 ? "Completed" : "Error";
}

export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rest = s % 60;
  return rest === 0 ? `${m}m0s` : `${m}m${rest}s`;
}

export function ipsInCIDR(cidr: string): { base: number; size: number } {
  const [ip, bitsText] = cidr.split("/");
  const bits = Number(bitsText);
  const parts = ip.split(".").map(Number);
  if (parts.length !== 4 || parts.some((p) => !Number.isInteger(p) || p < 0 || p > 255) || !(bits >= 0 && bits <= 32)) {
    throw new Error(`invalid CIDR ${cidr}`);
  }
  const base = ((parts[0] << 24) | (parts[1] << 16) | (parts[2] << 8) | parts[3]) >>> 0;
  const size = 2 ** (32 - bits);
  return { base: base - (base % size), size };
}

export function ipToString(n: number): string {
  return [n >>> 24, (n >>> 16) & 255, (n >>> 8) & 255, n & 255].join(".");
}

export function pickPodIP(cidr: string, taken: Set<string>): string | null {
  const { base, size } = ipsInCIDR(cidr);
  for (let offset = 2; offset < size - 1; offset++) {
    const ip = ipToString(base + offset);
    if (!taken.has(ip)) return ip;
  }
  return null;
}

export function gatewayIP(cidr: string): string {
  return ipToString(ipsInCIDR(cidr).base + 1);
}

export type ProbeKind = "startup" | "readiness" | "liveness";
export type ProbeResult = "success" | "failure" | "unknown";

export interface ProbeState {
  lastResult?: ProbeResult;
  resultRun: number;
  nextAtMs: number;
}

export interface ProbeSpec {
  initialDelayMs: number;
  periodMs: number;
  timeoutMs: number;
  successThreshold: number;
  failureThreshold: number;
}

export function probeSpec(probe: Probe): ProbeSpec {
  return {
    initialDelayMs: (probe.initialDelaySeconds ?? 0) * 1000,
    periodMs: (probe.periodSeconds || 10) * 1000,
    timeoutMs: (probe.timeoutSeconds || 1) * 1000,
    successThreshold: probe.successThreshold || 1,
    failureThreshold: probe.failureThreshold || 3,
  };
}

export interface ProbeStep {
  state: ProbeState;
  settled?: ProbeResult;
}

export function stepProbe(prev: ProbeState, result: ProbeResult, spec: ProbeSpec, nowMs: number): ProbeStep {
  const resultRun = prev.lastResult === result ? prev.resultRun + 1 : 1;
  const state: ProbeState = { lastResult: result, resultRun, nextAtMs: nowMs + spec.periodMs };
  if ((result === "failure" && resultRun < spec.failureThreshold) || (result === "success" && resultRun < spec.successThreshold)) {
    return { state };
  }
  if (result === "unknown") return { state };
  return { state, settled: result };
}

export function probePort(port: number | string, container: PodContainer): number {
  if (typeof port === "number") return port;
  const named = container.ports?.find((p) => p.name === port);
  if (named) return named.containerPort;
  const n = Number(port);
  if (Number.isInteger(n) && n > 0) return n;
  throw new Error(`probe port ${port} not found`);
}

export interface RunState {
  phase?: "Succeeded" | "Failed";
  reason?: string;
  message?: string;
  podIP?: string;
  hostIP?: string;
  startTime?: string;
  containerID?: string;
  state: ContainerState;
  lastState?: ContainerState;
  restartCount: number;
  ready: boolean;
  started: boolean;
  sandboxReady: boolean;
}

function condition(previous: PodCondition[] | undefined, next: PodCondition, now: string): PodCondition {
  const prev = previous?.find((c) => c.type === next.type);
  const unchanged = prev && prev.status === next.status;
  const out: PodCondition = { ...next, lastTransitionTime: unchanged ? prev.lastTransitionTime ?? now : now, lastProbeTime: null };
  return out;
}

export function podPhase(pod: Pod, run: RunState): string {
  if (run.phase) return run.phase;
  const policy = pod.spec.restartPolicy ?? "Always";
  const { state } = run;
  if (state.running) return "Running";
  const terminated = state.terminated ?? (state.waiting ? run.lastState?.terminated : undefined);
  if (!terminated) return "Pending";
  if (policy === "Always") return "Running";
  if (terminated.exitCode === 0) return "Succeeded";
  if (policy === "Never") return "Failed";
  return "Running";
}

export function buildPodStatus(pod: Pod, run: RunState, nowIso: string): PodStatus {
  const container = pod.spec.containers[0];
  const prev = pod.status;
  const phase = podPhase(pod, run);
  const containerStatus: ContainerStatus = {
    name: container.name,
    image: container.image,
    imageID: "",
    containerID: run.containerID,
    ready: run.ready,
    started: run.started,
    restartCount: run.restartCount,
    state: run.state,
    lastState: run.lastState ?? {},
  };
  const conditions: PodCondition[] = [];
  conditions.push(condition(prev?.conditions, { type: "PodReadyToStartContainers", status: run.sandboxReady ? "True" : "False" }, nowIso));
  conditions.push(condition(prev?.conditions, phase === "Succeeded" ? { type: "Initialized", status: "True", reason: "PodCompleted" } : { type: "Initialized", status: "True" }, nowIso));
  let containersReady: PodCondition;
  if (phase === "Succeeded") containersReady = { type: "ContainersReady", status: "False", reason: "PodCompleted" };
  else if (phase === "Failed") containersReady = { type: "ContainersReady", status: "False", reason: "PodFailed" };
  else if (!run.ready) containersReady = { type: "ContainersReady", status: "False", reason: "ContainersNotReady", message: `containers with unready status: [${container.name}]` };
  else containersReady = { type: "ContainersReady", status: "True" };
  conditions.push(condition(prev?.conditions, { ...containersReady, type: "Ready" }, nowIso));
  conditions.push(condition(prev?.conditions, containersReady, nowIso));
  conditions.push(condition(prev?.conditions, { type: "PodScheduled", status: "True" }, nowIso));
  for (const c of prev?.conditions ?? []) {
    if (!conditions.some((n) => n.type === c.type)) conditions.push(c);
  }
  const status: PodStatus = {
    ...prev,
    phase,
    conditions,
    containerStatuses: [containerStatus],
    startTime: run.startTime ?? prev?.startTime,
  };
  if (run.reason) status.reason = run.reason;
  else delete status.reason;
  if (run.message) status.message = run.message;
  else delete status.message;
  if (run.hostIP) {
    status.hostIP = run.hostIP;
    status.hostIPs = [{ ip: run.hostIP }];
  }
  if (run.podIP) {
    status.podIP = run.podIP;
    status.podIPs = [{ ip: run.podIP }];
  }
  return status;
}

export function terminationGraceMs(pod: Pod): number {
  const seconds = pod.metadata.deletionGracePeriodSeconds ?? pod.spec.terminationGracePeriodSeconds ?? DEFAULT_TERMINATION_GRACE_S;
  return Math.max(0, seconds) * 1000;
}
