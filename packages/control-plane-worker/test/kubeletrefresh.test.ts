import assert from "node:assert/strict";
import { mock, test } from "node:test";
import { NodeTunnel } from "../../node-tunnel/src/index.ts";

const minute = 60 * 1000;
const hour = 60 * minute;
const kubeletClientLifetime = 24 * hour;

let clusters = 0;

function rig() {
  const state = { down: false, kubeletRequests: 0 };
  const env = {
    CLUSTER_UID: `kubelet-refresh-${++clusters}`,
    CLUSTER: {
      idFromName: (name: string) => name,
      get: () => ({ fetch: async () => Response.json({ revision: 1, kv: { key: "k", value: btoa("component-key"), modRevision: 1 } }) }),
    },
    __apiserver: async (request: Request) => {
      if (state.down) return new Response("unavailable", { status: 503 });
      if (new URL(request.url).pathname === "/internal/kubelet-client") {
        state.kubeletRequests++;
        return Response.json({ cert: `leaf-issued-at-${Date.now()}`, key: "k", ca: "ca" });
      }
      return Response.json({ cert: "pc", key: "pk" });
    },
  };
  const ctx = { getWebSockets: () => [], blockConcurrencyWhile: (fn: () => Promise<void>) => fn(), storage: { get: async () => undefined } };
  const tunnel = new NodeTunnel(ctx as unknown as DurableObjectState, env as unknown as Env);
  const serve = () => tunnel.fetch(new Request("https://nodetunnel.internal/node/n1/stats/summary"));
  const leaf = () => (globalThis as unknown as { __goEnv: Record<string, string> }).__goEnv.KUBELET_CLIENT_CERT;
  return { state, serve, leaf };
}

function issuedAt(leaf: string): number {
  return Number(leaf.replace("leaf-issued-at-", ""));
}

test("the node tunnel reissues the apiserver-to-kubelet client leaf long before it expires and runs on the new one", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: 1_000_000_000_000 });
  const { state, serve, leaf } = rig();
  await serve();
  const first = leaf();
  assert.equal(state.kubeletRequests, 1);

  t.mock.timers.tick(59 * minute);
  await serve();
  assert.equal(state.kubeletRequests, 1);
  assert.equal(leaf(), first);

  t.mock.timers.tick(2 * minute);
  await serve();
  assert.equal(state.kubeletRequests, 2);
  assert.notEqual(leaf(), first);
  assert.equal(issuedAt(leaf()), Date.now());

  const startedAt = Date.now();
  let oldest = 0;
  while (Date.now() - startedAt < 2 * kubeletClientLifetime) {
    t.mock.timers.tick(10 * minute);
    await serve();
    oldest = Math.max(oldest, Date.now() - issuedAt(leaf()));
  }
  assert.ok(oldest < kubeletClientLifetime / 12, `the leaf in use was ${oldest / minute} minutes old, a 24 hour leaf must be replaced long before it expires`);
});

test("a failed refresh keeps the leaf in use and the node tunnel retries after a minute", async (t) => {
  t.mock.timers.enable({ apis: ["Date"], now: 2_000_000_000_000 });
  const { state, serve, leaf } = rig();
  await serve();
  const first = leaf();

  state.down = true;
  t.mock.timers.tick(61 * minute);
  const resp = await serve();
  assert.equal(resp.status, 200);
  assert.equal(leaf(), first);

  state.down = false;
  t.mock.timers.tick(30 * 1000);
  await serve();
  assert.equal(leaf(), first);

  t.mock.timers.tick(31 * 1000);
  await serve();
  assert.notEqual(leaf(), first);
  assert.equal(issuedAt(leaf()), Date.now());
});
