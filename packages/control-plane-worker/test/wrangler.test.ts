import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { test } from "node:test";

const root = resolve(import.meta.dirname, "../../..");
const satellites = ["wrangler.attachdetach.jsonc", "wrangler.edge.jsonc", "wrangler.hpa.jsonc"];

test("every deployed Worker samples traces at one and the audit Worker enables Logpush", async () => {
  for (const file of satellites) {
    const config = await readFile(resolve(root, file), "utf8");
    assert.match(config, /"traces": \{ "enabled": true, "head_sampling_rate": 1, "persist": true \}/);
  }
  const main = await readFile(resolve(root, "cloudflare.config.ts"), "utf8");
  assert.match(main, /traces: \{\s*enabled: true,\s*headSamplingRate: 1,\s*persist: true,\s*\}/);
  assert.match(main, /logpush: true/);
});
