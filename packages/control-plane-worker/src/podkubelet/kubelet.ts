import { DurableObject } from "cloudflare:workers";
import { clusterName } from "../clusterid.ts";
import { apiserverFetch } from "../loader.ts";
import { PodAPI } from "./api.ts";
import { routeEgress } from "./egress.ts";
import { declaredImages } from "./images.generated.ts";
import type { PodLedger } from "./ledger.ts";
import {
  API_HOST,
  DEFAULT_POD_CIDR,
  IMAGE_ANNOTATION,
  INITIAL_START_BACKOFF_MS,
  INSTANCE_ANNOTATION,
  MAX_START_BACKOFF_MS,
  VIRTUAL_NODE,
  buildPodStatus,
  classifyStartFailure,
  composeEntrypoint,
  formatDuration,
  gatewayIP,
  imageEnv,
  parseExit,
  parseImageRef,
  parseInstance,
  planRestart,
  platformEnv,
  probePort,
  probeSpec,
  resolveEnv,
  shouldRestart,
  stepProbe,
  terminatedReason,
  terminationGraceMs,
  type BackoffEntry,
  type ContainerState,
  type ContainerStateTerminated,
  type Pod,
  type PodContainer,
  type Probe,
  type ProbeKind,
  type ProbeResult,
  type ProbeState,
  type RunState,
} from "./spec.ts";

export const KEEPALIVE_INTERVAL_MS = 60_000;
const INACTIVITY_TIMEOUT_MS = 6 * 3600_000;
const TOKEN_TTL_S = 3600;
const TOKEN_REFRESH_MARGIN_MS = 300_000;

type Phase = "idle" | "starting" | "running" | "backoff" | "startfailed" | "terminating" | "finished";

interface KubeletState {
  ref?: { namespace: string; name: string };
  pod?: Pod;
  phase: Phase;
  generation: number;
  restartCount: number;
  podIP?: string;
  hostIP?: string;
  startTime?: string;
  containerStartedAt?: string;
  current: ContainerState;
  lastState?: ContainerState;
  terminal?: "Succeeded" | "Failed";
  crashBackoff?: BackoffEntry;
  startBackoffMs?: number;
  ready: boolean;
  started: boolean;
  sandboxReady: boolean;
  probes: Partial<Record<ProbeKind, ProbeState>>;
  killReason?: string;
  alarms: Record<string, number>;
  reported?: string;
}

const platformFailure = /no container instance that can be provided|temporarily unavailable|Invalid custom instance|Image reference must be digest-pinned|exceeds account limits|internal error|has not been started|failed to start|failed to pull|not found in|invalid image/i;

function initialState(): KubeletState {
  return { phase: "idle", generation: 0, restartCount: 0, current: { waiting: { reason: "ContainerCreating" } }, ready: false, started: false, sandboxReady: false, probes: {}, alarms: {} };
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export class PodKubelet extends DurableObject<Env> {
  private state: KubeletState = initialState();
  private readonly api: PodAPI;
  private token?: { token: string; expiresAt: number };
  private monitoring = false;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.api = new PodAPI(env);
    ctx.blockConcurrencyWhile(async () => {
      this.state = (await ctx.storage.get<KubeletState>("state")) ?? initialState();
      if (this.container?.running && (this.state.phase === "running" || this.state.phase === "starting" || this.state.phase === "terminating")) this.attachMonitor();
    });
  }

  private get container(): Container | undefined {
    return this.ctx.container;
  }

  private get uid(): string {
    return this.ctx.id.name ?? "";
  }

  private ledger(): DurableObjectStub<PodLedger> {
    return this.env.POD_LEDGER.get(this.env.POD_LEDGER.idFromName(clusterName(this.env)));
  }

  private async save(): Promise<void> {
    await this.ctx.storage.put("state", this.state);
  }

  private log(message: string): void {
    const ref = this.state.ref ? `${this.state.ref.namespace}/${this.state.ref.name}` : this.uid;
    console.log(`podkubelet ${ref}: ${message}`);
  }

  async reconcile(ref: { namespace: string; name: string }): Promise<{ phase: Phase }> {
    if (!this.state.ref) {
      this.state.ref = ref;
      await this.save();
    }
    await this.sync();
    return { phase: this.state.phase };
  }

  async status(): Promise<{ phase: Phase; running: boolean; podIP?: string; restartCount: number }> {
    return { phase: this.state.phase, running: Boolean(this.container?.running), podIP: this.state.podIP, restartCount: this.state.restartCount };
  }

  async fetch(request: Request): Promise<Response> {
    return routeEgress(request, {
      apiFetch: (req) => apiserverFetch(this.env, req),
      serviceAccountToken: () => this.serviceAccountToken(),
    });
  }

  private async serviceAccountToken(): Promise<string | null> {
    const pod = this.state.pod;
    if (!pod) return null;
    if (this.token && this.token.expiresAt - Date.now() > TOKEN_REFRESH_MARGIN_MS) return this.token.token;
    const minted = await this.api.serviceAccountToken(pod, TOKEN_TTL_S).catch((err) => {
      this.log(`token request failed: ${errorText(err)}`);
      return null;
    });
    if (!minted) return null;
    this.token = minted;
    return minted.token;
  }

  private async sync(): Promise<void> {
    const ref = this.state.ref;
    if (!ref) return;
    const pod = await this.api.getPod(ref.namespace, ref.name);
    if (!pod || pod.metadata.uid !== this.uid) {
      await this.podGone();
      return;
    }
    if (pod.spec.nodeName !== VIRTUAL_NODE) return;
    this.state.pod = pod;
    if (pod.metadata.deletionTimestamp) {
      await this.beginTermination(pod);
      return;
    }
    switch (this.state.phase) {
      case "idle":
        await this.start(pod);
        return;
      case "running":
      case "starting":
        if (!this.container?.running && !this.monitoring) {
          await this.onExit(new Error("container is not running: ContainerStatusUnknown"));
          return;
        }
        break;
      case "startfailed":
        if (this.state.alarms.startRetry === undefined) {
          await this.start(pod);
          return;
        }
        break;
    }
    await this.writeStatus(pod);
  }

  private async podGone(): Promise<void> {
    this.log("pod is gone, destroying container");
    await this.destroyContainer();
    await this.ledger().release(this.uid).catch(() => {});
    await this.ctx.storage.deleteAlarm();
    await this.ctx.storage.deleteAll();
    this.state = initialState();
  }

  private async destroyContainer(): Promise<void> {
    if (!this.container) return;
    try {
      await this.container.destroy();
    } catch (err) {
      this.log(`destroy: ${errorText(err)}`);
    }
  }

  private async ensureNetwork(pod: Pod): Promise<void> {
    if (this.state.podIP && this.state.hostIP) return;
    const node = await this.api.getNode(VIRTUAL_NODE);
    const cidr = node?.spec?.podCIDR ?? node?.spec?.podCIDRs?.[0] ?? DEFAULT_POD_CIDR;
    const entry = await this.ledger().claim(this.uid, pod.metadata.namespace, pod.metadata.name, cidr);
    this.state.podIP = entry.podIP;
    this.state.hostIP = node?.status?.addresses?.find((a) => a.type === "InternalIP")?.address ?? gatewayIP(cidr);
  }

  private async start(pod: Pod): Promise<void> {
    const container = this.container;
    if (!container) throw new Error("PodKubelet has no container binding");
    const spec = pod.spec.containers[0];
    const fieldPath = `spec.containers{${spec.name}}`;
    const now = new Date();
    this.state.phase = "starting";
    this.state.terminal = undefined;
    this.state.killReason = undefined;
    this.state.current = { waiting: { reason: "ContainerCreating" } };
    this.state.ready = false;
    this.state.started = false;
    this.state.probes = {};
    delete this.state.alarms.startRetry;
    delete this.state.alarms.restart;
    await this.save();

    let startOptions: ContainerStartupOptions;
    try {
      await this.ensureNetwork(pod);
      const image = parseImageRef(pod.metadata.annotations?.[IMAGE_ANNOTATION], spec.image);
      const imageRef = image.declared ? container.images[image.declared] : image.literal;
      if (!imageRef) throw new Error(`image ${image.declared} is not declared in this deployment`);
      const meta = image.declared ? declaredImages[image.declared] : undefined;
      const podEnv = await resolveEnv({ pod, container: spec, podIP: this.state.podIP!, hostIP: this.state.hostIP! }, {
        configMap: (name) => this.api.configMapData(pod.metadata.namespace, name),
        secret: (name) => this.api.secretData(pod.metadata.namespace, name),
      });
      const env = { ...imageEnv(meta), ...platformEnv(), ...podEnv };
      const entrypoint = composeEntrypoint(spec, meta, env);
      startOptions = { image: imageRef, entrypoint, env, enableInternet: true, instance: parseInstance(pod.metadata.annotations?.[INSTANCE_ANNOTATION]) as ContainerStartupOptions["instance"] };
    } catch (err) {
      await this.failStart(pod, "CreateContainerConfigError", errorText(err), false);
      return;
    }

    try {
      container.start(startOptions);
    } catch (err) {
      const message = errorText(err);
      await this.failStart(pod, "CreateContainerError", message, classifyStartFailure(message) === "transient");
      return;
    }
    this.state.generation++;
    this.state.phase = "running";
    this.state.sandboxReady = true;
    this.state.startTime ??= now.toISOString();
    this.state.containerStartedAt = now.toISOString();
    this.state.current = { running: { startedAt: now.toISOString() } };
    this.state.started = !spec.startupProbe;
    this.state.ready = this.state.started && !spec.readinessProbe;
    this.state.alarms.keepalive = Date.now() + KEEPALIVE_INTERVAL_MS;
    this.armProbes(spec, Date.now());
    await this.save();
    this.attachMonitor();
    await this.ledger().setRunning(this.uid, true).catch(() => {});
    const self = this.env.POD_KUBELET.get(this.ctx.id) as unknown as Fetcher;
    try {
      await container.setInactivityTimeout(INACTIVITY_TIMEOUT_MS);
      await container.interceptAllOutboundHttp(self);
      await container.interceptOutboundHttps(`${API_HOST}:443`, self);
    } catch (err) {
      this.log(`container setup: ${errorText(err)}`);
    }
    await this.scheduleAlarm();
    this.log(`started generation ${this.state.generation} restarts=${this.state.restartCount}`);
    await this.api.event(pod, "Normal", "Started", `Started container ${spec.name}`, fieldPath).catch(() => {});
    await this.writeStatus(pod);
  }

  private async failStart(pod: Pod, reason: string, message: string, transient: boolean): Promise<void> {
    const spec = pod.spec.containers[0];
    this.state.phase = "startfailed";
    this.state.current = { waiting: { reason, message } };
    this.state.ready = false;
    this.state.started = false;
    if (transient) {
      const delay = this.state.startBackoffMs ? Math.min(this.state.startBackoffMs * 2, MAX_START_BACKOFF_MS) : INITIAL_START_BACKOFF_MS;
      this.state.startBackoffMs = delay;
      this.state.alarms.startRetry = Date.now() + delay;
      await this.api.event(pod, "Warning", "FailedCreatePodSandBox", `Failed to create pod sandbox: ${message} (retrying in ${formatDuration(delay)})`).catch(() => {});
    } else {
      delete this.state.alarms.startRetry;
      await this.api.event(pod, "Warning", "Failed", `Error: ${message}`, `spec.containers{${spec.name}}`).catch(() => {});
    }
    await this.save();
    await this.scheduleAlarm();
    this.log(`start failed (${reason}, transient=${transient}): ${message}`);
    await this.writeStatus(pod);
  }

  private attachMonitor(): void {
    const container = this.container;
    if (!container || this.monitoring) return;
    const generation = this.state.generation;
    this.monitoring = true;
    container.monitor().then(
      () => this.exited(generation, undefined),
      (err) => this.exited(generation, err),
    );
  }

  private exited(generation: number, err: unknown): void {
    this.monitoring = false;
    if (generation !== this.state.generation) return;
    this.ctx.waitUntil(this.onExit(err).catch((e) => this.log(`onExit: ${errorText(e)}`)));
  }

  private async onExit(err: unknown): Promise<void> {
    const pod = this.state.pod;
    const now = Date.now();
    const finishedAt = new Date(now).toISOString();
    const message = err === undefined ? "" : errorText(err);
    this.state.startBackoffMs = undefined;
    await this.ledger().setRunning(this.uid, false).catch(() => {});
    delete this.state.alarms.keepalive;
    delete this.state.alarms.kill;
    for (const kind of ["startup", "readiness", "liveness"] as ProbeKind[]) delete this.state.alarms[`probe:${kind}`];

    if (this.state.phase === "terminating") {
      const exit = parseExit(err);
      this.state.current = { terminated: this.terminated(exit.exitCode, exit.message, finishedAt) };
      await this.save();
      await this.finalizeTermination();
      return;
    }

    if (message && !/exit/i.test(message) && platformFailure.test(message) && !this.state.killReason) {
      const transient = classifyStartFailure(message) === "transient";
      if (pod) await this.failStart(pod, transient ? "ContainerCreating" : "CreateContainerError", message, transient);
      return;
    }

    const exit = parseExit(err);
    const terminated = this.terminated(exit.exitCode, this.state.killReason ?? exit.message, finishedAt);
    this.state.lastState = { terminated };
    this.state.ready = false;
    this.state.started = false;
    this.state.probes = {};
    this.state.killReason = undefined;
    const policy = pod?.spec.restartPolicy ?? "Always";
    const spec = pod?.spec.containers[0];
    this.log(`exited code=${exit.exitCode} restarts=${this.state.restartCount} policy=${policy}`);
    if (!spec || !shouldRestart(policy, exit.exitCode)) {
      this.state.phase = "finished";
      this.state.current = { terminated };
      this.state.lastState = undefined;
      this.state.terminal = exit.exitCode === 0 ? "Succeeded" : "Failed";
      await this.save();
      await this.destroyContainer();
      await this.scheduleAlarm();
      if (pod) await this.writeStatus(pod);
      return;
    }
    const plan = planRestart(this.state.crashBackoff, now, now);
    this.state.crashBackoff = plan.entry;
    if (plan.startAtMs > now) {
      this.state.phase = "backoff";
      this.state.current = { waiting: { reason: "CrashLoopBackOff", message: `back-off ${formatDuration(plan.backoffMs)} restarting failed container=${spec.name} pod=${pod.metadata.name}_${pod.metadata.namespace}(${pod.metadata.uid})` } };
      this.state.alarms.restart = plan.startAtMs;
      await this.save();
      await this.scheduleAlarm();
      await this.api.event(pod, "Warning", "BackOff", `Back-off restarting failed container ${spec.name} in pod ${pod.metadata.name}_${pod.metadata.namespace}(${pod.metadata.uid})`, `spec.containers{${spec.name}}`).catch(() => {});
      await this.writeStatus(pod);
      return;
    }
    this.state.restartCount++;
    this.state.phase = "idle";
    await this.save();
    await this.start(pod);
  }

  private terminated(exitCode: number, message: string, finishedAt: string): ContainerStateTerminated {
    return {
      exitCode,
      reason: terminatedReason(exitCode),
      message: message || undefined,
      startedAt: this.state.containerStartedAt,
      finishedAt,
      containerID: this.containerID(),
    };
  }

  private containerID(): string | undefined {
    return this.state.generation > 0 ? `cloudflare://${this.uid}/${this.state.generation}` : undefined;
  }

  private async beginTermination(pod: Pod): Promise<void> {
    if (this.state.phase === "terminating") {
      await this.writeStatus(pod);
      return;
    }
    const spec = pod.spec.containers[0];
    const wasRunning = this.state.phase === "running" && Boolean(this.container?.running);
    this.state.phase = "terminating";
    delete this.state.alarms.restart;
    delete this.state.alarms.startRetry;
    for (const kind of ["startup", "readiness", "liveness"] as ProbeKind[]) delete this.state.alarms[`probe:${kind}`];
    this.state.ready = false;
    await this.save();
    this.log(`terminating (running=${wasRunning}, grace=${terminationGraceMs(pod)}ms)`);
    await this.api.event(pod, "Normal", "Killing", `Stopping container ${spec.name}`, `spec.containers{${spec.name}}`).catch(() => {});
    if (wasRunning) {
      this.signal(15);
      this.state.alarms.kill = Date.now() + terminationGraceMs(pod);
      await this.save();
      await this.scheduleAlarm();
      await this.writeStatus(pod);
      return;
    }
    if (!this.state.current.terminated) {
      const exit = this.state.lastState?.terminated;
      this.state.current = exit ? { terminated: exit } : { terminated: this.terminated(137, "pod deleted before the container started", new Date().toISOString()) };
    }
    await this.save();
    await this.finalizeTermination();
  }

  private async finalizeTermination(): Promise<void> {
    const ref = this.state.ref;
    const pod = this.state.pod;
    await this.destroyContainer();
    if (pod) {
      const exitCode = this.state.current.terminated?.exitCode ?? 137;
      this.state.terminal = exitCode === 0 ? "Succeeded" : "Failed";
      this.state.phase = "finished";
      await this.save();
      await this.writeStatus(pod).catch((err) => this.log(`final status: ${errorText(err)}`));
    }
    if (ref) await this.api.deletePod(ref.namespace, ref.name, this.uid).catch((err) => this.log(`final delete: ${errorText(err)}`));
    await this.ledger().release(this.uid).catch(() => {});
    await this.ctx.storage.deleteAlarm();
    await this.ctx.storage.deleteAll();
    this.state = initialState();
    this.log("terminated and removed");
  }

  private signal(signo: number): void {
    try {
      this.container?.signal(signo);
    } catch (err) {
      this.log(`signal ${signo}: ${errorText(err)}`);
    }
  }

  private async killContainer(pod: Pod, reason: string): Promise<void> {
    if (this.state.phase !== "running") return;
    this.state.killReason = reason;
    this.state.alarms.kill = Date.now() + terminationGraceMs(pod);
    for (const kind of ["startup", "readiness", "liveness"] as ProbeKind[]) delete this.state.alarms[`probe:${kind}`];
    await this.save();
    this.signal(15);
    await this.scheduleAlarm();
  }

  private armProbes(spec: PodContainer, nowMs: number): void {
    const arm = (kind: ProbeKind, probe: Probe | undefined) => {
      if (!probe) return;
      const p = probeSpec(probe);
      this.state.probes[kind] = { resultRun: 0, nextAtMs: nowMs + p.initialDelayMs };
      this.state.alarms[`probe:${kind}`] = nowMs + p.initialDelayMs;
    };
    arm("startup", spec.startupProbe);
    if (!spec.startupProbe) {
      arm("readiness", spec.readinessProbe);
      arm("liveness", spec.livenessProbe);
    }
  }

  private async runProbe(kind: ProbeKind, pod: Pod): Promise<void> {
    const spec = pod.spec.containers[0];
    const probe = spec[`${kind}Probe` as const];
    const state = this.state.probes[kind];
    if (!probe || !state || this.state.phase !== "running" || this.state.killReason) return;
    const p = probeSpec(probe);
    const result = await this.probeOnce(probe, spec, p.timeoutMs);
    const step = stepProbe(state, result.result, p, Date.now());
    this.state.probes[kind] = step.state;
    this.state.alarms[`probe:${kind}`] = step.state.nextAtMs;
    if (step.settled === undefined) {
      await this.save();
      return;
    }
    const fieldPath = `spec.containers{${spec.name}}`;
    if (step.settled === "failure") {
      await this.api.event(pod, "Warning", "Unhealthy", `${kind[0].toUpperCase()}${kind.slice(1)} probe failed: ${result.message}`, fieldPath).catch(() => {});
    }
    if (kind === "readiness") {
      this.state.ready = step.settled === "success" && this.state.started;
    } else if (kind === "startup") {
      if (step.settled === "success") {
        this.state.started = true;
        this.state.ready = !spec.readinessProbe;
        delete this.state.probes.startup;
        delete this.state.alarms["probe:startup"];
        const now = Date.now();
        const arm = (k: ProbeKind, pr: Probe | undefined) => {
          if (!pr) return;
          this.state.probes[k] = { resultRun: 0, nextAtMs: now };
          this.state.alarms[`probe:${k}`] = now;
        };
        arm("readiness", spec.readinessProbe);
        arm("liveness", spec.livenessProbe);
      } else {
        await this.api.event(pod, "Normal", "Killing", `Container ${spec.name} failed startup probe, will be restarted`, fieldPath).catch(() => {});
        await this.killContainer(pod, "startup probe failed");
      }
    } else if (step.settled === "failure") {
      await this.api.event(pod, "Normal", "Killing", `Container ${spec.name} failed liveness probe, will be restarted`, fieldPath).catch(() => {});
      await this.killContainer(pod, "liveness probe failed");
    }
    await this.save();
    await this.writeStatus(pod);
  }

  private async probeOnce(probe: Probe, spec: PodContainer, timeoutMs: number): Promise<{ result: ProbeResult; message: string }> {
    const container = this.container;
    if (!container) return { result: "unknown", message: "no container" };
    try {
      if (probe.httpGet) {
        const port = probePort(probe.httpGet.port, spec);
        const headers = new Headers({ "User-Agent": "kube-probe/1.0", Accept: "*/*" });
        for (const h of probe.httpGet.httpHeaders ?? []) headers.set(h.name, h.value);
        const resp = await container.getTcpPort(port).fetch(`http://${probe.httpGet.host ?? "pod"}:${port}${probe.httpGet.path ?? "/"}`, { headers, signal: AbortSignal.timeout(timeoutMs) });
        const body = (await resp.text()).slice(0, 10 * 1024);
        if (resp.status >= 200 && resp.status < 400) return { result: "success", message: body };
        return { result: "failure", message: `HTTP probe failed with statuscode: ${resp.status}` };
      }
      if (probe.tcpSocket) {
        const port = probePort(probe.tcpSocket.port, spec);
        const socket = container.getTcpPort(port).connect(`pod:${port}`);
        await Promise.race([socket.opened, new Promise((_, reject) => setTimeout(() => reject(new Error("dial timeout")), timeoutMs))]);
        await socket.close();
        return { result: "success", message: "" };
      }
      if (probe.exec) {
        const proc = await container.exec(probe.exec.command, { stdout: "pipe", stderr: "pipe" });
        const timer = setTimeout(() => proc.kill(9), timeoutMs);
        const out = await proc.output();
        clearTimeout(timer);
        const text = new TextDecoder().decode(out.stdout) + new TextDecoder().decode(out.stderr);
        if (out.exitCode === 0) return { result: "success", message: text };
        return { result: "failure", message: text.trim() || `command exited with ${out.exitCode}` };
      }
      return { result: "unknown", message: "probe has no handler" };
    } catch (err) {
      return { result: "failure", message: errorText(err) };
    }
  }

  private async keepalive(): Promise<void> {
    const container = this.container;
    if (!container) return;
    if (this.state.phase !== "running") return;
    if (!container.running) {
      if (!this.monitoring) await this.onExit(new Error("container is not running: ContainerStatusUnknown"));
      return;
    }
    try {
      await container.setInactivityTimeout(INACTIVITY_TIMEOUT_MS);
    } catch (err) {
      this.log(`keepalive: ${errorText(err)}`);
    }
    this.state.alarms.keepalive = Date.now() + KEEPALIVE_INTERVAL_MS;
    await this.save();
  }

  private async scheduleAlarm(): Promise<void> {
    const times = Object.values(this.state.alarms);
    if (times.length === 0) {
      await this.ctx.storage.deleteAlarm();
      return;
    }
    await this.ctx.storage.setAlarm(Math.max(Date.now(), Math.min(...times)));
  }

  async alarm(): Promise<void> {
    const now = Date.now();
    const due = Object.entries(this.state.alarms).filter(([, at]) => at <= now).map(([name]) => name);
    const pod = this.state.pod;
    for (const name of due) {
      if (name === "keepalive") {
        await this.keepalive();
      } else if (name === "restart" || name === "startRetry") {
        delete this.state.alarms[name];
        if (name === "restart") this.state.restartCount++;
        this.state.phase = "idle";
        await this.save();
        await this.sync();
      } else if (name === "kill") {
        delete this.state.alarms.kill;
        await this.save();
        if (this.container?.running) this.signal(9);
      } else if (name.startsWith("probe:") && pod) {
        await this.runProbe(name.slice("probe:".length) as ProbeKind, pod);
      } else {
        delete this.state.alarms[name];
        await this.save();
      }
    }
    await this.scheduleAlarm();
  }

  private runState(): RunState {
    return {
      phase: this.state.terminal,
      podIP: this.state.podIP,
      hostIP: this.state.hostIP,
      startTime: this.state.startTime,
      containerID: this.containerID(),
      state: this.state.current,
      lastState: this.state.lastState,
      restartCount: this.state.restartCount,
      ready: this.state.ready,
      started: this.state.started,
      sandboxReady: this.state.sandboxReady,
    };
  }

  private async writeStatus(pod: Pod): Promise<void> {
    const status = buildPodStatus(pod, this.runState(), new Date().toISOString());
    const encoded = JSON.stringify(status);
    if (encoded === this.state.reported) return;
    const written = await this.api.putStatus(pod, status);
    if (!written) return;
    this.state.pod = written;
    this.state.reported = encoded;
    await this.save();
  }
}
