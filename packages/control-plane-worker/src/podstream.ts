export function asBytes(data: unknown): Uint8Array {
  if (typeof data === "string") {
    const out = new Uint8Array(data.length);
    for (let i = 0; i < data.length; i++) out[i] = data.charCodeAt(i) & 0xff;
    return out;
  }
  if (data && typeof data === "object" && "byteLength" in data) {
    const v = data as ArrayBuffer | ArrayBufferView;
    if ("buffer" in v && "byteOffset" in v) {
      const view = v as ArrayBufferView;
      return new Uint8Array(view.buffer, view.byteOffset, view.byteLength).slice();
    }
    return new Uint8Array(v as ArrayBuffer).slice();
  }
  return new Uint8Array();
}

export async function bytesOf(data: unknown): Promise<Uint8Array> {
  if (data && typeof data === "object" && "size" in data && "arrayBuffer" in data) {
    const blob = data as { arrayBuffer?: () => Promise<ArrayBuffer> };
    if (typeof blob.arrayBuffer === "function") {
      return new Uint8Array(await blob.arrayBuffer());
    }
  }
  return asBytes(data);
}

export function sendBinary(ws: WebSocket, data: unknown): void {
  ws.send(asBytes(data));
}
