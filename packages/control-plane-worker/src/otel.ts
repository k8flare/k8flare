const NANOS_PER_MS = 1_000_000;

type AttrValue = string | number | boolean;

interface OTLPAttr {
  key: string;
  value: { stringValue: string } | { intValue: string } | { boolValue: boolean };
}

interface OTLPSpan {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  name: string;
  kind: number;
  startTimeUnixNano: string;
  endTimeUnixNano: string;
  attributes: OTLPAttr[];
  status?: { code: number; message?: string };
}

export interface SpanContext {
  traceId: string;
  spanId: string;
}

function hex(bytes: number): string {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
}

function attrs(from: Record<string, AttrValue> | undefined): OTLPAttr[] {
  if (!from) return [];
  return Object.entries(from).map(([key, v]) => {
    if (typeof v === "number") return { key, value: { intValue: String(Math.round(v)) } };
    if (typeof v === "boolean") return { key, value: { boolValue: v } };
    return { key, value: { stringValue: v } };
  });
}

export class Span {
  readonly context: SpanContext;
  private readonly startMs = Date.now();
  private ended = false;

  constructor(
    private readonly trace: Trace,
    private readonly name: string,
    parent: SpanContext | undefined,
    private readonly initial: Record<string, AttrValue> | undefined,
    private readonly parentSpanId: string | undefined = parent?.spanId,
  ) {
    this.context = { traceId: trace.traceId, spanId: hex(8) };
  }

  child(name: string, extra?: Record<string, AttrValue>): Span {
    return this.trace.span(name, this.context, extra);
  }

  end(extra?: Record<string, AttrValue>, error?: unknown): void {
    if (this.ended) return;
    this.ended = true;
    const span: OTLPSpan = {
      traceId: this.context.traceId,
      spanId: this.context.spanId,
      parentSpanId: this.parentSpanId,
      name: this.name,
      kind: 1,
      startTimeUnixNano: String(this.startMs * NANOS_PER_MS),
      endTimeUnixNano: String(Date.now() * NANOS_PER_MS),
      attributes: attrs({ ...this.initial, ...extra }),
    };
    if (error !== undefined) {
      span.status = { code: 2, message: String(error instanceof Error ? `${error.name}: ${error.message}` : error) };
    }
    this.trace.collect(span);
  }

  async measure<T>(fn: () => Promise<T>): Promise<T> {
    try {
      const out = await fn();
      this.end();
      return out;
    } catch (err) {
      this.end(undefined, err);
      throw err;
    }
  }
}

export class Trace {
  readonly traceId: string;
  private readonly spans: OTLPSpan[] = [];

  private constructor(private readonly endpoint: string | undefined, private readonly service: string, traceId?: string) {
    this.traceId = traceId ?? hex(16);
  }

  static start(env: unknown, service: string, traceparent?: string): Trace {
    const endpoint = (env as { OTEL_ENDPOINT?: string } | undefined)?.OTEL_ENDPOINT;
    const parsed = parseTraceparent(traceparent);
    return new Trace(endpoint, service, parsed?.traceId);
  }

  get enabled(): boolean {
    return Boolean(this.endpoint);
  }

  span(name: string, parent?: SpanContext, extra?: Record<string, AttrValue>): Span {
    return new Span(this, name, parent, extra);
  }

  root(name: string, traceparent?: string, extra?: Record<string, AttrValue>): Span {
    const parsed = parseTraceparent(traceparent);
    return new Span(this, name, undefined, extra, parsed?.spanId);
  }

  collect(span: OTLPSpan): void {
    if (this.endpoint) this.spans.push(span);
  }

  traceparent(ctx: SpanContext): string {
    return `00-${ctx.traceId}-${ctx.spanId}-01`;
  }

  async flush(): Promise<void> {
    if (!this.endpoint || this.spans.length === 0) return;
    const body = JSON.stringify({
      resourceSpans: [{
        resource: { attributes: attrs({ "service.name": this.service }) },
        scopeSpans: [{ scope: { name: "k8flare" }, spans: this.spans }],
      }],
    });
    this.spans.length = 0;
    try {
      await fetch(`${this.endpoint}/v1/traces`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
        signal: AbortSignal.timeout(5_000),
      });
    } catch (err) {
      console.log(`otel: export failed: ${String(err)}`);
    }
  }
}

function parseTraceparent(tp: string | undefined): SpanContext | null {
  if (!tp) return null;
  const parts = tp.split("-");
  if (parts.length !== 4 || parts[1].length !== 32 || parts[2].length !== 16) return null;
  return { traceId: parts[1], spanId: parts[2] };
}
