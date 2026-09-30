import assert from "node:assert/strict";
import { test } from "node:test";
import { routeEgress } from "../src/podkubelet/egress.ts";
import {
  INITIAL_CRASH_LOOP_BACKOFF_MS,
  MAX_CRASH_LOOP_BACKOFF_MS,
  buildPodStatus,
  classifyStartFailure,
  composeEntrypoint,
  expand,
  gatewayIP,
  mappingFor,
  parseExit,
  parseImageRef,
  parseInstance,
  parseQuantity,
  pickPodIP,
  planRestart,
  podPhase,
  probeSpec,
  resolveEnv,
  stepProbe,
  terminationGraceMs,
  type ImageMeta,
  type Pod,
  type ProbeState,
  type RunState,
} from "../src/podkubelet/spec.ts";

const image: ImageMeta = { entrypoint: ["httpd"], cmd: ["-f", "-p", "8080"], workingDir: "", user: "", env: ["PATH=/bin"] };

function pod(overrides: Partial<Pod["spec"]> = {}, metadata: Partial<Pod["metadata"]> = {}): Pod {
  return {
    metadata: { name: "web", namespace: "default", uid: "u1", ...metadata },
    spec: { containers: [{ name: "app", image: "images/httpd", resources: { requests: { cpu: "250m", memory: "64Mi" }, limits: { cpu: "1500m", memory: "512Mi" } } }], ...overrides },
  };
}

test("expand follows the kubelet's $(VAR) expansion rules", () => {
  const mapping = mappingFor({ A: "x", B: "y" });
  assert.equal(expand("$(A)-$(B)", mapping), "x-y");
  assert.equal(expand("$$(A)", mapping), "$(A)");
  assert.equal(expand("$(MISSING)", mapping), "$(MISSING)");
  assert.equal(expand("$(A", mapping), "$(A");
  assert.equal(expand("cost $5", mapping), "cost $5");
  assert.equal(expand("trailing$", mapping), "trailing$");
});

test("composeEntrypoint applies command/args over the image's ENTRYPOINT/CMD", () => {
  const env = { PORT: "9090" };
  assert.deepEqual(composeEntrypoint({ name: "a", image: "i" }, image, env), ["httpd", "-f", "-p", "8080"]);
  assert.deepEqual(composeEntrypoint({ name: "a", image: "i", args: ["-p", "$(PORT)"] }, image, env), ["httpd", "-p", "9090"]);
  assert.deepEqual(composeEntrypoint({ name: "a", image: "i", command: ["sh", "-c"], args: ["echo $(PORT)"] }, image, env), ["sh", "-c", "echo 9090"]);
  assert.deepEqual(composeEntrypoint({ name: "a", image: "i", command: ["sleep", "1"] }, image, env), ["sleep", "1"]);
  assert.deepEqual(composeEntrypoint({ name: "a", image: "i", command: ["sleep"] }, undefined, env), ["sleep"]);
  assert.throws(() => composeEntrypoint({ name: "a", image: "i", args: ["x"] }, undefined, env), /set spec.containers\[\].command/);
  assert.throws(() => composeEntrypoint({ name: "a", image: "i" }, { ...image, entrypoint: [], cmd: [] }, env), /neither ENTRYPOINT nor CMD/);
});

test("resolveEnv layers envFrom, literal, references and downward API values", async () => {
  const p = pod({ serviceAccountName: "svc" }, { labels: { tier: "web" } });
  p.spec.containers[0].envFrom = [{ prefix: "CM_", configMapRef: { name: "cm" } }, { secretRef: { name: "missing", optional: true } }];
  p.spec.containers[0].env = [
    { name: "PLAIN", value: "v" },
    { name: "DERIVED", value: "$(PLAIN)/$(CM_a)" },
    { name: "FROM_CM", valueFrom: { configMapKeyRef: { name: "cm", key: "a" } } },
    { name: "FROM_SECRET", valueFrom: { secretKeyRef: { name: "sec", key: "pw" } } },
    { name: "SKIPPED", valueFrom: { secretKeyRef: { name: "sec", key: "nope", optional: true } } },
    { name: "POD_NAME", valueFrom: { fieldRef: { fieldPath: "metadata.name" } } },
    { name: "POD_IP", valueFrom: { fieldRef: { fieldPath: "status.podIP" } } },
    { name: "NODE", valueFrom: { fieldRef: { fieldPath: "spec.nodeName" } } },
    { name: "SA", valueFrom: { fieldRef: { fieldPath: "spec.serviceAccountName" } } },
    { name: "TIER", valueFrom: { fieldRef: { fieldPath: "metadata.labels['tier']" } } },
    { name: "CPU_LIMIT", valueFrom: { resourceFieldRef: { resource: "limits.cpu" } } },
    { name: "CPU_MILLI", valueFrom: { resourceFieldRef: { resource: "limits.cpu", divisor: "1m" } } },
    { name: "MEM_REQ", valueFrom: { resourceFieldRef: { resource: "requests.memory", divisor: "1Mi" } } },
  ];
  const env = await resolveEnv({ pod: p, container: p.spec.containers[0], podIP: "10.42.255.7", hostIP: "10.42.255.1" }, {
    configMap: async (name) => (name === "cm" ? { a: "1", "bad key": "x" } : null),
    secret: async (name) => (name === "sec" ? { pw: "hunter2" } : null),
  });
  assert.deepEqual(env, {
    CM_a: "1",
    PLAIN: "v",
    DERIVED: "v/1",
    FROM_CM: "1",
    FROM_SECRET: "hunter2",
    POD_NAME: "web",
    POD_IP: "10.42.255.7",
    NODE: "cloudflare",
    SA: "svc",
    TIER: "web",
    CPU_LIMIT: "2",
    CPU_MILLI: "1500",
    MEM_REQ: "64",
  });
});

test("resolveEnv fails on a required key that does not exist", async () => {
  const p = pod();
  p.spec.containers[0].env = [{ name: "X", valueFrom: { configMapKeyRef: { name: "cm", key: "zz" } } }];
  await assert.rejects(
    resolveEnv({ pod: p, container: p.spec.containers[0], podIP: "", hostIP: "" }, { configMap: async () => ({ a: "1" }), secret: async () => null }),
    /couldn't find key zz in ConfigMap default\/cm/,
  );
  await assert.rejects(
    resolveEnv({ pod: p, container: p.spec.containers[0], podIP: "", hostIP: "" }, { configMap: async () => null, secret: async () => null }),
    /configmap "cm" not found/,
  );
});

test("parseQuantity handles binary and decimal suffixes", () => {
  assert.equal(parseQuantity("64Mi"), 64 * 1024 * 1024);
  assert.equal(parseQuantity("1500m"), 1.5);
  assert.equal(parseQuantity("2G"), 2e9);
  assert.equal(parseQuantity("3"), 3);
});

test("planRestart follows the kubelet crash-loop schedule: 10s doubling to 5m, reset after a 10m run", () => {
  let entry = undefined as ReturnType<typeof planRestart>["entry"] | undefined;
  let now = 1_000_000;
  const first = planRestart(entry, now, now);
  assert.equal(first.startAtMs, now);
  assert.equal(first.entry.delayMs, INITIAL_CRASH_LOOP_BACKOFF_MS);
  entry = first.entry;
  const delays: number[] = [];
  for (let i = 0; i < 7; i++) {
    now = entry.lastUpdateMs + 500;
    const plan = planRestart(entry, now, now);
    delays.push(plan.startAtMs - now);
    entry = plan.entry;
  }
  assert.deepEqual(delays, [10_000, 20_000, 40_000, 80_000, 160_000, 300_000, 300_000]);
  assert.equal(entry.delayMs, MAX_CRASH_LOOP_BACKOFF_MS);
  const longRun = entry.lastUpdateMs + 2 * MAX_CRASH_LOOP_BACKOFF_MS + 1;
  const reset = planRestart(entry, longRun, longRun);
  assert.equal(reset.startAtMs, longRun);
  assert.equal(reset.entry.delayMs, INITIAL_CRASH_LOOP_BACKOFF_MS);
});

test("stepProbe settles only after the thresholds and keeps the run count on repeats", () => {
  const spec = probeSpec({ periodSeconds: 5, failureThreshold: 3, successThreshold: 2 });
  let state: ProbeState = { resultRun: 0, nextAtMs: 0 };
  let step = stepProbe(state, "failure", spec, 1000);
  assert.equal(step.settled, undefined);
  step = stepProbe(step.state, "failure", spec, 2000);
  assert.equal(step.settled, undefined);
  step = stepProbe(step.state, "failure", spec, 3000);
  assert.equal(step.settled, "failure");
  assert.equal(step.state.nextAtMs, 8000);
  step = stepProbe(step.state, "success", spec, 9000);
  assert.equal(step.settled, undefined);
  step = stepProbe(step.state, "success", spec, 10_000);
  assert.equal(step.settled, "success");
  assert.equal(probeSpec({}).periodMs, 10_000);
  assert.equal(probeSpec({}).failureThreshold, 3);
});

function run(overrides: Partial<RunState>): RunState {
  return { state: { waiting: { reason: "ContainerCreating" } }, restartCount: 0, ready: false, started: false, sandboxReady: false, ...overrides };
}

test("podPhase mirrors getPhase for a single app container", () => {
  assert.equal(podPhase(pod(), run({})), "Pending");
  assert.equal(podPhase(pod(), run({ state: { running: {} } })), "Running");
  const crashed = run({ state: { waiting: { reason: "CrashLoopBackOff" } }, lastState: { terminated: { exitCode: 1 } } });
  assert.equal(podPhase(pod(), crashed), "Running");
  assert.equal(podPhase(pod({ restartPolicy: "Never" }), run({ state: { terminated: { exitCode: 1 } } })), "Failed");
  assert.equal(podPhase(pod({ restartPolicy: "Never" }), run({ state: { terminated: { exitCode: 0 } } })), "Succeeded");
  assert.equal(podPhase(pod({ restartPolicy: "OnFailure" }), run({ state: { terminated: { exitCode: 0 } } })), "Succeeded");
  assert.equal(podPhase(pod({ restartPolicy: "OnFailure" }), run({ state: { terminated: { exitCode: 3 } } })), "Running");
  assert.equal(podPhase(pod(), run({ phase: "Failed", state: { terminated: { exitCode: 143 } } })), "Failed");
});

test("buildPodStatus writes conditions, container status and addresses; transition times survive unchanged statuses", () => {
  const p = pod();
  const t1 = "2026-10-01T00:00:00.000Z";
  const running = run({ state: { running: { startedAt: t1 } }, ready: true, started: true, sandboxReady: true, podIP: "10.42.255.5", hostIP: "10.42.255.1", startTime: t1, containerID: "cloudflare://u1/1" });
  const first = buildPodStatus(p, running, t1);
  assert.equal(first.phase, "Running");
  assert.equal(first.podIP, "10.42.255.5");
  assert.deepEqual(first.podIPs, [{ ip: "10.42.255.5" }]);
  assert.equal(first.hostIP, "10.42.255.1");
  assert.deepEqual(
    first.conditions?.map((c) => [c.type, c.status]),
    [["PodReadyToStartContainers", "True"], ["Initialized", "True"], ["Ready", "True"], ["ContainersReady", "True"], ["PodScheduled", "True"]],
  );
  assert.equal(first.containerStatuses?.[0].containerID, "cloudflare://u1/1");
  assert.equal(first.containerStatuses?.[0].ready, true);
  const t2 = "2026-10-01T00:05:00.000Z";
  const second = buildPodStatus({ ...p, status: first }, running, t2);
  assert.equal(second.conditions?.find((c) => c.type === "Ready")?.lastTransitionTime, t1);
  const unready = buildPodStatus({ ...p, status: second }, { ...running, ready: false }, t2);
  const ready = unready.conditions?.find((c) => c.type === "Ready");
  assert.equal(ready?.status, "False");
  assert.equal(ready?.reason, "ContainersNotReady");
  assert.equal(ready?.message, "containers with unready status: [app]");
  assert.equal(ready?.lastTransitionTime, t2);
  const done = buildPodStatus(pod({ restartPolicy: "Never" }), run({ state: { terminated: { exitCode: 0, reason: "Completed" } }, sandboxReady: true }), t2);
  assert.equal(done.phase, "Succeeded");
  assert.equal(done.conditions?.find((c) => c.type === "ContainersReady")?.reason, "PodCompleted");
  assert.equal(done.conditions?.find((c) => c.type === "Initialized")?.reason, "PodCompleted");
});

test("pod IPs come from the node's CIDR, skipping the network, gateway and broadcast addresses", () => {
  assert.equal(pickPodIP("10.42.255.0/24", new Set()), "10.42.255.2");
  assert.equal(pickPodIP("10.42.255.0/24", new Set(["10.42.255.2", "10.42.255.3"])), "10.42.255.4");
  assert.equal(gatewayIP("10.42.255.0/24"), "10.42.255.1");
  const full = new Set<string>();
  for (let i = 2; i < 255; i++) full.add(`10.42.255.${i}`);
  assert.equal(pickPodIP("10.42.255.0/24", full), null);
  assert.equal(pickPodIP("10.42.7.9/24", new Set()), "10.42.7.2");
});

test("annotations select the instance shape and the image", () => {
  assert.equal(parseInstance("lite"), "lite");
  assert.deepEqual(parseInstance('{"vcpu":1.5,"memoryMib":4608,"diskMb":8000}'), { vcpu: 1.5, memoryMib: 4608, diskMb: 8000 });
  assert.equal(parseInstance(undefined), undefined);
  assert.throws(() => parseInstance('{"vcpu":1}'), /invalid containers.k8flare.com\/instance/);
  assert.deepEqual(parseImageRef("images/httpd", "x"), { declared: "httpd" });
  assert.deepEqual(parseImageRef(undefined, "cloudflare/debian-trixie"), { literal: "cloudflare/debian-trixie" });
  assert.deepEqual(parseImageRef("registry.cloudflare.com/acc/web@sha256:abc", "x"), { literal: "registry.cloudflare.com/acc/web@sha256:abc" });
});

test("start failures are transient only for capacity messages; exits carry their code", () => {
  assert.equal(classifyStartFailure("There is no container instance that can be provided at this time"), "transient");
  assert.equal(classifyStartFailure("connection temporarily unavailable"), "transient");
  assert.equal(classifyStartFailure("Invalid custom instance: memoryMib must be at least 3072 per vcpu"), "terminal");
  assert.equal(classifyStartFailure("Image reference must be digest-pinned"), "terminal");
  assert.deepEqual(parseExit(undefined), { exitCode: 0, message: "" });
  assert.equal(parseExit(new Error("container exited with code 137")).exitCode, 137);
  assert.equal(parseExit(new Error("Container exited with a non-zero exit code: 2")).exitCode, 2);
  assert.equal(parseExit(new Error("something else happened")).exitCode, 1);
});

test("termination grace comes from the deletion, then the spec, then the 30s default", () => {
  assert.equal(terminationGraceMs(pod()), 30_000);
  assert.equal(terminationGraceMs(pod({ terminationGracePeriodSeconds: 5 })), 5_000);
  assert.equal(terminationGraceMs(pod({ terminationGracePeriodSeconds: 5 }, { deletionGracePeriodSeconds: 0 })), 0);
});

test("routeEgress attaches the ServiceAccount token to API traffic and passes the rest through", async () => {
  const seen: Request[] = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    seen.push(input instanceof Request ? input : new Request(input));
    return new Response("passthrough");
  }) as typeof fetch;
  try {
    const api: Request[] = [];
    const deps = {
      apiFetch: async (req: Request) => {
        api.push(req);
        return new Response("api");
      },
      serviceAccountToken: async () => "sa-token",
    };
    const res = await routeEgress(new Request("https://kubernetes.default.svc/api/v1/namespaces/default/pods?limit=1", { headers: { Authorization: "Bearer from-container", Connection: "keep-alive" } }), deps);
    assert.equal(await res.text(), "api");
    assert.equal(api[0].url, "https://kubernetes.default.svc/api/v1/namespaces/default/pods?limit=1");
    assert.equal(api[0].headers.get("Authorization"), "Bearer sa-token");
    assert.equal(api[0].headers.get("Connection"), null);
    const out = await routeEgress(new Request("http://example.com/index.html"), deps);
    assert.equal(await out.text(), "passthrough");
    assert.equal(seen[0].url, "http://example.com/index.html");
    const targeted = await routeEgress(new Request("http://10.43.0.10/"), { ...deps, clusterTarget: async (host, port) => (host === "10.43.0.10" && port === 80 ? ({ fetch: async () => new Response("pod") } as unknown as Fetcher) : null) });
    assert.equal(await targeted.text(), "pod");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
