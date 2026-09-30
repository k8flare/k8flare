const HOUR_MS = 3_600_000;
const DEFAULT_SNAPSHOT_INTERVAL_HOURS = 12;
const DEFAULT_SNAPSHOT_RETENTION = 5;

export function nextAlarmAt(deadlines: (number | null)[], now: number, minDelayMs = 1000): number | null {
  const due = deadlines.filter((d): d is number => d !== null);
  if (due.length === 0) return null;
  return Math.max(Math.min(...due), now + minDelayMs);
}

export function compactionTarget(input: { revision: number; timeTarget: number; maxRetained: number }): number {
  return Math.max(input.timeTarget, input.revision - input.maxRetained);
}

export interface SnapshotSchedule {
  intervalMs: number;
  retention: number;
}

export interface SnapshotVars {
  SNAPSHOT_INTERVAL_HOURS?: string;
  SNAPSHOT_RETENTION?: string;
}

export function snapshotSchedule(vars: SnapshotVars): SnapshotSchedule | null {
  const hours = vars.SNAPSHOT_INTERVAL_HOURS === undefined ? DEFAULT_SNAPSHOT_INTERVAL_HOURS : Number(vars.SNAPSHOT_INTERVAL_HOURS);
  if (hours === 0) return null;
  const retention = Number(vars.SNAPSHOT_RETENTION ?? DEFAULT_SNAPSHOT_RETENTION);
  return {
    intervalMs: (Number.isFinite(hours) && hours > 0 ? hours : DEFAULT_SNAPSHOT_INTERVAL_HOURS) * HOUR_MS,
    retention: Number.isInteger(retention) && retention > 0 ? retention : DEFAULT_SNAPSHOT_RETENTION,
  };
}

export function snapshotsToPrune(keys: string[], scheduledPrefix: string, retention: number): string[] {
  const scheduled = keys.filter((k) => k.startsWith(scheduledPrefix)).sort();
  return scheduled.slice(0, Math.max(0, scheduled.length - retention));
}
