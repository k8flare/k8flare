export type WakeTarget = "scheduler" | "controllers";

export const insuranceMs = 25_000;
export const absorbedRetryMs = 5_000;

function store(env: Env): DurableObjectStub {
  return env.CLUSTER.get(env.CLUSTER.idFromName("default"));
}

async function post(env: Env, path: string, body: unknown): Promise<void> {
  const resp = await store(env).fetch(`https://cluster.internal${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!resp.ok) console.error(`${path}: HTTP ${resp.status}`);
}

export async function scheduleWake(env: Env, target: WakeTarget, delayMs: number, holdMs = 0, insured = false): Promise<void> {
  await post(env, "/wake", { target, delayMs, holdMs, insured });
}

export async function settleWake(env: Env, target: WakeTarget): Promise<void> {
  await post(env, "/wake/settle", { target });
}
