export type SeatRequest = {
  id?: string;
  level: string;
  limit: number;
  queues: number;
  queueLengthLimit: number;
  hand: number[];
  waitMs: number;
  user?: string;
};

export type SeatOutcome = { admitted: true; release: () => void } | { admitted: false; retryAfter: number };

export const SEAT_HOLD_MS = 60_000;
const BUCKET_SWEEP_SIZE = 1024;

type Waiter = { admit: () => void };

class Level {
  limit = 0;
  executing = 0;
  waiting = new Map<number, Waiter[]>();
  executingIn = new Map<number, number>();
  cursor = 0;

  queued(queue: number): number {
    return this.waiting.get(queue)?.length ?? 0;
  }

  load(queue: number): number {
    return this.queued(queue) + (this.executingIn.get(queue) ?? 0);
  }

  anyWaiting(): boolean {
    for (const q of this.waiting.values()) if (q.length > 0) return true;
    return false;
  }
}

type Bucket = { tokens: number; at: number };

export class Seats {
  private levels = new Map<string, Level>();
  private buckets = new Map<string, Bucket>();
  private held = new Map<string, () => void>();
  private qps: number;
  private burst: number;
  private now: () => number;
  private setTimer: (fn: () => void, ms: number) => unknown;
  private clearTimer: (t: unknown) => void;

  constructor(
    qps: number,
    burst: number,
    now: () => number = Date.now,
    setTimer: (fn: () => void, ms: number) => unknown = setTimeout,
    clearTimer: (t: unknown) => void = (t) => clearTimeout(t as ReturnType<typeof setTimeout>),
  ) {
    this.qps = qps;
    this.burst = burst;
    this.now = now;
    this.setTimer = setTimer;
    this.clearTimer = clearTimer;
  }

  acquire(req: SeatRequest): Promise<SeatOutcome> {
    if (req.user) {
      const retryAfter = this.takeToken(req.user);
      if (retryAfter > 0) return Promise.resolve({ admitted: false, retryAfter });
    }
    let level = this.levels.get(req.level);
    if (!level) {
      level = new Level();
      this.levels.set(req.level, level);
    }
    level.limit = req.limit;
    this.dispatch(level);
    if (req.queues === 0 || req.hand.length === 0) {
      if (level.executing < level.limit) return Promise.resolve(this.admit(level, 0, req.id));
      return Promise.resolve({ admitted: false, retryAfter: 1 });
    }
    const queue = req.hand.reduce((best, q) => (level.load(q) < level.load(best) ? q : best), req.hand[0]);
    if (!level.anyWaiting() && level.executing < level.limit) return Promise.resolve(this.admit(level, queue, req.id));
    if (level.queued(queue) >= req.queueLengthLimit) return Promise.resolve({ admitted: false, retryAfter: 1 });
    return new Promise((resolve) => {
      const waiter: Waiter = {
        admit: () => {
          this.clearTimer(timer);
          resolve(this.admit(level, queue, req.id));
        },
      };
      const leave = () => {
        this.clearTimer(timer);
        if (req.id) this.held.delete(req.id);
        const q = level.waiting.get(queue) ?? [];
        const i = q.indexOf(waiter);
        if (i >= 0) q.splice(i, 1);
        resolve({ admitted: false, retryAfter: 1 });
      };
      const timer = this.setTimer(leave, req.waitMs);
      if (req.id) this.held.set(req.id, leave);
      const q = level.waiting.get(queue) ?? [];
      q.push(waiter);
      level.waiting.set(queue, q);
    });
  }

  release(id: string): void {
    this.held.get(id)?.();
  }

  private admit(level: Level, queue: number, id?: string): SeatOutcome {
    level.executing++;
    level.executingIn.set(queue, (level.executingIn.get(queue) ?? 0) + 1);
    let released = false;
    const release = () => {
      if (released) return;
      released = true;
      if (id) this.held.delete(id);
      this.clearTimer(hold);
      level.executing--;
      level.executingIn.set(queue, (level.executingIn.get(queue) ?? 1) - 1);
      this.dispatch(level);
    };
    const hold = this.setTimer(release, SEAT_HOLD_MS);
    if (id) this.held.set(id, release);
    return { admitted: true, release };
  }

  private dispatch(level: Level): void {
    while (level.executing < level.limit && level.anyWaiting()) {
      const queues = [...level.waiting.keys()].sort((a, b) => a - b);
      const next = queues.find((q) => q >= level.cursor && level.queued(q) > 0) ?? queues.find((q) => level.queued(q) > 0)!;
      level.cursor = next + 1;
      level.waiting.get(next)!.shift()!.admit();
    }
  }

  private takeToken(user: string): number {
    if (this.qps <= 0) return 0;
    const now = this.now();
    if (this.buckets.size > BUCKET_SWEEP_SIZE) this.sweep(now);
    const bucket = this.buckets.get(user) ?? { tokens: this.burst, at: now };
    bucket.tokens = Math.min(this.burst, bucket.tokens + ((now - bucket.at) / 1000) * this.qps);
    bucket.at = now;
    this.buckets.set(user, bucket);
    if (bucket.tokens >= 1) {
      bucket.tokens -= 1;
      return 0;
    }
    return Math.max(1, Math.ceil((1 - bucket.tokens) / this.qps));
  }

  private sweep(now: number): void {
    for (const [user, bucket] of this.buckets) {
      if (bucket.tokens + ((now - bucket.at) / 1000) * this.qps >= this.burst) this.buckets.delete(user);
    }
  }
}
