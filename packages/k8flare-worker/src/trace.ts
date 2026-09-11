export interface TraceEnv {
  PUMP_TRACE?: string;
}

export function pumpTraceEnabled(env: TraceEnv): boolean {
  return env.PUMP_TRACE === "1";
}

export function formatPumpTrace(
  boundary: string,
  component: string,
  fields: Record<string, string | number>,
  at: number,
): string {
  return `pumptrace ${JSON.stringify({ b: boundary, c: component, w: 0, t: at, ...fields })}`;
}

export function pumpTrace(
  env: TraceEnv,
  boundary: string,
  component: string,
  fields: Record<string, string | number>,
): void {
  if (!pumpTraceEnabled(env)) return;
  console.log(formatPumpTrace(boundary, component, fields, Date.now()));
}
