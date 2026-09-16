export type WakeTarget = "scheduler" | "controllers";

export const pokeDeadlineMs = 25_000;
export const retryMs = 15_000;

export async function scheduleWake(env: Env, target: WakeTarget, delayMs: number): Promise<void> {
  const store = env.CLUSTER.get(env.CLUSTER.idFromName("default"));
  const resp = await store.fetch("https://cluster.internal/wake", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ target, delayMs }),
  });
  if (!resp.ok) console.error(`wake ${target}: HTTP ${resp.status}`);
}
