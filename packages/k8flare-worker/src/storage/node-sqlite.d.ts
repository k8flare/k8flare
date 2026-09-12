// Minimal surface of node:sqlite, declared here rather than adding "node" to
// the root tsconfig's types: the worker's globals come from
// @cloudflare/workers-types and Node's would shadow several of them. Used
// only by facetfailure.test.ts, which needs a real SQLite to exercise the
// kine schema's unique index.
declare module "node:sqlite" {
  export class DatabaseSync {
    constructor(path: string);
    exec(sql: string): void;
    prepare(sql: string): {
      run(...params: unknown[]): unknown;
      all(...params: unknown[]): unknown[];
    };
  }
}
