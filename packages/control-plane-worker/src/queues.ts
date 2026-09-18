import type { QueueMessage } from "@k8flare/cluster-store";

const leaseGraceMs = 60_000;
const refusedRetryMs = 5_000;
const unschedulableMaxDelayS = 60;
const leasePrefix = "/registry/leases/kube-node-lease/";

type Target = "scheduler" | "leases" | "workloads" | "crds" | "gc";

function targetOf(queueName: string): Target | null {
  if (queueName.endsWith("-scheduler")) return "scheduler";
  if (queueName.endsWith("-controllers")) return "leases";
  if (queueName.endsWith("-workloads")) return "workloads";
  if (queueName.endsWith("-crds")) return "crds";
  if (queueName.endsWith("-gc")) return "gc";
  return null;
}

async function leaseExpired(env: Env, node: string): Promise<boolean> {
  const store = env.CLUSTER.get(env.CLUSTER.idFromName("default"));
  const resp = await store.fetch(`https://cluster.internal/kv?key=${encodeURIComponent(leasePrefix + node)}`);
  if (!resp.ok) return false;
  const data = (await resp.json()) as { kv: { value: string } | null };
  if (!data.kv) return false;
  const lease = JSON.parse(atob(data.kv.value)) as { spec?: { renewTime?: string } };
  const renewed = Date.parse(lease.spec?.renewTime ?? "");
  return !Number.isFinite(renewed) || Date.now() - renewed >= leaseGraceMs;
}

async function consumeScheduler(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  let attempt = -1;
  for (const msg of batch.messages) {
    const body = msg.body;
    if (body.kind === "change") attempt = Math.max(attempt, 0);
    else if (body.kind === "retry") attempt = Math.max(attempt, (body.attempt ?? 0) + 1);
  }
  if (attempt < 0) {
    batch.ackAll();
    return;
  }
  if (batch.messages.some((m) => m.body.kind === "change")) attempt = 0;
  const result = await env.SCHEDULER.schedule();
  if (!result) {
    await env.SCHED_Q.send({ kind: "retry", attempt: 0 } satisfies QueueMessage, { delaySeconds: refusedRetryMs / 1000 });
  } else {
    console.log(`scheduler: bound=${result.bound} unschedulable=${result.unschedulable.length} attempt=${attempt}`);
    if (result.unschedulable.length > 0) {
      const delaySeconds = Math.min(unschedulableMaxDelayS, 2 ** attempt);
      await env.SCHED_Q.send({ kind: "retry", attempt } satisfies QueueMessage, { delaySeconds });
    }
  }
  batch.ackAll();
}

const namespacePrefix = "/registry/namespaces/";

async function consumeWorkloads(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const terminating = new Set<string>();
  for (const msg of batch.messages) {
    if (msg.body.kind === "change" && msg.body.key.startsWith(namespacePrefix)) terminating.add(msg.body.key.slice(namespacePrefix.length));
  }
  if (terminating.size > 0) {
    const names = [...terminating];
    const namespaces = await env.WORKLOADS.namespaces(names);
    if (namespaces) console.log(`namespaces: asked=${names.length} terminating=${namespaces.terminating} deleted=${namespaces.deleted} remaining=${namespaces.remaining}`);
    const delayMs = namespaces ? namespaces.nextMs : refusedRetryMs;
    if (delayMs > 0) {
      await env.WL_Q.sendBatch(
        names.map((name) => ({ body: { kind: "change", key: namespacePrefix + name, type: "modified", rev: 0 } satisfies QueueMessage, delaySeconds: Math.ceil(delayMs / 1000) })),
      );
    }
  }
  const result = await env.WORKLOADS.sync();
  if (!result) {
    await env.WL_Q.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: refusedRetryMs / 1000 });
  } else {
    console.log(`workloads: ${Object.entries(result.objects).map(([k, v]) => `${k}=${v}`).join(" ")} drained=${result.drained}`);
    const delayMs = result.drained ? result.nextMs : refusedRetryMs;
    if (delayMs > 0) await env.WL_Q.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: Math.ceil(delayMs / 1000) });
  }
  batch.ackAll();
}

async function consumeCRDs(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const resp = await env.CUSTOMRESOURCES.fetch("https://customresources.internal/apis");
  await resp.text();
  const pending = Number(resp.headers.get("X-CRD-Pending") ?? "1") || 0;
  console.log(`crds: status=${resp.status} pending=${pending}`);
  if (resp.status !== 200 || pending > 0) await env.CRD_Q.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: refusedRetryMs / 1000 });
  batch.ackAll();
}

const gcSettleMs = 2_000;

async function consumeGC(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const result = await env.GC.collect();
  if (result) console.log(`gc: items=${result.items} deleted=${result.deleted} patched=${result.patched} pending=${result.pending}`);
  const changed = !result || result.deleted > 0 || result.patched > 0 || result.pending > 0;
  if (changed) {
    await env.GC_Q.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: Math.ceil((result ? gcSettleMs : refusedRetryMs) / 1000) });
  }
  batch.ackAll();
}

async function consumeLeases(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  for (const msg of batch.messages) {
    const body = msg.body;
    if (body.kind !== "lease-check" || !(await leaseExpired(env, body.node))) continue;
    const health = await env.WORKLOADS.nodeHealth(body.node);
    console.log(`lease expired: ${body.node} evicted=${health?.evicted} waiting=${health?.waiting}`);
    const delayMs = health ? health.nextMs : refusedRetryMs;
    if (delayMs > 0) {
      await env.CTRL_Q.send({ kind: "lease-check", node: body.node } satisfies QueueMessage, { delaySeconds: Math.ceil(delayMs / 1000) });
    }
  }
  batch.ackAll();
}

export async function consume(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  switch (targetOf(batch.queue)) {
    case "scheduler":
      return consumeScheduler(batch, env);
    case "workloads":
      return consumeWorkloads(batch, env);
    case "crds":
      return consumeCRDs(batch, env);
    case "gc":
      return consumeGC(batch, env);
    case "leases":
      return consumeLeases(batch, env);
  }
  batch.ackAll();
}
