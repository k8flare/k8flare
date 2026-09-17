import type { QueueMessage } from "@k8flare/cluster-store";
import type { RunResult } from "./scheduler.ts";

const runWindowMs = 300_000;
const leaseGraceMs = 60_000;
const leaseHoldMs = 60_000;
const refusedRetryMs = 5_000;
const leasePrefix = "/registry/leases/kube-node-lease/";

type Target = "scheduler" | "controllers";

function targetOf(queueName: string): Target | null {
  if (queueName.endsWith("-scheduler")) return "scheduler";
  if (queueName.endsWith("-controllers")) return "controllers";
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

async function followUp(env: Env, target: Target, result: RunResult): Promise<void> {
  let delayMs = 0;
  if (result.status === 204) delayMs = result.retryAfterMs > 0 ? result.retryAfterMs : refusedRetryMs;
  else if (result.status === 200 && result.nextMs > 0) delayMs = result.nextMs;
  else if (result.status !== 200) delayMs = refusedRetryMs;
  if (delayMs <= 0) return;
  const queue = target === "scheduler" ? env.SCHED_Q : env.CTRL_Q;
  await queue.send({ kind: "retry" } satisfies QueueMessage, { delaySeconds: Math.ceil(delayMs / 1000) });
}

export async function consume(batch: MessageBatch<QueueMessage>, env: Env): Promise<void> {
  const target = targetOf(batch.queue);
  if (!target) {
    batch.ackAll();
    return;
  }
  let needRun = false;
  let minMs = 0;
  let reset = false;
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
    if (body.kind === "change") reset = true;
  }
  if (!needRun) {
    batch.ackAll();
    return;
  }
  const entrypoint = target === "scheduler" ? env.SCHEDULER : env.CONTROLLERS;
  const result = (await entrypoint.run(runWindowMs, minMs, reset)) as RunResult;
  await followUp(env, target, result);
  batch.ackAll();
}
