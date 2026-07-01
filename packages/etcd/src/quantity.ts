/** Parse a Kubernetes CPU quantity string into millicores. "500m" -> 500, "1" -> 1000, "0.5" -> 500. */
export function parseCPU(q: string | undefined): number {
  if (!q) return 0;
  if (q.endsWith("m")) return parseInt(q.slice(0, -1), 10) || 0;
  return Math.round(parseFloat(q) * 1000) || 0;
}
