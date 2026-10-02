import type { QueueMessage } from "@k8flare/cluster-store";
import { Trace } from "./otel";
import { apiserverFetch } from "./loader.ts";
import { clusterStub } from "./clusterid.ts";
import { deployAddons, reconcileHelm } from "./addons.ts";
import { wakePodKubelets } from "./podkubelet/wake.ts";

type Target = "scheduler" | "leases" | "workloads" | "crds" | "gc" | "accounts" | "extensions" | "containers" | "attachdetach" | "addons" | "hpa";

type BucketChange = { kind?: undefined; action: string; bucket: string; object: { key: string } };

type FollowSend = {
  queue: string;
  delaySeconds?: number;
  kind: string;
  attempt?: number;
  changed?: string[];
  names?: string[];
  node?: string;
  once?: boolean;
};

function targetOf(queueName: string): Target | null {
  if (queueName.endsWith("-scheduler")) return "scheduler";
  if (queueName.endsWith("-controllers")) return "leases";
  if (queueName.endsWith("-workloads")) return "workloads";
  if (queueName.endsWith("-crds")) return "crds";
  if (queueName.endsWith("-gc")) return "gc";
  if (queueName.endsWith("-accounts")) return "accounts";
  if (queueName.endsWith("-extensions")) return "extensions";
  if (queueName.endsWith("-containers")) return "containers";
  if (queueName.endsWith("-attachdetach")) return "attachdetach";
  if (queueName.endsWith("-addons")) return "addons";
  if (queueName.endsWith("-hpa")) return "hpa";
  return null;
}

async function followUp(env: Env, body: Record<string, unknown>): Promise<{ sends: FollowSend[]; stop: boolean }> {
  const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/queue/followup", {
    method: "POST",
    headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }));
  if (!resp.ok) return { sends: [], stop: false };
  return resp.json() as Promise<{ sends: FollowSend[]; stop: boolean }>;
}

function queueOf(env: Env, name: string): Queue | null {
  switch (name) {
    case "sched": return env.SCHED_Q;
    case "wl": return env.WL_Q;
    case "crd": return env.CRD_Q;
    case "gc": return env.GC_Q;
    case "acct": return env.ACCT_Q;
    case "ext": return env.EXT_Q;
    case "hpa": return env.HPA_Q;
    case "containers": return env.CONTAINERS_Q;
    case "addons": return env.ADDON_Q;
    case "ctrl": return env.CTRL_Q;
    default: return null;
  }
}

function targetForQueue(name: string): Target | null {
  switch (name) {
    case "sched": return "scheduler";
    case "wl": return "workloads";
    case "crd": return "crds";
    case "gc": return "gc";
    case "acct": return "accounts";
    case "ext": return "extensions";
    case "hpa": return "hpa";
    case "containers": return "containers";
    case "addons": return "addons";
    default: return null;
  }
}

async function applySends(env: Env, sends: FollowSend[]): Promise<void> {
  for (const send of sends) {
    if (send.kind === "lease-check") {
      await clusterStub(env).fetch("https://cluster.internal/lease-check", {
        method: "POST",
        body: JSON.stringify({ node: send.node ?? "", delayMs: (send.delaySeconds ?? 0) * 1000 }),
      });
      continue;
    }
    if (send.once) {
      const target = targetForQueue(send.queue);
      if (target) {
        await clusterStub(env).fetch("https://cluster.internal/enqueue", {
          method: "POST",
          body: JSON.stringify({ target, delayMs: (send.delaySeconds ?? 0) * 1000, once: true }),
        });
        continue;
      }
    }
    const queue = queueOf(env, send.queue);
    if (!queue) continue;
    const body: QueueMessage = { kind: "retry", attempt: send.attempt, changed: send.changed, names: send.names };
    await queue.send(body, send.delaySeconds ? { delaySeconds: send.delaySeconds } : undefined);
  }
}

async function consumeScheduler(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const result = await env.SCHEDULER.schedule(batch.messages.map((m) => m.body));
  const follow = await followUp(env, {
    target: "scheduler",
    ok: Boolean(result),
    skip: Boolean(result?.skip),
    attempt: result?.attempt ?? 0,
    retryAfterS: result?.retryAfterS ?? 0,
  });
  if (result && !result.skip) console.log(`scheduler: bound=${result.bound} unschedulable=${result.unschedulable.length} attempt=${result.attempt}`);
  await applySends(env, follow.sends);
  batch.ackAll();
}

async function queuePlan(env: Env, messages: readonly QueueMessage[]): Promise<{ resources: string[]; serviceKeys: string[]; gatewayKeys: string[] } | null> {
  const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/queue/plan", {
    method: "POST",
    headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
    body: JSON.stringify({ messages }),
  }));
  if (!resp.ok) return null;
  return resp.json() as Promise<{ resources: string[]; serviceKeys: string[]; gatewayKeys: string[] }>;
}

async function consumeWorkloads(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const trace = Trace.start(env, "control-plane");
  const root = trace.root("queue.consume", undefined, {
    "k8flare.queue": "k8flare-workloads",
    "k8flare.messages": batch.messages.length,
  });
  try {
    await root.measure(() => consumeWorkloadsInner(batch, env, root));
  } finally {
    await trace.flush();
  }
}

async function consumeWorkloadsInner(batch: MessageBatch<QueueMessage>, env: Env, root: import("./otel").Span): Promise<void> {
  const planSpan = root.child("queue.plan", { "k8flare.messages": batch.messages.length });
  const plan = await planSpan.measure(() => queuePlan(env, batch.messages.map((m) => m.body)));
  if (!plan) {
    await applySends(env, (await followUp(env, { target: "workloads" })).sends);
    batch.ackAll();
    return;
  }
  const changed = plan.resources;
  console.log(`workloads: consume changed=${changed.join(",") || "(none)"} msgs=${batch.messages.length}`);
  let provisionFailed = false;
  if (plan.serviceKeys.length > 0) {
    const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/loadbalancer/provision", {
      method: "POST",
      headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
      body: JSON.stringify({ keys: plan.serviceKeys }),
    }));
    if (!resp.ok) provisionFailed = true;
  }
  if (plan.gatewayKeys.length > 0) {
    const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/gateway/provision", {
      method: "POST",
      headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
      body: JSON.stringify({ keys: plan.gatewayKeys }),
    }));
    if (!resp.ok) provisionFailed = true;
  }
  let hasResult = false;
  let drained = false;
  let nextMs = 0;
  if (changed.length > 0) {
    const syncSpan = root.child("rpc.workloads.sync", { "k8flare.changed": changed.join(",") });
    let result: Awaited<ReturnType<typeof env.WORKLOADS.sync>> = null;
    try {
      result = await syncSpan.measure(() => env.WORKLOADS.sync(changed, traceparent(syncSpan)));
    } catch (err) {
      console.log(`workloads: sync failed, deferring to follow-up: ${String(err)}`);
    }
    hasResult = Boolean(result);
    drained = Boolean(result?.drained);
    nextMs = result?.nextMs ?? 0;
    if (result) console.log(`workloads: ${Object.entries(result.objects).map(([k, v]) => `${k}=${v}`).join(" ")} drained=${result.drained}`);
  }
  const followSpan = root.child("queue.followup", { "k8flare.changed": changed.join(",") });
  await followSpan.measure(async () => {
    await applySends(env, (await followUp(env, { target: "workloads", planOK: true, provisionFailed, changed, hasResult, drained, nextMs })).sends);
  });
  batch.ackAll();
}

function traceparent(span: import("./otel").Span): string {
  return `00-${span.context.traceId}-${span.context.spanId}-01`;
}

async function consumeAttachDetach(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const plan = await queuePlan(env, batch.messages.map((m) => m.body));
  const changed = plan?.resources ?? [];
  console.log(`attachdetach: consume changed=${changed.join(",") || "(none)"} msgs=${batch.messages.length}`);
  if (changed.length === 0) {
    batch.ackAll();
    return;
  }
  try {
    const attached = await env.ATTACHDETACH.sync();
    if (attached) {
      console.log(`attachdetach: ${Object.entries(attached.objects).map(([k, v]) => `${k}=${v}`).join(" ")} drained=${attached.drained}`);
    } else {
      console.log("attachdetach: sync returned null");
    }
  } catch (err) {
    console.log(`attachdetach: sync threw ${err}`);
  }
  batch.ackAll();
}

async function consumeHPA(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  console.log(`hpa: consume msgs=${batch.messages.length}`);
  try {
    await apiserverFetch(env, new Request("https://apiserver.internal/internal/metrics/scrape", {
      method: "POST",
      headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}` },
    }));
  } catch (err) {
    console.log(`hpa: metrics scrape threw ${err}`);
  }
  let hasResult = false;
  let drained = false;
  let hpas = 0;
  try {
    const synced = await env.HPA.sync();
    if (synced) {
      hasResult = true;
      drained = Boolean(synced.drained);
      hpas = synced.objects["horizontalpodautoscalers"] ?? 0;
      console.log(`hpa: ${Object.entries(synced.objects).map(([k, v]) => `${k}=${v}`).join(" ")} drained=${synced.drained}`);
    } else {
      console.log("hpa: sync returned null");
    }
  } catch (err) {
    console.log(`hpa: sync threw ${err}`);
  }
  await applySends(env, (await followUp(env, { target: "hpa", hasResult, drained, hpas })).sends);
  batch.ackAll();
}

async function consumeCRDs(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const resp = await env.CUSTOMRESOURCES.fetch("https://customresources.internal/apis");
  await resp.text();
  const pending = Number(resp.headers.get("X-CRD-Pending") ?? "1") || 0;
  console.log(`crds: status=${resp.status} pending=${pending}`);
  await applySends(env, (await followUp(env, { target: "crds", status: resp.status, pending })).sends);
  batch.ackAll();
}

async function consumeGC(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const result = await env.GC.collect();
  if (result) console.log(`gc: items=${result.items} deleted=${result.deleted} patched=${result.patched} pending=${result.pending}`);
  await applySends(env, (await followUp(env, {
    target: "gc",
    hasResult: Boolean(result),
    deleted: result?.deleted ?? 0,
    patched: result?.patched ?? 0,
    pending: result?.pending ?? 0,
  })).sends);
  batch.ackAll();
}

async function consumeAccounts(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  let namespaces: Awaited<ReturnType<typeof env.WORKLOADS.namespaces>> = null;
  try {
    namespaces = await env.WORKLOADS.namespaces(batch.messages.map((m) => m.body));
  } catch (err) {
    console.log(`namespaces: delete failed, deferring to follow-up: ${String(err)}`);
  }
  if (namespaces) console.log(`namespaces: asked=${namespaces.names?.length ?? 0} terminating=${namespaces.terminating} deleted=${namespaces.deleted} remaining=${namespaces.remaining}`);
  const first = await followUp(env, {
    target: "accounts",
    hasResult: Boolean(namespaces),
    nextMs: namespaces?.nextMs ?? 0,
    names: namespaces?.names ?? [],
    remaining: namespaces?.remaining ?? 0,
    terminating: namespaces?.terminating ?? 0,
  });
  await applySends(env, first.sends);
  const provisionOnly = first.stop;
  let result: Awaited<ReturnType<typeof env.WORKLOADS.sync>> = null;
  try {
    result = await env.WORKLOADS.sync(provisionOnly ? ["namespaces"] : ["namespaces", "serviceaccounts", "configmaps"]);
  } catch (err) {
    console.log(`accounts: sync failed, deferring to follow-up: ${String(err)}`);
  }
  if (result) console.log(`accounts: ${Object.entries(result.objects).map(([k, v]) => `${k}=${v}`).join(" ")} drained=${result.drained}`);
  await applySends(env, (await followUp(env, { target: "accounts", phase: "sync", hasResult: Boolean(result), drained: Boolean(result?.drained) })).sends);
  batch.ackAll();
}

async function consumeExtensions(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/extensions/dispatch", {
    method: "POST",
    headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}`, "Content-Type": "application/json" },
    body: JSON.stringify({ messages: batch.messages.map((m) => m.body) }),
  }));
  await applySends(env, (await followUp(env, { target: "extensions", ok: resp.ok })).sends);
  batch.ackAll();
}

async function consumeLeases(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  for (const msg of batch.messages) {
    const body = msg.body;
    if (body.kind !== "lease-check") continue;
    const health = await env.WORKLOADS.nodeHealth(body.node);
    console.log(`lease check: ${body.node} evicted=${health?.evicted} waiting=${health?.waiting}`);
    await applySends(env, (await followUp(env, { target: "leases", node: body.node, hasResult: Boolean(health), nextMs: health?.nextMs ?? 0 })).sends);
  }
  batch.ackAll();
}

export async function consume(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const target = targetOf(batch.queue);
  await dispatch(batch, env, target);
  if (target && target !== "leases") await recordPass(env, target);
}

async function recordPass(env: Env, target: Target): Promise<void> {
  try {
    await clusterStub(env).fetch("https://cluster.internal/pass", { method: "POST", body: JSON.stringify({ target }) });
  } catch (err) {
    console.log(`queues: recording ${target} pass failed: ${String(err)}`);
  }
}

async function dispatch(batch: MessageBatch<QueueMessage>, env: Env, target: Target | null): Promise<void> {
  switch (target) {
    case "scheduler":
      return consumeScheduler(batch, env);
    case "workloads":
      return consumeWorkloads(batch, env);
    case "crds":
      return consumeCRDs(batch, env);
    case "gc":
      return consumeGC(batch, env);
    case "accounts":
      return consumeAccounts(batch, env);
    case "leases":
      return consumeLeases(batch, env);
    case "extensions":
      return consumeExtensions(batch, env);
    case "containers":
      return consumeContainers(batch, env);
    case "attachdetach":
      return consumeAttachDetach(batch, env);
    case "addons":
      return consumeAddons(batch, env);
    case "hpa":
      return consumeHPA(batch, env);
  }
  batch.ackAll();
}

async function migrateStorage(env: Env): Promise<{ done: boolean; rewritten: number; pending: string[] } | null> {
  const resp = await apiserverFetch(env, new Request("https://apiserver.internal/internal/storage-migrate", {
    method: "POST",
    headers: { Authorization: `Bearer ${env.ADMIN_TOKEN}` },
  }));
  if (!resp.ok) return null;
  return resp.json() as Promise<{ done: boolean; rewritten: number; pending: string[] }>;
}

async function consumeAddons(batch: MessageBatch<QueueMessage | BucketChange>, env: Env): Promise<void> {
  const migrateOnly = batch.messages.every((m) => m.body.kind === "retry" && m.body.names?.includes("storage-migrate"));
  const addonsOK = migrateOnly || (await deployAddons(env));
  const helmOK = migrateOnly || (await reconcileHelm(env));
  const ok = addonsOK && helmOK;
  const migration = await migrateStorage(env);
  const pending = migration ? migration.pending.length : 1;
  console.log(`addons: ok=${ok} addons=${addonsOK} helm=${helmOK} storage-migrate rewritten=${migration?.rewritten ?? 0} pending=${pending} msgs=${batch.messages.length}`);
  await applySends(env, (await followUp(env, { target: "addons", ok, pending })).sends);
  batch.ackAll();
}

async function consumeContainers(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const keys = new Set<string>();
  for (const msg of batch.messages) {
    if (msg.body.kind === "change") keys.add(msg.body.key);
  }
  const { clusterName } = await import("./clusterid.ts");
  const stub = env.NODE_SCHED.get(env.NODE_SCHED.idFromName(clusterName(env)));
  const resp = await stub.fetch("https://nodesched.internal/reconcile");
  const body = resp.ok ? ((await resp.json()) as { hasWork?: boolean }) : {};
  const woken = await wakePodKubelets(env, [...keys]);
  console.log(`containers: keys=${keys.size} status=${resp.status} hasWork=${Boolean(body.hasWork)} kubelets=${woken}`);
  await applySends(env, (await followUp(env, { target: "containers", hasWork: Boolean(body.hasWork) })).sends);
  batch.ackAll();
}
