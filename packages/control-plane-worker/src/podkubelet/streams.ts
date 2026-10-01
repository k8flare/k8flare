import { bytesOf } from "../podstream.ts";
import { LOGS_UNAVAILABLE } from "./dial.ts";
import type { ExecOptions, ExecStreams } from "./kubelet.ts";
import {
  STREAM_ERR,
  STREAM_RESIZE,
  STREAM_STDERR,
  STREAM_STDIN,
  STREAM_STDOUT,
  channelCodec,
  encodeStatus,
  internalErrorStatus,
  parseExecQuery,
  parsePortForwardQuery,
  parseStreamLocation,
  portFrame,
  type ChannelCodec,
  type StreamLocation,
} from "./protocol.ts";
import { VIRTUAL_NODE } from "./spec.ts";
import { podKubeletStub, podLedgerStub } from "./wake.ts";

export interface StreamKubelet {
  exec(command: string[], options: ExecOptions): Promise<ExecStreams>;
  connectPort(port: number, input: ReadableStream<Uint8Array>): Promise<ReadableStream<Uint8Array>>;
}

export interface StreamSocket {
  send(data: Uint8Array | string): void;
  close(code?: number, reason?: string): void;
  addEventListener(type: "message", listener: (ev: { data: unknown }) => void): void;
  addEventListener(type: "close" | "error", listener: () => void): void;
}

export const ATTACH_UNAVAILABLE = `attach is not supported for Pods on node ${VIRTUAL_NODE}: the container's stdio belongs to the platform; use exec`;

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function closeQuietly(socket: StreamSocket, code: number, reason: string): void {
  try {
    socket.close(code, reason.slice(0, 120));
  } catch {}
}

function relayInOrder(socket: StreamSocket, onMessage: (bytes: Uint8Array) => void, onClose: () => void): () => void {
  let queue: Promise<void> = Promise.resolve();
  socket.addEventListener("message", (ev) => {
    queue = queue.then(() => bytesOf(ev.data)).then(onMessage, () => {});
  });
  const close = () => {
    queue = queue.then(onClose, () => {});
  };
  socket.addEventListener("close", close);
  socket.addEventListener("error", close);
  return close;
}

async function pump(stream: ReadableStream<Uint8Array>, write: (bytes: Uint8Array) => void): Promise<void> {
  const reader = stream.getReader();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) return;
      if (value && value.byteLength > 0) write(value);
    }
  } finally {
    reader.releaseLock();
  }
}

async function readAll(stream: ReadableStream<Uint8Array>): Promise<Uint8Array> {
  const chunks: Uint8Array[] = [];
  await pump(stream, (bytes) => chunks.push(bytes));
  const out = new Uint8Array(chunks.reduce((n, c) => n + c.byteLength, 0));
  let offset = 0;
  for (const c of chunks) {
    out.set(c, offset);
    offset += c.byteLength;
  }
  return out;
}

export async function serveExec(socket: StreamSocket, codec: ChannelCodec, kubelet: StreamKubelet, search: string): Promise<void> {
  const req = parseExecQuery(search);
  const stdin = req.stdin ? new TransformStream<Uint8Array, Uint8Array>() : null;
  const control = new TransformStream<Uint8Array, Uint8Array>();
  const stdinWriter = stdin?.writable.getWriter();
  const controlWriter = control.writable.getWriter();
  let open = true;
  const send = (channel: number, data: Uint8Array) => {
    if (!open) throw new Error("client closed");
    socket.send(codec.encode(channel, data));
  };
  const closeInputs = relayInOrder(
    socket,
    (bytes) => {
      const frame = codec.decode(bytes);
      if (!frame) return;
      if ("close" in frame) {
        if (frame.close === STREAM_STDIN) void stdinWriter?.close().catch(() => {});
        return;
      }
      if (frame.channel === STREAM_STDIN) void stdinWriter?.write(frame.data).catch(() => {});
      else if (frame.channel === STREAM_RESIZE) void controlWriter.write(frame.data).catch(() => {});
    },
    () => {
      open = false;
      void stdinWriter?.close().catch(() => {});
      void controlWriter.close().catch(() => {});
    },
  );
  const finish = () => {
    open = false;
    closeInputs();
  };
  send(req.stdout ? STREAM_STDOUT : req.stderr ? STREAM_STDERR : STREAM_ERR, new Uint8Array());
  if (req.command.length === 0) {
    send(STREAM_ERR, encodeStatus(internalErrorStatus("no command specified")));
    finish();
    closeQuietly(socket, 1008, "no command specified");
    return;
  }
  try {
    const streams = await kubelet.exec(req.command, {
      stdin: stdin?.readable ?? null,
      stdout: req.stdout,
      stderr: req.stderr,
      tty: req.tty,
      control: control.readable,
    });
    const pumps: Promise<void>[] = [];
    if (streams.stdout) pumps.push(pump(streams.stdout, (bytes) => send(STREAM_STDOUT, bytes)));
    if (streams.stderr) pumps.push(pump(streams.stderr, (bytes) => send(STREAM_STDERR, bytes)));
    await Promise.all(pumps);
    send(STREAM_ERR, await readAll(streams.status));
    finish();
    closeQuietly(socket, 1000, "done");
  } catch (err) {
    if (open) {
      try {
        send(STREAM_ERR, encodeStatus(internalErrorStatus(errorText(err))));
      } catch {}
    }
    finish();
    closeQuietly(socket, 1011, errorText(err));
  }
}

export async function servePortForward(socket: StreamSocket, codec: ChannelCodec, kubelet: StreamKubelet, search: string, pod: { name: string; uid: string }): Promise<void> {
  const ports = parsePortForwardQuery(search);
  if (ports.length === 0) {
    closeQuietly(socket, 1008, 'query parameter "port" is required');
    return;
  }
  let open = true;
  const send = (channel: number, data: Uint8Array) => {
    if (!open) throw new Error("client closed");
    socket.send(codec.encode(channel, data));
  };
  const inputs = ports.map(() => new TransformStream<Uint8Array, Uint8Array>());
  const writers = inputs.map((t) => t.writable.getWriter());
  const closeInputs = relayInOrder(
    socket,
    (bytes) => {
      const frame = codec.decode(bytes);
      if (!frame || "close" in frame) return;
      if (frame.channel % 2 !== 0) return;
      const writer = writers[frame.channel / 2];
      if (writer) void writer.write(frame.data).catch(() => {});
    },
    () => {
      open = false;
      for (const w of writers) void w.close().catch(() => {});
    },
  );
  const finish = () => {
    open = false;
    closeInputs();
  };
  for (let i = 0; i < ports.length; i++) {
    send(i * 2, portFrame(ports[i]));
    send(i * 2 + 1, portFrame(ports[i]));
  }
  await Promise.all(
    ports.map(async (port, i) => {
      try {
        const readable = await kubelet.connectPort(port, inputs[i].readable);
        await pump(readable, (bytes) => send(i * 2, bytes));
      } catch (err) {
        if (!open) return;
        try {
          send(i * 2 + 1, new TextEncoder().encode(`error forwarding port ${port} to pod ${pod.name}, uid ${pod.uid}: ${errorText(err)}`));
        } catch {}
      }
    }),
  );
  finish();
  closeQuietly(socket, 1000, "done");
}

export function serveAttach(socket: StreamSocket, codec: ChannelCodec, search: string): void {
  const req = parseExecQuery(search);
  try {
    socket.send(codec.encode(req.stdout ? STREAM_STDOUT : req.stderr ? STREAM_STDERR : STREAM_ERR, new Uint8Array()));
    socket.send(codec.encode(STREAM_ERR, encodeStatus(internalErrorStatus(ATTACH_UNAVAILABLE))));
  } catch {}
  closeQuietly(socket, 1000, "done");
}

export async function servePodStream(env: Env, ctx: ExecutionContext, loc: { url: string; protocol?: string }): Promise<Response> {
  const location = parseStreamLocation(loc.url);
  const protocol = loc.protocol ?? "";
  const pair = new WebSocketPair();
  const server = pair[1];
  server.accept();
  ctx.waitUntil(dispatch(env, server, location, protocol));
  const headers = new Headers();
  if (protocol) headers.set("Sec-WebSocket-Protocol", protocol);
  return new Response(null, { status: 101, webSocket: pair[0], headers });
}

async function dispatch(env: Env, server: WebSocket, location: StreamLocation | null, protocol: string): Promise<void> {
  if (!location) {
    closeQuietly(server, 1008, "unsupported stream path");
    return;
  }
  if (location.kind === "containerLogs") {
    closeQuietly(server, 1008, LOGS_UNAVAILABLE);
    return;
  }
  const codec = channelCodec(protocol);
  if (!codec) {
    closeQuietly(server, 1002, `requested protocol ${protocol} is not supported`);
    return;
  }
  if (location.kind === "attach") {
    serveAttach(server, codec, location.search);
    return;
  }
  const entries = await podLedgerStub(env).lookup(location.namespace, location.name);
  const entry = entries.find((e) => e.running) ?? entries[entries.length - 1];
  if (!entry) {
    closeQuietly(server, 1008, `pod ${location.namespace}/${location.name} has no container on ${VIRTUAL_NODE}`);
    return;
  }
  const kubelet = podKubeletStub(env, entry.uid) as unknown as StreamKubelet;
  if (location.kind === "exec") await serveExec(server, codec, kubelet, location.search);
  else await servePortForward(server, codec, kubelet, location.search, { name: location.name, uid: entry.uid });
}
