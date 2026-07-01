export const SCHEMA = [
  `CREATE TABLE IF NOT EXISTS kine (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT,
    created INTEGER,
    deleted INTEGER,
    create_revision INTEGER,
    prev_revision INTEGER,
    lease INTEGER,
    value BLOB,
    old_value BLOB
  )`,
  `CREATE INDEX IF NOT EXISTS kine_name_index ON kine (name)`,
  `CREATE INDEX IF NOT EXISTS kine_name_id_index ON kine (name,id)`,
  `CREATE INDEX IF NOT EXISTS kine_id_deleted_index ON kine (id,deleted)`,
  `CREATE INDEX IF NOT EXISTS kine_prev_revision_index ON kine (prev_revision)`,
  `CREATE UNIQUE INDEX IF NOT EXISTS kine_name_prev_revision_uindex ON kine (name, prev_revision)`,
];

// GET_SQL is used for exact-key lookups (getCurrent). Using = instead of LIKE
// avoids the "LIKE or GLOB pattern too complex" error that D1 raises when keys
// contain many special characters (e.g. k8s event names with dots and hashes).
export const GET_SQL = (includeDeleted: boolean): string => `
  SELECT
    (SELECT MAX(rkv.id) FROM kine AS rkv) AS current_rev,
    (SELECT MAX(crkv.prev_revision) FROM kine AS crkv WHERE crkv.name = 'compact_rev_key') AS compact_rev,
    kv.id AS theid, kv.name AS thename, kv.created, kv.deleted,
    kv.create_revision, kv.prev_revision, kv.lease, kv.value, kv.old_value
  FROM kine AS kv
  WHERE kv.name = ?1 AND (kv.deleted = 0 OR ${includeDeleted ? 1 : 0})
  ORDER BY kv.id DESC
  LIMIT 1`;

// LIST_SQL uses range queries (>= and <) instead of LIKE to avoid D1's
// "LIKE or GLOB pattern too complex" error. ?1 = prefix, ?2 = prefixEnd
// (prefix with last char incremented). ?3 = includeDeleted (0 or 1).
export const LIST_SQL = (extraCondition: string): string => `
  SELECT *
  FROM (
    SELECT
      (SELECT MAX(rkv.id) FROM kine AS rkv) AS current_rev,
      (SELECT MAX(crkv.prev_revision) FROM kine AS crkv WHERE crkv.name = 'compact_rev_key') AS compact_rev,
      kv.id AS theid, kv.name AS thename, kv.created, kv.deleted,
      kv.create_revision, kv.prev_revision, kv.lease, kv.value, kv.old_value
    FROM kine AS kv
    JOIN (
      SELECT MAX(mkv.id) AS id
      FROM kine AS mkv
      WHERE mkv.name >= ?1 AND mkv.name < ?2 ${extraCondition}
      GROUP BY mkv.name
    ) AS maxkv ON maxkv.id = kv.id
    WHERE kv.deleted = 0 OR ?3
  ) AS lkv
  ORDER BY lkv.thename ASC`;

export const INSERT_SQL = `INSERT INTO kine(name, created, deleted, create_revision, prev_revision, lease, value, old_value)
  VALUES(?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`;

// AFTER_SQL retrieves all events after a given revision. No LIKE needed —
// prefix filtering is done in JS (broadcastEvent, handleWebSocket) to avoid
// D1's "LIKE or GLOB pattern too complex" error.
export const AFTER_SQL = `
  SELECT
    (SELECT MAX(rkv.id) FROM kine AS rkv) AS current_rev,
    (SELECT MAX(crkv.prev_revision) FROM kine AS crkv WHERE crkv.name = 'compact_rev_key') AS compact_rev,
    kv.id AS theid, kv.name AS thename, kv.created, kv.deleted,
    kv.create_revision, kv.prev_revision, kv.lease, kv.value, kv.old_value
  FROM kine AS kv
  WHERE kv.id > ?1
  ORDER BY kv.id ASC`;
