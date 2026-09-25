# Handoff: pod logs over websockets -- empty payload. It is the wrong TRANSPORT, not framing.

State: `3ceecf4` fixed the routing. The upgrade now terminates in the shell Worker instead of being
carried into a Go worker that cannot answer it. Hang gone: 30 s+ with 2 `bridge: fetch timed out` ->
6.2 s with 0 timeouts. Remaining failure: `Unexpected websocket logs:` -- handshake completes, zero
log bytes arrive.

## The relay does NOT reshape bytes. Ruled out by reading.
control-plane-worker/src/podstream.ts:28-30
    export function sendBinary(ws: WebSocket, data: unknown): void { ws.send(asBytes(data)); }
No channel byte is added or stripped anywhere in the shell relay. Both directions are a straight
byte pass-through: control-plane-worker/src/index.ts:120-127 (client->sink) and :164-169
(upstream->client). The channel prefix that exec and attach carry is produced by the KUBELET, which
speaks the channel protocol natively -- not by us. So the premise "logs must bypass our prefixing"
does not apply; there is no prefixing to bypass.

## The actual cause: the shell asks the NodeTunnel for a websocket the kubelet does not have.
index.ts:143-160 builds the upstream request with an upgrade and requires a socket back:
    headers.set("Upgrade", "websocket"); headers.set("Connection", "Upgrade");
    const upstreamResp = await stub.fetch(dest.toString(), { headers });
    const upstream = upstreamResp.webSocket;
    if (!upstream) { server.close(1011, "no upstream socket"); return; }
and node-tunnel/src/index.ts:106 only handles `/node/...` when `Upgrade: websocket` is present.
The kubelet's exec, attach and portForward endpoints ARE websocket/SPDY endpoints, which is why this
works for those three. `containerLogs` is NOT -- it is a plain HTTP GET returning text/plain.
So for a log request the tunnel is asked to upgrade something that never upgrades: no webSocket comes
back, the shell closes 1011 with zero bytes, and the client sees an opened-then-closed socket with an
empty payload. The ~6 s is the dial-and-close cycle, which is why it no longer hits any 30 s budget.
Note `server.accept()` and the 101 are returned at :117 and :198 BEFORE the waitUntil work runs, so
the handshake succeeding tells you nothing about the upstream -- which is exactly what the symptom
looks like.

## The transport that works already exists in this repo
apiserver-core/podlog.go:61-74 is the non-websocket `kubectl logs` path. It builds
    /node/<node>/containerLogs/<ns>/<pod>/<container>
and fetches it over `r.proxy.Transport` as ordinary HTTP with ContentType text/plain. Same URL the
stream locator now produces after `3ceecf4` mapped kind `log` -> `containerLogs`
(edgehost/stream.go:43-46). So the proven path is plain HTTP through the tunnel; only the shell's
half needs a second shape.

## The change, in one place
In the `ctx.waitUntil` block at index.ts:128-197, branch on the log kind:
 1. `stub.fetch(dest.toString())` WITHOUT the Upgrade/Connection headers (keep Authorization and
    X-Stream-Query).
 2. Take `upstreamResp.body` -- a ReadableStream -- and pump it to the client:
        const reader = upstreamResp.body.getReader();
        for (;;) { const {done, value} = await reader.read(); if (done) break;
                   if (value?.byteLength) sendBinary(server, value); }
        server.close(1000, "done");
 3. Skip the client->upstream direction and the `early[]` buffer entirely. Logs are one-way; the
    client sends nothing.
 4. Close `server` when the body ends or the fetch throws, mirroring :170-191.
Prefer keying this off the locator rather than re-parsing the path in the shell: LocateStream
(edgehost/stream.go:68-71) already returns {node, url, protocol}; add e.g. `"stream": "http"` for the
log kind so the transport decision lives with the path construction.

## Protocol detail that decides whether the bytes are accepted
`3ceecf4` made StreamProtocol accept `binary.k8s.io` and `base64.k8s.io`. Whichever it negotiates
must be honoured by the pump:
 - binary.k8s.io  -> raw bytes as binary frames. `sendBinary` already does this.
 - base64.k8s.io  -> base64-encode each chunk and send as a TEXT frame. `sendBinary` would be wrong.
 - v4/v5.channel.k8s.io -> would need a leading 0x00 channel byte per frame. The e2e spec requests
   binary.k8s.io, so this is optional, but if StreamProtocol can select a channel protocol for a log
   request it should either add the byte or decline those for logs.
The locator echoes the negotiated protocol to the client via streamUpgrade (index.ts:63-67), so the
client and the pump must agree; a mismatch here reproduces as an empty or garbled payload rather than
an error.

## Verification
Run the single spec: `[sig-node] Pods should support retrieving logs from the container over
websockets`. It ran in ~6 s standalone, so iteration is cheap. Criteria, in order:
 1. `bridge: fetch timed out` stays 0 (the routing fix must not regress).
 2. The `stream in ...` and `stream-msg ...` lines at index.ts:150 and :122 show non-zero byte counts.
    Zero `stream-msg` lines means the pump is not running; lines with byteLength 0 means framing.
 3. The spec's assertion sees the container's output.
Also re-run the three exec/attach/portforward-adjacent specs to confirm the websocket branch was not
disturbed, since the log branch shares the block.

## Do not re-litigate (settled by measurement earlier in this work)
 - The turn machinery is clear: 119,809 off-turn inline fetches settled, `turn=true` on the one
   failure, and that failure was routing. window_js.go:327-330's unconditional Store(previous) is
   correct-in-itself hygiene with nothing observed behind it; land it alone or not at all.
 - Contention is out: inflight=1 on the failing fetch.
 - The ~198 timeouts in a full suite run were load-dependent and largely incidental; 5 of the same 6
   specs pass in isolation at identical procs=4.
 - The protobuf strip is permanently rejected -- real kubectl cannot write to a JSON-only server
   because ContentType is not negotiated the way Accept is. plans/remaining.md records why.
