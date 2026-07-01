import { GET_SQL, INSERT_SQL } from "./schema.ts";
import type { KineRow, KineEvent } from "./helpers.ts";
import { rowToEvent } from "./helpers.ts";

export interface SqlExec {
  exec(query: string, ...params: unknown[]): SqlCursor;
}

export interface SqlCursor {
  one(): Record<string, unknown>;
  toArray(): KineRow[];
}

export function currentRevision(sql: SqlExec): number {
  const row = sql.exec("SELECT MAX(id) AS rev FROM kine").one();
  return (row.rev as number) || 0;
}

export function getCurrent(
  sql: SqlExec,
  key: string,
  includeDeleted: boolean = false,
): { rev: number; event: KineEvent | null } {
  const q = GET_SQL(includeDeleted);
  const rows = sql.exec(q, key).toArray();
  if (rows.length === 0) return { rev: currentRevision(sql), event: null };
  return { rev: rows[0].current_rev, event: rowToEvent(rows[0]) };
}

export function insert(
  sql: SqlExec,
  key: string,
  create: boolean,
  del: boolean,
  createRevision: number,
  prevRevision: number,
  lease: number,
  value: ArrayBuffer | null,
  oldValue: ArrayBuffer | string | null,
): number {
  sql.exec(
    INSERT_SQL,
    key,
    create ? 1 : 0,
    del ? 1 : 0,
    createRevision,
    prevRevision,
    lease,
    value,
    oldValue,
  );
  return sql.exec("SELECT last_insert_rowid() AS id").one().id as number;
}
