import assert from "node:assert/strict";
import { test } from "node:test";
import type { ExecOptions, ExecStreams } from "../src/podkubelet/kubelet.ts";
import { STREAM_CLOSE, STREAM_ERR, STREAM_RESIZE, STREAM_STDIN, STREAM_STDOUT, channelCodec } from "../src/podkubelet/protocol.ts";
import { ATTACH_UNAVAILABLE, serveAttach, serveExec, servePortForward, type StreamKubelet } from "../src/podkubelet/streams.ts";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

class FakeSocket {
  sent: Uint8Array[] = [];
  closed: { code?: number; reason?: string } | null = null;
  private listeners: Record<string, Array<(ev: { data: unknown }) => void>> = {};

  send(data: Uint8Array | string): void {
    if (this.closed) throw new Error("socket closed");
    this.sent.push(typeof data === "string" ? encoder.encode(data) : data);
  }

  close(code?: number, reason?: string): void {
    if (this.closed) return;
    this.closed = { code, reason };
    for (const fn of this.listeners.close ?? []) fn({ data: undefined });
  }

  addEventListener(type: string, listener: (ev: { data: unknown }) => void): void {
    (this.listeners[type] ??= []).push(listener);
  }

  receive(bytes: Uint8Array): void {
    for (const fn of this.listeners.message ?? []) fn({ data: bytes });
  }

  frames(): Array<{ channel: number; text: string }> {
    return this.sent.map((f) => ({ channel: f[0], text: decoder.decode(f.subarray(1)) }));
  }
}

async function readAll(stream: ReadableStream<Uint8Array>): Promise<string> {
  let out = "";
  const reader = stream.getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) return out;
    out += decoder.decode(value);
  }
}

function tick(): Promise<void> {
  return new Promise((r) => setTimeout(r, 5));
}

class FakeKubelet implements StreamKubelet {
  execs: Array<{ command: string[]; options: ExecOptions }> = [];
  stdinText: Promise<string> | null = null;
  resizes: string[] = [];
  exitCode = 0;
  stdoutText = "hello\n";
  connects: Array<{ port: number; input: Promise<string> }> = [];
  connectError: string | null = null;

  async exec(command: string[], options: ExecOptions): Promise<ExecStreams> {
    this.execs.push({ command, options });
    if (options.stdin) this.stdinText = readAll(options.stdin);
    void (async () => {
      const reader = options.control.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) return;
        this.resizes.push(decoder.decode(value));
      }
    })();
    const stdinDone = this.stdinText ?? Promise.resolve("");
    const stdout = new ReadableStream<Uint8Array>({
      start: (c) => {
        c.enqueue(encoder.encode(this.stdoutText));
        void stdinDone.then(() => c.close());
      },
    });
    const status = new ReadableStream<Uint8Array>({
      start: (c) => {
        c.enqueue(encoder.encode(JSON.stringify(this.exitCode === 0 ? { metadata: {}, status: "Success" } : { metadata: {}, status: "Failure", reason: "NonZeroExitCode" })));
        c.close();
      },
    });
    return { stdout: options.stdout ? stdout : null, stderr: null, status };
  }

  async connectPort(port: number, input: ReadableStream<Uint8Array>): Promise<ReadableStream<Uint8Array>> {
    if (this.connectError) throw new Error(this.connectError);
    const inputDone = readAll(input);
    this.connects.push({ port, input: inputDone });
    return new ReadableStream<Uint8Array>({
      start: (c) => {
        c.enqueue(encoder.encode(`echo ${port}`));
        void inputDone.then(() => c.close());
      },
    });
  }
}

test("exec over v5.channel.k8s.io: empty first frame, stdout frames, Status on the error channel, stdin and resize relayed, close signal half-closes stdin", async () => {
  const socket = new FakeSocket();
  const kubelet = new FakeKubelet();
  const done = serveExec(socket, channelCodec("v5.channel.k8s.io")!, kubelet, "input=1&output=1&tty=1&command=sh&command=-c&command=cat");
  await tick();
  socket.receive(new Uint8Array([STREAM_STDIN, ...encoder.encode("typed")]));
  socket.receive(new Uint8Array([STREAM_RESIZE, ...encoder.encode('{"Width":80,"Height":24}')]));
  socket.receive(new Uint8Array([STREAM_CLOSE, STREAM_STDIN]));
  await done;
  assert.deepEqual(kubelet.execs[0].command, ["sh", "-c", "cat"]);
  assert.equal(kubelet.execs[0].options.tty, true);
  assert.equal(await kubelet.stdinText, "typed");
  assert.deepEqual(kubelet.resizes, ['{"Width":80,"Height":24}']);
  const frames = socket.frames();
  assert.deepEqual(frames[0], { channel: STREAM_STDOUT, text: "" });
  assert.deepEqual(frames[1], { channel: STREAM_STDOUT, text: "hello\n" });
  assert.equal(frames[2].channel, STREAM_ERR);
  assert.deepEqual(JSON.parse(frames[2].text), { metadata: {}, status: "Success" });
  assert.deepEqual(socket.closed, { code: 1000, reason: "done" });
});

test("exec without a command and a kubelet failure both end with an InternalError Status", async () => {
  const socket = new FakeSocket();
  await serveExec(socket, channelCodec("v5.channel.k8s.io")!, new FakeKubelet(), "output=1");
  assert.match(socket.frames()[1].text, /no command specified.*"reason":"InternalError"/);
  assert.equal(socket.closed?.code, 1008);
  const failing = new FakeSocket();
  const kubelet = new FakeKubelet();
  kubelet.exec = async () => {
    throw new Error("pod default/web has no running container on cloudflare");
  };
  await serveExec(failing, channelCodec("v5.channel.k8s.io")!, kubelet, "output=1&command=ls");
  const frames = failing.frames();
  assert.equal(frames[1].channel, STREAM_ERR);
  assert.match(frames[1].text, /Internal error occurred: pod default\/web has no running container on cloudflare/);
  assert.equal(failing.closed?.code, 1011);
});

test("port-forward writes the port on both channels of each pair, relays data by channel and reports dial errors on the error channel", async () => {
  const socket = new FakeSocket();
  const kubelet = new FakeKubelet();
  const done = servePortForward(socket, channelCodec("v4.channel.k8s.io")!, kubelet, "port=8080&port=9090", { name: "web", uid: "uid-1" });
  await tick();
  socket.receive(new Uint8Array([2, ...encoder.encode("GET /")]));
  socket.close(1000, "client done");
  await done;
  const frames = socket.sent;
  assert.deepEqual([...frames[0]], [0, 0x90, 0x1f]);
  assert.deepEqual([...frames[1]], [1, 0x90, 0x1f]);
  assert.deepEqual([...frames[2]], [2, 0x82, 0x23]);
  assert.deepEqual([...frames[3]], [3, 0x82, 0x23]);
  const data = frames.slice(4).map((f) => ({ channel: f[0], text: decoder.decode(f.subarray(1)) })).sort((a, b) => a.channel - b.channel);
  assert.deepEqual(data, [
    { channel: 0, text: "echo 8080" },
    { channel: 2, text: "echo 9090" },
  ]);
  assert.deepEqual(kubelet.connects.map((c) => c.port), [8080, 9090]);
  assert.equal(await kubelet.connects[1].input, "GET /");
  const failing = new FakeSocket();
  const refusing = new FakeKubelet();
  refusing.connectError = "connection refused";
  await servePortForward(failing, channelCodec("")!, refusing, "port=5000", { name: "web", uid: "uid-1" });
  assert.deepEqual(failing.frames()[2], { channel: 1, text: "error forwarding port 5000 to pod web, uid uid-1: connection refused" });
  assert.equal(failing.closed?.code, 1000);
});

test("attach answers with a clear Status instead of pretending to attach", () => {
  const socket = new FakeSocket();
  serveAttach(socket, channelCodec("v5.channel.k8s.io")!, "output=1&input=1");
  const frames = socket.frames();
  assert.deepEqual(frames[0], { channel: STREAM_STDOUT, text: "" });
  assert.equal(frames[1].channel, STREAM_ERR);
  assert.equal(JSON.parse(frames[1].text).message, `Internal error occurred: ${ATTACH_UNAVAILABLE}`);
  assert.equal(socket.closed?.code, 1000);
});
