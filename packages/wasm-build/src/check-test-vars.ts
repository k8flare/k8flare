import fs from "node:fs";
import path from "node:path";

const CONFIG = "packages/k8flare-worker/wrangler.jsonc";

const HARNESS_ONLY = ["PUMP_WINDOW_DROP_CLOSE", "KCM_DISABLED", "SCHED_DISABLED", "CM_DISABLED"];

const source = fs.readFileSync(path.join(process.cwd(), CONFIG), "utf8");
const declared = HARNESS_ONLY.filter((name) => new RegExp(`"${name}"\\s*:`).test(source));

if (declared.length === 0) {
  console.log(`check-test-vars: none of ${HARNESS_ONLY.length} harness-only vars are declared`);
  process.exit(0);
}

console.error(
  [
    `check-test-vars: ${CONFIG} declares ${declared.length} harness-only var(s):`,
    ...declared.map((n) => `  ${n}`),
    "",
    "Every one of these is passed per invocation by a test lane or by",
    "e2e-conformance.yml (`wrangler dev --var ...`), and none is meaningful in a",
    "deployment. PUMP_WINDOW_DROP_CLOSE wedges pump windows on purpose; the",
    "*_DISABLED switches turn off a resident controller so a host process can",
    "take its place. A deployment that declares one runs a crippled control",
    "plane and says nothing about it. Remove them from the config.",
  ].join("\n"),
);
process.exit(1);
