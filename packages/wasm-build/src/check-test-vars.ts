import fs from "node:fs";
import path from "node:path";

const CONFIG = "packages/k8flare-worker/wrangler.jsonc";

const TEST_ONLY = ["PUMP_WINDOW_DROP_CLOSE", "KCM_DISABLED"];

const source = fs.readFileSync(path.join(process.cwd(), CONFIG), "utf8");
const declared = TEST_ONLY.filter((name) => new RegExp(`"${name}"\\s*:`).test(source));

if (declared.length === 0) {
  console.log(`check-test-vars: none of ${TEST_ONLY.length} test-only vars are declared`);
  process.exit(0);
}

console.error(
  [
    `check-test-vars: ${CONFIG} declares ${declared.length} test-only var(s):`,
    ...declared.map((n) => `  ${n}`),
    "",
    "These are fault-injection seams. They are set per invocation by the test",
    "harnesses (`wrangler dev --var ...`), never by a deployment: each one makes",
    "the control plane misbehave on purpose. Remove them from the config.",
  ].join("\n"),
);
process.exit(1);
