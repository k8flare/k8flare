import { describe, expect, it, vi } from "vite-plus/test";
import { formatPumpTrace, pumpTrace, pumpTraceEnabled } from "./trace.ts";

describe("pump-window tracing", () => {
  it("is off unless the operator asked for it", () => {
    for (const env of [{}, { PUMP_TRACE: "" }, { PUMP_TRACE: "0" }, { PUMP_TRACE: "true" }]) {
      expect(pumpTraceEnabled(env)).toBe(false);
    }
    expect(pumpTraceEnabled({ PUMP_TRACE: "1" })).toBe(true);
  });

  it("writes nothing at all when it is off", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    try {
      pumpTrace({}, "commit", "storage", { o: "k", rv: 7 });
      expect(log).not.toHaveBeenCalled();
    } finally {
      log.mockRestore();
    }
  });

  it("emits one searchable JSON line carrying the boundary and component", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    try {
      pumpTrace({ PUMP_TRACE: "1" }, "commit", "storage", { o: "/registry/pods/a", rv: 747 });
      expect(log).toHaveBeenCalledTimes(1);
      const line = log.mock.calls[0][0] as string;
      expect(line.startsWith("pumptrace {")).toBe(true);
      const parsed = JSON.parse(line.slice("pumptrace ".length));
      expect(parsed).toMatchObject({ b: "commit", c: "storage", o: "/registry/pods/a", rv: 747 });
      expect(typeof parsed.t).toBe("number");
    } finally {
      log.mockRestore();
    }
  });

  it("is recognisable after Go's logger has prefixed it with a timestamp", () => {
    // The production relay reads lines a Go dynamic worker emitted, and
    // log.Printf prepends its own date. Anchoring the match at the start
    // of the line silently relayed nothing (S45).
    const emitted = formatPumpTrace(
      "observed.add",
      "kcm",
      { w: 5, o: "ns/p", rv: 7 },
      1757600000000,
    );
    const asGoLogsIt = `2026/09/11 19:58:37 ${emitted}`;
    const at = asGoLogsIt.indexOf("pumptrace {");
    expect(at).toBeGreaterThan(0);
    expect(asGoLogsIt.startsWith("pumptrace ")).toBe(false);
    expect(JSON.parse(asGoLogsIt.slice(at + "pumptrace ".length))).toMatchObject({
      b: "observed.add",
      c: "kcm",
      w: 5,
    });
  });

  it("marks a relayed copy so it is not counted as a second event", () => {
    // Under `wrangler dev` the dynamic worker's output is printed and
    // relayed, so every Go-side line appears twice. Without a marker the
    // duplicate reads as a real duplicate event (S61).
    const emitted = formatPumpTrace("observed.add", "kcm", { w: 5, rv: 7 }, 1757600000000);
    const at = emitted.indexOf("pumptrace {");
    const relayed = `pumptrace {"r":1,${emitted.slice(at + "pumptrace ".length + 1)}`;
    const parsed = JSON.parse(relayed.slice("pumptrace ".length));
    expect(parsed.r).toBe(1);
    expect(parsed).toMatchObject({ b: "observed.add", c: "kcm", w: 5, rv: 7 });
  });

  it("lets a field override the placeholder window id", () => {
    const line = formatPumpTrace("observed", "kcm", { w: 12, rv: 747 }, 1757600000000);
    expect(JSON.parse(line.slice("pumptrace ".length))).toMatchObject({ w: 12, t: 1757600000000 });
  });
});
