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

test("a message on the HPA queue scrapes metrics, calls the HPA controller entrypoint, and follows up", async () => {
  let scraped = false;
  let synced = false;
  const followUps: Record<string, unknown>[] = [];
  const passes: string[] = [];
  const env = {
    ADMIN_TOKEN: "admin-test-token",
    HPA: {
      sync: async () => {
        synced = true;
        return {
          objects: { horizontalpodautoscalers: 2, pods: 3 },
          drained: true,
          nextMs: 0,
        };
      },
    },
    __apiserver: async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname === "/internal/metrics/scrape") {
        scraped = true;
        return Response.json({ nodes: 1 });
      }
      if (url.pathname === "/internal/queue/followup") {
        const body = (await request.json()) as Record<string, unknown>;
        followUps.push(body);
        return Response.json({ sends: [], stop: false });
      }
      return new Response("{}", { status: 200 });
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

  assert.equal(scraped, true);
  assert.equal(synced, true);
  assert.equal(batch.acked, true);
  assert.deepEqual(passes, ["hpa"]);
  assert.equal(followUps.length, 1);
  assert.deepEqual(followUps[0], { target: "hpa", hasResult: true, drained: true, hpas: 2 });
});
