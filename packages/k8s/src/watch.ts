import type { KineEvent, WatchEvent } from "./types.ts";
import { decodeKineValue } from "./helpers.ts";
import { dwAuth } from "./auth.ts";
import { urlToStoragePrefix } from "./url-mapping.ts";

/**
 * Convert a kine event (from DO WebSocket) to a Kubernetes WatchEvent.
 */
export function kineEventToWatchEvent(kineEvent: KineEvent): WatchEvent {
  let type: WatchEvent["type"];
  if (kineEvent.create) {
    type = "ADDED";
  } else if (kineEvent.delete) {
    type = "DELETED";
  } else {
    type = "MODIFIED";
  }

  let object: Record<string, unknown> = {};
  if (kineEvent.kv && kineEvent.kv.value) {
    try {
      object = JSON.parse(decodeKineValue(kineEvent.kv.value));
    } catch {
      // If value is not valid JSON, wrap it as-is
      object = { rawValue: kineEvent.kv.value };
    }
  }

  // Ensure metadata exists and set resourceVersion
  if (!object.metadata) {
    object.metadata = {};
  }
  (object.metadata as Record<string, unknown>).resourceVersion = String(kineEvent.kv.modRevision);

  return { type, object };
}

/**
 * Handle a watch request by opening a WebSocket to the KineStore DO and
 * streaming Kubernetes WatchEvent JSON lines back to the client.
 *
 * `env` is typed as `any` because the full Env type lives in @k8flare/worker.
 */
export async function handleWatch(
  req: Request,
  env: any,
  url: URL,
  ctx?: ExecutionContext,
): Promise<Response> {
  if (!dwAuth(req, env)) {
    return new Response(
      JSON.stringify({
        kind: "Status",
        apiVersion: "v1",
        status: "Failure",
        message: "Unauthorized",
        code: 401,
      }),
      { status: 401, headers: { "Content-Type": "application/json" } },
    );
  }

  // Determine storage prefix from the URL path
  const prefix = urlToStoragePrefix(url.pathname);
  if (!prefix) {
    return new Response(
      JSON.stringify({
        kind: "Status",
        apiVersion: "v1",
        status: "Failure",
        message: "unable to determine watch prefix",
        code: 400,
      }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  const resourceVersion = url.searchParams.get("resourceVersion") || "0";

  // Parse fieldSelector into an array of {field, value} pairs
  const fieldSelectorParam = url.searchParams.get("fieldSelector") || "";
  const fieldSelectors: { field: string; value: string }[] = fieldSelectorParam
    ? (fieldSelectorParam
        .split(",")
        .map((s) => {
          const eqIdx = s.indexOf("=");
          if (eqIdx === -1) return null;
          return { field: s.slice(0, eqIdx), value: s.slice(eqIdx + 1) };
        })
        .filter(Boolean) as { field: string; value: string }[])
    : [];

  // Get KineStore DO stub
  const ns = env.KINE_STORE;
  const id = ns.idFromName("default");
  const stub = ns.get(id);

  // Open WebSocket to DO
  const wsUrl = new URL("/watch", "http://do.internal");
  wsUrl.searchParams.set("prefix", prefix);
  wsUrl.searchParams.set("revision", resourceVersion);
  const wsReq = new Request(wsUrl.toString(), {
    headers: { Upgrade: "websocket" },
  });

  let wsResp: Response;
  try {
    wsResp = await stub.fetch(wsReq);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "unknown error";
    return new Response(
      JSON.stringify({
        kind: "Status",
        apiVersion: "v1",
        status: "Failure",
        message: "failed to connect to storage: " + message,
        code: 500,
      }),
      { status: 500, headers: { "Content-Type": "application/json" } },
    );
  }

  const ws = wsResp.webSocket;
  if (!ws) {
    return new Response(
      JSON.stringify({
        kind: "Status",
        apiVersion: "v1",
        status: "Failure",
        message: "failed to establish watch connection",
        code: 500,
      }),
      { status: 500, headers: { "Content-Type": "application/json" } },
    );
  }

  ws.accept();

  // Stream kine events as Kubernetes WatchEvent JSON lines
  const { readable, writable } = new TransformStream();
  const writer = writable.getWriter();
  const encoder = new TextEncoder();

  ws.addEventListener("message", (event: MessageEvent) => {
    try {
      const data = JSON.parse(event.data as string);
      if (data.events) {
        for (const kineEvent of data.events) {
          const watchEvent = kineEventToWatchEvent(kineEvent);
          // Apply fieldSelector filter if specified
          if (fieldSelectors.length > 0) {
            const obj = watchEvent.object;
            const match = fieldSelectors.every(({ field, value }) => {
              const parts = field.split(".");
              let current: unknown = obj;
              for (const part of parts) {
                if (current == null) return false;
                current = (current as Record<string, unknown>)[part];
              }
              return current === value;
            });
            if (!match) continue;
          }
          writer.write(encoder.encode(JSON.stringify(watchEvent) + "\n"));
        }
      }
    } catch {
      // Ignore malformed messages
    }
  });

  ws.addEventListener("close", () => {
    try {
      writer.close();
    } catch {
      // ignore
    }
  });

  ws.addEventListener("error", () => {
    try {
      writer.close();
    } catch {
      // ignore
    }
  });

  // Keep the Worker alive while the WebSocket is open. Without this, the
  // Worker exits after returning the Response, which terminates the DO
  // WebSocket and closes the stream immediately.
  if (ctx) {
    ctx.waitUntil(
      new Promise<void>((resolve) => {
        ws.addEventListener("close", () => resolve());
        ws.addEventListener("error", () => resolve());
      }),
    );
  }

  return new Response(readable, {
    headers: {
      "Content-Type": "application/json",
      "Transfer-Encoding": "chunked",
      "Cache-Control": "no-cache, no-transform",
    },
  });
}
