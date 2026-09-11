// The watch stream's lifecycle. Two of these guard measured defects: a
// client that goes away used to leave the WatchHub WebSocket open forever
// (11 minutes produced 1306 open sockets and 0 closed, and every write to
// the departed client rejected -- the ~300 "Network connection lost."
// lines in a conformance run were exactly this), and a re-listing
// WatchList reflector used to be served an empty stream, which emptied its
// informer cache and made the real nodelifecycle controller log "Removing
// Node" for two healthy Nodes.
import { describe, expect, it, vi } from "vite-plus/test";

vi.mock("cloudflare:workers", () => ({ DurableObject: class {} }));

import { handleWatch } from "./watch.ts";

const TOKEN = "k8flare-dev-token";

type Listener = (event: unknown) => void;

/** The DO-side WebSocket handleWatch fans out from. */
class FakeWebSocket {
  accepted = false;
  closes: { code: number; reason: string }[] = [];
  private listeners = new Map<string, Listener[]>();

  accept(): void {
    this.accepted = true;
  }
  addEventListener(type: string, fn: Listener): void {
    const fns = this.listeners.get(type) ?? [];
    fns.push(fn);
    this.listeners.set(type, fns);
  }
  close(code: number, reason: string): void {
    this.closes.push({ code, reason });
  }

  message(payload: unknown): void {
    for (const fn of this.listeners.get("message") ?? []) fn({ data: JSON.stringify(payload) });
  }
  hangUp(): void {
    for (const fn of this.listeners.get("close") ?? []) fn({});
  }
}

function newEnv() {
  const ws = new FakeWebSocket();
  const hubRequests: string[] = [];
  const env = {
    K3S_TOKEN: TOKEN,
    WATCHHUB: {
      idFromName: (name: string) => ({ name }),
      get: () => ({
        fetch: (req: Request) => {
          hubRequests.push(req.url);
          return Promise.resolve({ webSocket: ws } as unknown as Response);
        },
      }),
    },
  };
  return { ws, hubRequests, env };
}

function watch(env: unknown, path: string, opts: { auth?: boolean } = {}) {
  const url = new URL(`http://gateway.internal${path}`);
  const headers: Record<string, string> = {};
  if (opts.auth !== false) headers.Authorization = `Bearer ${TOKEN}`;
  return handleWatch(new Request(url.toString(), { headers }), env, url);
}

/** One newline-delimited JSON line from the response stream. */
async function readLine(resp: Response): Promise<string> {
  const reader = resp.body!.getReader();
  const { value, done } = await reader.read();
  reader.releaseLock();
  if (done || !value) throw new Error("stream ended before a line arrived");
  return new TextDecoder().decode(value).trimEnd();
}

function kineValue(object: unknown): string {
  return btoa(JSON.stringify(object));
}

describe("handleWatch request handling", () => {
  it("rejects an unauthenticated watch", async () => {
    const { env } = newEnv();
    const resp = await watch(env, "/api/v1/pods?watch=true", { auth: false });
    expect(resp.status).toBe(401);
  });

  it("400s a path with no storage prefix", async () => {
    const { env } = newEnv();
    const resp = await watch(env, "/healthz?watch=true");
    expect(resp.status).toBe(400);
  });

  // An empty 200 stream can never carry the initial-events-end bookmark,
  // and a WatchList reflector then blocks its factory's WaitForCacheSync
  // forever -- which is how the scheduler's bring-up wedged on resources
  // this apiserver does not serve.
  it("404s a resource this apiserver does not serve, rather than serving an empty stream", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/frobnicators?watch=true");
    expect(resp.status).toBe(404);
    expect(ws.accepted).toBe(false);
  });

  it("passes the client's resourceVersion to the hub as a replay cursor", async () => {
    const { env, hubRequests } = newEnv();
    await watch(env, "/api/v1/pods?watch=true&resourceVersion=76");
    expect(new URL(hubRequests[0]).searchParams.get("revision")).toBe("76");
    expect(new URL(hubRequests[0]).searchParams.get("prefix")).toBe("/registry/pods/");
  });

  // resourceVersion on a sendInitialEvents watch is a freshness floor, not
  // a replay cursor: replaying from it hands a re-listing reflector a
  // stream with no items at all, and Replace()ing an informer cache with
  // that empties it. The controllers then act on the phantom deletions.
  it("serves the full current state for a sendInitialEvents watch", async () => {
    const { env, hubRequests } = newEnv();
    await watch(env, "/api/v1/nodes?watch=true&sendInitialEvents=true&resourceVersion=76");
    expect(new URL(hubRequests[0]).searchParams.get("revision")).toBe("0");
  });
});

describe("handleWatch streaming", () => {
  it("streams a kine event as a Kubernetes watch event with its resourceVersion stamped", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/pods?watch=true");
    ws.message({
      events: [
        {
          create: true,
          kv: {
            key: "/registry/pods/default/nginx",
            modRevision: 42,
            value: kineValue({ kind: "Pod", metadata: { name: "nginx" } }),
          },
        },
      ],
    });

    expect(JSON.parse(await readLine(resp))).toEqual({
      type: "ADDED",
      object: { kind: "Pod", metadata: { name: "nginx", resourceVersion: "42" } },
    });
  });

  it("marks the end of the initial replay with the annotation client-go requires", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(
      env,
      "/api/v1/nodes?watch=true&sendInitialEvents=true&allowWatchBookmarks=true",
    );
    ws.message({ bookmark: 76 });

    const event = JSON.parse(await readLine(resp));
    expect(event.type).toBe("BOOKMARK");
    // Without the kind, client-go rejects the whole stream with "Object
    // 'Kind' is missing"; without the annotation, a reflector never
    // considers itself synced.
    expect(event.object.kind).toBe("Node");
    expect(event.object.metadata.annotations["k8s.io/initial-events-end"]).toBe("true");
    expect(event.object.metadata.resourceVersion).toBe("76");
  });

  it("does not send bookmarks to a client that did not ask for them", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/nodes?watch=true");
    ws.message({ bookmark: 76 });
    ws.message({
      events: [
        {
          create: true,
          kv: { key: "/registry/nodes/n1", modRevision: 77, value: kineValue({ kind: "Node" }) },
        },
      ],
    });

    expect(JSON.parse(await readLine(resp)).type).toBe("ADDED");
  });

  it("ignores a malformed hub message instead of tearing the stream down", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/pods?watch=true");
    ws.message(undefined);
    ws.message({
      events: [
        {
          create: true,
          kv: {
            key: "/registry/pods/default/a",
            modRevision: 5,
            value: kineValue({ kind: "Pod" }),
          },
        },
      ],
    });

    expect(JSON.parse(await readLine(resp)).type).toBe("ADDED");
  });

  // The lifecycle fix: a client that hangs up must take the DO-side socket
  // with it. Nothing else can notice its departure, so without this the
  // hub keeps fanning out to a stream nobody reads.
  it("closes the hub socket when the client cancels the stream", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/pods?watch=true");
    await resp.body!.cancel();

    expect(ws.closes).toEqual([{ code: 1001, reason: "watch client gone" }]);
  });

  it("drops events that arrive after the client is gone without throwing", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/pods?watch=true");
    await resp.body!.cancel();
    ws.message({
      events: [
        {
          create: true,
          kv: {
            key: "/registry/pods/default/a",
            modRevision: 9,
            value: kineValue({ kind: "Pod" }),
          },
        },
      ],
    });

    expect(ws.closes).toHaveLength(1);
  });

  it("ends the response stream when the hub socket closes", async () => {
    const { env, ws } = newEnv();
    const resp = await watch(env, "/api/v1/pods?watch=true");
    const reader = resp.body!.getReader();
    ws.hangUp();

    expect(await reader.read()).toEqual({ done: true, value: undefined });
  });
});
