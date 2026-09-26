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

// A log stream is one-way and carries no channel byte: binary.k8s.io wants raw
// binary frames, base64.k8s.io wants the same bytes base64-encoded as text.
export function sendLog(ws: WebSocket, protocol: string, bytes: Uint8Array): void {
  if (bytes.byteLength === 0) return;
  if (protocol === "base64.k8s.io") {
    let binary = "";
    for (const b of bytes) binary += String.fromCharCode(b);
    ws.send(btoa(binary));
    return;
  }
  ws.send(bytes);
}
