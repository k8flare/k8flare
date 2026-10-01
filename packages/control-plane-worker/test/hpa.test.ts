import assert from "node:assert/strict";
import { test } from "node:test";
import { consume } from "../src/queues.ts";

function batchOf(bodies: unknown[]) {
  const batch = {
    queue: "k8flare-hpa",
    messages: bodies.map((body) => ({ body })),
    acked: false,
    ackAll: () => void (batch.acked = true),
  };
  return batch;
}

test("a message on the HPA queue calls the HPA controller entrypoint and acks the batch", async () => {
  let synced = false;
  const passes: string[] = [];
  const env = {
    HPA: {
      sync: async () => {
        synced = true;
        return {
          objects: { horizontalpodautoscalers: 1, pods: 2 },
          drained: true,
          nextMs: 0,
        };
      },
    },
    CLUSTER: {
      idFromName: (name: string) => name,
      get: () => ({
        fetch: async (url: string, init?: RequestInit) => {
          if (new URL(url).pathname === "/pass") {
            const body = JSON.parse(String(init?.body ?? "{}"));
            passes.push(body.target);
            return Response.json({ ok: true });
          }
          return new Response("{}");
        },
      }),
    },
  };

  const batch = batchOf([{ kind: "retry" }]);
  await consume(batch as any, env as any);

  assert.equal(synced, true);
  assert.equal(batch.acked, true);
  assert.deepEqual(passes, ["hpa"]);
});
