const bases = process.argv.slice(2);
for (const base of bases) {
  let list;
  try {
    list = await (await fetch(`${base}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
  } catch (err) {
    console.log(`${base}: no inspector (${err.message})`);
    continue;
  }
  for (const target of list) {
    console.log(`${base} target id=${target.id} title=${target.title} url=${target.url}`);
    if (!target.webSocketDebuggerUrl) continue;
    const ws = new WebSocket(target.webSocketDebuggerUrl);
    try {
      await new Promise((resolve, reject) => {
        ws.onopen = resolve;
        ws.onerror = reject;
        setTimeout(() => reject(new Error("connect timeout")), 10000);
      });
    } catch (err) {
      console.log(`  connect failed: ${err.message}`);
      continue;
    }
    let id = 0;
    const send = (method, params = {}) => ws.send(JSON.stringify({ id: ++id, method, params }));
    const paused = new Promise((resolve) => {
      ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        if (msg.method === "Debugger.paused") resolve(msg.params);
      };
    });
    send("Debugger.enable");
    send("Debugger.pause");
    const params = await Promise.race([paused, new Promise((resolve) => setTimeout(() => resolve(null), 20000))]);
    if (!params) {
      console.log("  did not pause within 20s");
    } else {
      console.log(`  paused reason=${params.reason}`);
      for (const frame of params.callFrames) {
        console.log(`    ${frame.functionName || "(anonymous)"} ${frame.url} ${frame.location.lineNumber}:${frame.location.columnNumber}`);
      }
      send("Debugger.resume");
    }
    ws.close();
  }
}
process.exit(0);
