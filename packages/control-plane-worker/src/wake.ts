export type WakeTarget = "scheduler" | "controllers";

export const pokeDeadlineMs = 25_000;
export const retryMs = 15_000;

export const absorbedRetryMs = 5_000;

export async function scheduleWake(env: Env, target: WakeTarget, delayMs: number, holdMs = 0): Promise<void> {
  const store = env.CLUSTER.get(env.CLUSTER.idFromName("default"));
  const resp = await store.fetch("https://cluster.internal/wake", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ target, delayMs, holdMs }),
  });
  if (!resp.ok) console.error(`wake ${target}: HTTP ${resp.status}`);
}
