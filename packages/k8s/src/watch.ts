import type { KineEvent, KineKV, WatchEvent } from "./types.ts";
import { decodeKineValue } from "./helpers.ts";
import { dwAuth } from "./auth.ts";
import { urlToStoragePrefix, resourceKindForPath } from "./url-mapping.ts";
import {
  parseLabelSelector,
  matchesLabelSelector,
  type LabelRequirement,
} from "./label-selector.ts";

/** Decode a kine KV's JSON value into an object, stamping resourceVersion. */
function decodeKineValueObject(kv: KineKV): Record<string, unknown> {
  let object: Record<string, unknown> = {};
  if (kv.value) {
    try {
      object = JSON.parse(decodeKineValue(kv.value));
    } catch {
      // If value is not valid JSON, wrap it as-is
      object = { rawValue: kv.value };
    }
  }

  // Ensure metadata exists and set resourceVersion
  if (!object.metadata) {
    object.metadata = {};
  }
  (object.metadata as Record<string, unknown>).resourceVersion = String(kv.modRevision);

  return object;
}

/** Decode the pre-event KV carried on a kine event, if any. */
function decodePrevValueObject(kv: KineKV | undefined): Record<string, unknown> | null {
  if (!kv || !kv.value) return null;
  return decodeKineValueObject(kv);
}

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

  return { type, object: decodeKineValueObject(kineEvent.kv) };
}

interface FieldSelectorTerm {
  field: string;
  value: string;
  negate: boolean;
}

/** Parse a fieldSelector query param into individual terms. Supports both
 * "field=value" and "field!=value" (not-equal) terms, matching real
 * Kubernetes field selector syntax — real kube-scheduler's Pod informer, for
 * example, filters with "status.phase!=Succeeded,status.phase!=Failed". */
function parseFieldSelector(param: string): FieldSelectorTerm[] {
  if (!param) return [];
  return param
    .split(",")
    .map((s): FieldSelectorTerm | null => {
      // Check "!=" before "=", since "=" alone would otherwise match inside it.
      const neIdx = s.indexOf("!=");
      if (neIdx !== -1) {
        return { field: s.slice(0, neIdx), value: s.slice(neIdx + 2), negate: true };
      }
      const eqIdx = s.indexOf("=");
      if (eqIdx === -1) return null;
      return { field: s.slice(0, eqIdx), value: s.slice(eqIdx + 1), negate: false };
    })
    .filter((t): t is FieldSelectorTerm => t !== null);
}

/** Read a dotted field path from a decoded object, or undefined if any segment is missing. */
function getFieldValue(obj: Record<string, unknown>, field: string): unknown {
  let current: unknown = obj;
  for (const part of field.split(".")) {
    if (current == null || typeof current !== "object") return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

/** Check whether a decoded object satisfies both the label and field selectors. */
function objectMatchesSelectors(
  obj: Record<string, unknown>,
  labelRequirements: LabelRequirement[],
  fieldSelectors: FieldSelectorTerm[],
): boolean {
  if (labelRequirements.length > 0) {
    const metadata = obj.metadata as Record<string, unknown> | undefined;
    const labels = metadata?.labels as Record<string, string> | undefined;
    if (!matchesLabelSelector(labels, labelRequirements)) return false;
  }
  if (fieldSelectors.length > 0) {
    const match = fieldSelectors.every(({ field, value, negate }) => {
      const equal = getFieldValue(obj, field) === value;
      return negate ? !equal : equal;
    });
    if (!match) return false;
  }
  return true;
}

/**
 * Handle a watch request by opening a WebSocket to the WatchHub DO (fan-out
 * in front of Cluster's single upstream watch firehose) and streaming
 * Kubernetes WatchEvent JSON lines back to the client.
 *
 * `env` is typed as `any` because the full Env type lives in the calling
 * Worker (workers/gateway).
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
  const resourceKind = resourceKindForPath(url.pathname);
  const allowWatchBookmarks = url.searchParams.get("allowWatchBookmarks") === "true";

  const fieldSelectors = parseFieldSelector(url.searchParams.get("fieldSelector") || "");

  const labelSelectorParam = url.searchParams.get("labelSelector") || "";
  const labelRequirements = parseLabelSelector(labelSelectorParam);
  const hasSelectors = fieldSelectors.length > 0 || labelRequirements.length > 0;

  // Get WatchHub DO stub (fan-out in front of Cluster's single upstream
  // watch firehose -- see docs/multi-tenancy-and-hosting.md).
  const ns = env.WATCHHUB;
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
      if (typeof data.bookmark === "number" && resourceKind && allowWatchBookmarks) {
        // End of initial replay: tell the client's watch reflector it has
        // seen the full current state, so it can mark its cache synced.
        // The object must decode as the watched Kind, or client-go rejects
        // the whole watch stream with "Object 'Kind' is missing".
        const bookmarkEvent: WatchEvent = {
          type: "BOOKMARK",
          object: {
            kind: resourceKind.kind,
            apiVersion: resourceKind.apiVersion,
            metadata: {
              resourceVersion: String(data.bookmark),
              // client-go's reflector only treats a Bookmark as marking the
              // end of the initial events stream when this annotation is
              // present (k8s.io/apimachinery meta/v1.InitialEventsAnnotationKey).
              // Without it, reflectors log "hasn't received required bookmark
              // event" every 10s and never consider themselves synced.
              annotations: { "k8s.io/initial-events-end": "true" },
            },
          },
        };
        writer.write(encoder.encode(JSON.stringify(bookmarkEvent) + "\n"));
      }
      if (data.events) {
        for (const kineEvent of data.events) {
          const watchEvent = kineEventToWatchEvent(kineEvent);
          let finalType = watchEvent.type;
          let finalObject = watchEvent.object;

          if (hasSelectors) {
            const newMatches = objectMatchesSelectors(
              watchEvent.object,
              labelRequirements,
              fieldSelectors,
            );

            if (watchEvent.type === "ADDED") {
              if (!newMatches) continue;
            } else if (watchEvent.type === "DELETED") {
              const oldObj = decodePrevValueObject(kineEvent.prevKV);
              const oldMatches =
                oldObj !== null &&
                objectMatchesSelectors(oldObj, labelRequirements, fieldSelectors);
              if (!oldMatches && !newMatches) continue;
            } else {
              // MODIFIED: a selector transition can turn this into a synthetic
              // ADDED/DELETED, or suppress it entirely, mirroring how a real
              // apiserver watch cache treats objects entering/leaving a
              // selector's view.
              const oldObj = decodePrevValueObject(kineEvent.prevKV);
              const oldMatches =
                oldObj !== null &&
                objectMatchesSelectors(oldObj, labelRequirements, fieldSelectors);
              if (oldMatches && newMatches) {
                // stays MODIFIED
              } else if (!oldMatches && newMatches) {
                finalType = "ADDED";
              } else if (oldMatches && !newMatches) {
                finalType = "DELETED";
                finalObject = oldObj as Record<string, unknown>;
              } else {
                continue;
              }
            }
          }

          writer.write(
            encoder.encode(JSON.stringify({ type: finalType, object: finalObject }) + "\n"),
          );
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
      "Content-Encoding": "identity",
    },
  });
}
