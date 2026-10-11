import assert from "node:assert/strict";
import { test } from "node:test";
import { Hooks } from "../src/hooks.ts";

function rig(bindings: string[]) {
  const calls: { worker: string; url: string; method: string; body: string }[] = [];
  const fetcher = (worker: string) => ({
    async fetch(request: Request) {
      calls.push({ worker, url: request.url, method: request.method, body: await request.text() });
      return new Response(worker);
    },
  });
  const cluster = {
    idFromName: (name: string) => name,
    get: () => ({ fetch: async () => Response.json({ kv: { value: btoa("stored-component-key") } }) }),
  };
  const env: Record<string, unknown> = { ADMIN_TOKEN: "admin", CLUSTER: cluster, LOADER: { load: fetcher } };
  for (const name of bindings) env[name] = fetcher(name);
  return { hooks: new Hooks({} as never, env as never), calls };
}

test("a hook name with a WORKER_<name> service binding reaches that Worker at the path after the name", async () => {
  const { hooks, calls } = rig(["WORKER_my-controller"]);
  const resp = await hooks.fetch(new Request("https://hooks.internal/hook/my-controller/apis/example.com/v1?watch=1", { method: "POST", body: "{}" }));
  assert.equal(await resp.text(), "WORKER_my-controller");
  assert.deepEqual(calls, [{ worker: "WORKER_my-controller", url: "https://hooks.internal/apis/example.com/v1?watch=1", method: "POST", body: "{}" }]);
});

test("a webhook or controller call to a bound Worker arrives at its root path", async () => {
  const { hooks, calls } = rig(["WORKER_my-webhook"]);
  await hooks.fetch(new Request("https://hooks.internal/hook/my-webhook?timeout=10s", { method: "POST", body: "{}" }));
  assert.equal(calls[0].url, "https://hooks.internal/?timeout=10s");
});

test("a hook name without a binding still goes to hookecho with the request unchanged", async () => {
  const { hooks, calls } = rig(["WORKER_other"]);
  const resp = await hooks.fetch(new Request("https://hooks.internal/hook/admission-echo?timeout=10s", { method: "POST", body: "{}" }));
  assert.equal(await resp.text(), "hookecho");
  assert.deepEqual(calls, [{ worker: "hookecho", url: "https://hooks.internal/hook/admission-echo?timeout=10s", method: "POST", body: "{}" }]);
});
