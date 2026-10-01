export const STREAM_STDIN = 0;
export const STREAM_STDOUT = 1;
export const STREAM_STDERR = 2;
export const STREAM_ERR = 3;
export const STREAM_RESIZE = 4;
export const STREAM_CLOSE = 255;

export const CHANNEL_PROTOCOL_V5 = "v5.channel.k8s.io";

const binaryChannelProtocols = new Set(["", "channel.k8s.io", "v4.channel.k8s.io", CHANNEL_PROTOCOL_V5]);
const base64ChannelProtocols = new Set(["base64.channel.k8s.io", "v4.base64.channel.k8s.io"]);

export type Frame = { channel: number; data: Uint8Array } | { close: number };

export interface ChannelCodec {
  readonly protocol: string;
  readonly streamClose: boolean;
  encode(channel: number, data: Uint8Array): Uint8Array | string;
  decode(message: Uint8Array | string): Frame | null;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder();

function latin1(data: Uint8Array | string): string {
  if (typeof data === "string") return data;
  let out = "";
  for (const b of data) out += String.fromCharCode(b);
  return out;
}

export function toBytes(data: Uint8Array | string): Uint8Array {
  if (typeof data !== "string") return data;
  const out = new Uint8Array(data.length);
  for (let i = 0; i < data.length; i++) out[i] = data.charCodeAt(i) & 0xff;
  return out;
}

export function channelCodec(protocol: string): ChannelCodec | null {
  if (binaryChannelProtocols.has(protocol)) {
    const streamClose = protocol === CHANNEL_PROTOCOL_V5;
    return {
      protocol,
      streamClose,
      encode(channel, data) {
        const frame = new Uint8Array(data.byteLength + 1);
        frame[0] = channel;
        frame.set(data, 1);
        return frame;
      },
      decode(message) {
        const bytes = toBytes(message);
        if (bytes.byteLength === 0) return null;
        if (streamClose && bytes[0] === STREAM_CLOSE) {
          if (bytes.byteLength !== 2) return null;
          return { close: bytes[1] };
        }
        return { channel: bytes[0], data: bytes.subarray(1) };
      },
    };
  }
  if (base64ChannelProtocols.has(protocol)) {
    return {
      protocol,
      streamClose: false,
      encode(channel, data) {
        return String.fromCharCode(48 + channel) + btoa(latin1(data));
      },
      decode(message) {
        const text = latin1(message);
        if (text.length === 0) return null;
        const channel = text.charCodeAt(0) - 48;
        try {
          return { channel, data: toBytes(atob(text.slice(1))) };
        } catch {
          return null;
        }
      },
    };
  }
  return null;
}

export interface ExecRequest {
  command: string[];
  stdin: boolean;
  stdout: boolean;
  stderr: boolean;
  tty: boolean;
}

function flag(q: URLSearchParams, ...keys: string[]): boolean {
  return keys.some((k) => {
    const v = q.get(k);
    return v === "1" || v === "true";
  });
}

export function parseExecQuery(search: string): ExecRequest {
  const q = new URLSearchParams(search);
  return {
    command: q.getAll("command"),
    stdin: flag(q, "input", "stdin"),
    stdout: flag(q, "output", "stdout"),
    stderr: flag(q, "error", "stderr"),
    tty: flag(q, "tty"),
  };
}

export function parsePortForwardQuery(search: string): number[] {
  const q = new URLSearchParams(search);
  const ports: number[] = [];
  for (const raw of [...q.getAll("port"), ...q.getAll("ports")]) {
    for (const part of raw.split(",")) {
      const n = Number(part);
      if (Number.isInteger(n) && n > 0 && n < 65536) ports.push(n);
    }
  }
  return ports;
}

export function portFrame(port: number): Uint8Array {
  return new Uint8Array([port & 0xff, (port >> 8) & 0xff]);
}

export interface StreamStatus {
  metadata: Record<string, never>;
  status: "Success" | "Failure";
  message?: string;
  reason?: string;
  details?: { causes: Array<{ reason: string; message: string }> };
  code?: number;
}

export function exitStatus(exitCode: number): StreamStatus {
  if (exitCode === 0) return { metadata: {}, status: "Success" };
  return {
    metadata: {},
    status: "Failure",
    message: `command terminated with non-zero exit code: command terminated with exit code ${exitCode}`,
    reason: "NonZeroExitCode",
    details: { causes: [{ reason: "ExitCode", message: String(exitCode) }] },
  };
}

export function internalErrorStatus(message: string): StreamStatus {
  return { metadata: {}, status: "Failure", message: `Internal error occurred: ${message}`, reason: "InternalError", code: 500 };
}

export function encodeStatus(status: StreamStatus): Uint8Array {
  return encoder.encode(JSON.stringify(status));
}

export interface TerminalSize {
  cols: number;
  rows: number;
}

export function parseTerminalSize(data: Uint8Array): TerminalSize | null {
  try {
    const parsed = JSON.parse(decoder.decode(data)) as { Width?: unknown; Height?: unknown };
    if (typeof parsed.Width !== "number" || typeof parsed.Height !== "number") return null;
    return { cols: parsed.Width, rows: parsed.Height };
  } catch {
    return null;
  }
}

export interface StreamLocation {
  kind: "exec" | "attach" | "portForward" | "containerLogs";
  namespace: string;
  name: string;
  container?: string;
  search: string;
}

export function parseStreamLocation(url: string): StreamLocation | null {
  const u = new URL(url);
  const parts = u.pathname.split("/").filter(Boolean);
  if (parts[0] !== "node" || parts.length < 5) return null;
  const kind = parts[2];
  if (kind !== "exec" && kind !== "attach" && kind !== "portForward" && kind !== "containerLogs") return null;
  const location: StreamLocation = { kind, namespace: parts[3], name: parts[4], search: u.search.slice(1) };
  if (kind !== "portForward" && parts[5] && parts[5] !== "_q") location.container = parts[5];
  if (!location.search) {
    const q = parts.indexOf("_q");
    if (q >= 0 && parts[q + 1]) location.search = decodeURIComponent(parts[q + 1]);
  }
  return location;
}
