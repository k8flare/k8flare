import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const CONFIG = "packages/k8flare-worker/wrangler.jsonc";
const RECORD = "packages/k8flare-worker/migrations.sha256";
const OVERRIDE = "K8FLARE_ALLOW_MIGRATION_CHANGE";

function migrationsBlock(source: string): string {
  const start = source.indexOf('"migrations"');
  if (start === -1) throw new Error(`${CONFIG}: no "migrations" key`);
  const open = source.indexOf("[", start);
  if (open === -1) throw new Error(`${CONFIG}: "migrations" is not an array`);
  let depth = 0;
  for (let i = open; i < source.length; i++) {
    if (source[i] === "[") depth++;
    else if (source[i] === "]" && --depth === 0) return source.slice(start, i + 1);
  }
  throw new Error(`${CONFIG}: unterminated "migrations" array`);
}

const root = process.cwd();
const block = migrationsBlock(fs.readFileSync(path.join(root, CONFIG), "utf8"));
const actual = createHash("sha256").update(block).digest("hex");
const recordPath = path.join(root, RECORD);
const destructive = /"deleted_classes"/.test(block);

if (process.argv.includes("--record")) {
  fs.writeFileSync(recordPath, actual + "\n");
  console.log(`check-migrations: recorded ${actual.slice(0, 16)}…`);
  process.exit(0);
}

if (!fs.existsSync(recordPath)) {
  console.error(
    `check-migrations: ${RECORD} is missing. Run \`npm run check:migrations -- --record\` and commit it.`,
  );
  process.exit(1);
}

const expected = fs.readFileSync(recordPath, "utf8").trim();
if (actual === expected) {
  console.log(
    `check-migrations: unchanged (${actual.slice(0, 16)}…${destructive ? ", contains deleted_classes already applied" : ""})`,
  );
  process.exit(0);
}

if (process.env[OVERRIDE] === "1") {
  console.log(`check-migrations: change accepted via ${OVERRIDE}=1 (${actual.slice(0, 16)}…)`);
  process.exit(0);
}

console.error(
  [
    "check-migrations: the Durable Object migrations block changed.",
    "",
    `  recorded: ${expected}`,
    `  actual:   ${actual}`,
    "",
    destructive
      ? "It contains a deleted_classes entry. Applying an unapplied destructive tag"
      : "It has no deleted_classes entry, but the block still changed, and",
    "destroys that Durable Object's storage -- every cluster's state -- and",
    "`wrangler deploy` applies any tag this deployment has not applied yet,",
    "without prompting. There is no backup mechanism.",
    "",
    "If you intend this, make it a deliberate, separate act:",
    "  npm run check:migrations -- --record   # then commit migrations.sha256",
    `or set ${OVERRIDE}=1 for a single deploy you have reviewed.`,
  ].join("\n"),
);
process.exit(1);
