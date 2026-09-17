import type { QueueMessage } from "@k8flare/cluster-store";
import type { RunResult } from "./controllers.ts";

const runWindowMs = 300_000;
const leaseGraceMs = 60_000;
const leaseHoldMs = 60_000;
const refusedRetryMs = 5_000;
const unschedulableMaxDelayS = 60;
const leasePrefix = "/registry/leases/kube-node-lease/";
const crdPrefix = "/registry/apiextensions.k8s.io/customresourcedefinitions/";

type Target = "scheduler" | "controllers" | "workloads" | "crds";

function targetOf(queueName: string): Target | null {
  if (queueName.endsWith("-scheduler")) return "scheduler";
  if (queueName.endsWith("-controllers")) return "controllers";
  if (queueName.endsWith("-workloads")) return "workloads";
  if (queueName.endsWith("-crds")) return "crds";
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

async function followUp(env: Env, result: RunResult): Promise<void> {
  let delayMs = 0;
  if (result.status === 204) delayMs = result.retryAfterMs > 0 ? result.retryAfterMs : refusedRetryMs;
  else if (result.status === 200 && result.nextMs > 0) delayMs = result.nextMs;
  else if (result.status !== 200) delayMs = refusedRetryMs;
  if (delayMs <= 0) return;
  await env.CTRL_Q.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: Math.ceil(delayMs / 1000) });
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

async function consumeWorkloads(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
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
  const result = await env.WORKLOADS.syncCRDs();
  if (result) console.log(`crds: crds=${result.crds} drained=${result.drained}`);
  if (!result || !result.drained) await env.CRD_Q.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: refusedRetryMs / 1000 });
  batch.ackAll();
}

export async function consume(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const target = targetOf(batch.queue);
  if (!target) {
    batch.ackAll();
    return;
  }
  if (target === "scheduler") return consumeScheduler(batch, env);
  if (target === "workloads") return consumeWorkloads(batch, env);
  if (target === "crds") return consumeCRDs(batch, env);
  let needRun = false;
  let minMs = 0;
  let reset = false;
  let crdChanged = false;
  for (const msg of batch.messages) {
    const body = msg.body;
    if (body.kind === "lease-check") {
      if (await leaseExpired(env, body.node)) {
        console.log(`lease expired: ${body.node}`);
        needRun = true;
        minMs = Math.max(minMs, leaseHoldMs);
      }
      continue;
    }
    needRun = true;
    if (body.kind === "change") {
      reset = true;
      if (body.key.startsWith(crdPrefix)) crdChanged = true;
    }
  }
  if (crdChanged && target === "controllers") {
    await env.CUSTOMRESOURCES.fetch("https://customresources.internal/apis").then((r) => r.text()).catch(() => {});
  }
  if (!needRun) {
    batch.ackAll();
    return;
  }
  const result = (await env.CONTROLLERS.run(runWindowMs, minMs, reset)) as RunResult;
  await followUp(env, result);
  batch.ackAll();
}
