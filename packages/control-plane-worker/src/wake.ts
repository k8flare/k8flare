import { DurableObject } from "cloudflare:workers";

export type WakeTarget = "scheduler" | "controllers";

const targets: WakeTarget[] = ["scheduler", "controllers"];
const alarmWindowMs = 60_000;
const dueSlackMs = 1_000;

export class Wake extends DurableObject<Env> {
  async schedule(target: WakeTarget, delayMs: number): Promise<void> {
    const at = Date.now() + delayMs;
    const current = await this.ctx.storage.get<number>(target);
    if (current !== undefined && current <= at) return;
    await this.ctx.storage.put(target, at);
    await this.rearm();
  }

  async alarm(): Promise<void> {
    const now = Date.now();
    const pending = await this.ctx.storage.get<number>(targets);
    const due = targets.filter((target) => (pending.get(target) ?? Infinity) <= now + dueSlackMs);
    await this.ctx.storage.delete(due);
    await this.rearm();
    await Promise.all(due.map((target) => this.run(target)));
  }

  private async rearm(): Promise<void> {
    const pending = await this.ctx.storage.get<number>(targets);
    const next = Math.min(...pending.values());
    if (Number.isFinite(next)) await this.ctx.storage.setAlarm(next);
  }

  private async run(target: WakeTarget): Promise<void> {
    const entrypoint = target === "scheduler" ? this.env.SCHEDULER : this.env.CONTROLLERS;
    await entrypoint.run(alarmWindowMs).catch((err) => console.error(`wake ${target}:`, err));
  }
}

export async function scheduleWake(env: Env, target: WakeTarget, delayMs: number): Promise<void> {
  await env.WAKE.get(env.WAKE.idFromName("default")).schedule(target, delayMs);
}
