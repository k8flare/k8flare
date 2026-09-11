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

  it("lets a field override the placeholder window id", () => {
    const line = formatPumpTrace("observed", "kcm", { w: 12, rv: 747 }, 1757600000000);
    expect(JSON.parse(line.slice("pumptrace ".length))).toMatchObject({ w: 12, t: 1757600000000 });
  });
});
