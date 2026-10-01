import assert from "node:assert/strict";
import { test } from "node:test";
import {
  STREAM_CLOSE,
  STREAM_STDIN,
  STREAM_STDOUT,
  channelCodec,
  encodeStatus,
  exitStatus,
  parseExecQuery,
  parsePortForwardQuery,
  parseStreamLocation,
  parseTerminalSize,
  portFrame,
} from "../src/podkubelet/protocol.ts";

const bytes = (...b: number[]) => new Uint8Array(b);

test("v5.channel.k8s.io frames are channel byte plus payload and 255 is the stream close signal", () => {
  const codec = channelCodec("v5.channel.k8s.io")!;
  assert.equal(codec.streamClose, true);
  assert.deepEqual(codec.encode(STREAM_STDOUT, new TextEncoder().encode("hi")), bytes(1, 104, 105));
  assert.deepEqual(codec.decode(bytes(0, 108, 115)), { channel: STREAM_STDIN, data: bytes(108, 115) });
  assert.deepEqual(codec.decode(bytes(STREAM_CLOSE, STREAM_STDIN)), { close: STREAM_STDIN });
  assert.equal(codec.decode(bytes(STREAM_CLOSE, 0, 0)), null);
  assert.equal(codec.decode(bytes()), null);
});

test("v4 and the empty protocol are binary without the close signal; base64 variants prefix the channel digit", () => {
  const v4 = channelCodec("v4.channel.k8s.io")!;
  assert.equal(v4.streamClose, false);
  assert.deepEqual(v4.decode(bytes(255, 0)), { channel: 255, data: bytes(0) });
  assert.ok(channelCodec(""));
  const b64 = channelCodec("v4.base64.channel.k8s.io")!;
  assert.equal(b64.encode(STREAM_STDOUT, new TextEncoder().encode("hi")), "1aGk=");
  assert.deepEqual(b64.decode("0bHM="), { channel: STREAM_STDIN, data: bytes(108, 115) });
  assert.equal(channelCodec("SPDY/3.1+portforward.k8s.io"), null);
});

test("exec query flags follow the kubelet's names and command repeats", () => {
  assert.deepEqual(parseExecQuery("input=1&output=1&tty=1&command=sh&command=-c&command=echo%20hi"), { command: ["sh", "-c", "echo hi"], stdin: true, stdout: true, stderr: false, tty: true });
  assert.deepEqual(parseExecQuery("stdout=true&stderr=true&command=ls"), { command: ["ls"], stdin: false, stdout: true, stderr: true, tty: false });
});

test("port-forward ports come from repeated or comma-joined port parameters as little-endian frames", () => {
  assert.deepEqual(parsePortForwardQuery("port=8080&port=9090,443"), [8080, 9090, 443]);
  assert.deepEqual(parsePortForwardQuery("ports=80"), [80]);
  assert.deepEqual(parsePortForwardQuery("port=0&port=70000&port=abc"), []);
  assert.deepEqual(portFrame(8080), bytes(0x90, 0x1f));
});

test("the exit status is the apiserver's metav1.Status JSON on the error channel", () => {
  assert.deepEqual(JSON.parse(new TextDecoder().decode(encodeStatus(exitStatus(0)))), { metadata: {}, status: "Success" });
  assert.deepEqual(JSON.parse(new TextDecoder().decode(encodeStatus(exitStatus(3)))), {
    metadata: {},
    status: "Failure",
    message: "command terminated with non-zero exit code: command terminated with exit code 3",
    reason: "NonZeroExitCode",
    details: { causes: [{ reason: "ExitCode", message: "3" }] },
  });
});

test("terminal sizes are client-go TerminalSize JSON", () => {
  assert.deepEqual(parseTerminalSize(new TextEncoder().encode('{"Width":120,"Height":40}')), { cols: 120, rows: 40 });
  assert.equal(parseTerminalSize(new TextEncoder().encode("nope")), null);
});

test("located stream URLs are parsed with the query from either the search or the _q segment", () => {
  assert.deepEqual(parseStreamLocation("https://nodetunnel.internal/node/cloudflare/exec/default/web/app/_q/command%3Dls%26output%3D1?command=ls&output=1"), {
    kind: "exec",
    namespace: "default",
    name: "web",
    container: "app",
    search: "command=ls&output=1",
  });
  assert.deepEqual(parseStreamLocation("https://nodetunnel.internal/node/cloudflare/portForward/default/web/_q/port%3D8080"), { kind: "portForward", namespace: "default", name: "web", search: "port=8080" });
  assert.deepEqual(parseStreamLocation("https://nodetunnel.internal/node/cloudflare/containerLogs/default/web/app?follow=true"), { kind: "containerLogs", namespace: "default", name: "web", container: "app", search: "follow=true" });
  assert.equal(parseStreamLocation("https://nodetunnel.internal/dial/cloudflare/10.42.255.2/80/"), null);
});
