/** A single parsed label selector requirement. */
export interface LabelRequirement {
  key: string;
  operator: "In" | "NotIn" | "Exists" | "DoesNotExist" | "Equals" | "NotEquals";
  values: string[];
}

/**
 * Split a label selector string on top-level commas, ignoring commas nested
 * inside `in (...)` / `notin (...)` value lists.
 */
function splitRequirements(selector: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let current = "";
  for (const ch of selector) {
    if (ch === "(") depth++;
    if (ch === ")") depth--;
    if (ch === "," && depth === 0) {
      parts.push(current);
      current = "";
    } else {
      current += ch;
    }
  }
  if (current.trim()) parts.push(current);
  return parts;
}

function splitValues(raw: string): string[] {
  return raw
    .split(",")
    .map((v) => v.trim())
    .filter((v) => v.length > 0);
}

const KEY = "[A-Za-z0-9_./-]+";
const IN_RE = new RegExp(`^(${KEY})\\s+in\\s+\\(([^)]*)\\)$`);
const NOTIN_RE = new RegExp(`^(${KEY})\\s+notin\\s+\\(([^)]*)\\)$`);
const NOT_EQUALS_RE = new RegExp(`^(${KEY})\\s*!=\\s*(.+)$`);
const EQUALS_RE = new RegExp(`^(${KEY})\\s*==?\\s*(.+)$`);
const NOT_EXISTS_RE = new RegExp(`^!\\s*(${KEY})$`);
const EXISTS_RE = new RegExp(`^(${KEY})$`);

function parseRequirement(req: string): LabelRequirement | null {
  let m = req.match(IN_RE);
  if (m) return { key: m[1], operator: "In", values: splitValues(m[2]) };
  m = req.match(NOTIN_RE);
  if (m) return { key: m[1], operator: "NotIn", values: splitValues(m[2]) };
  m = req.match(NOT_EQUALS_RE);
  if (m) return { key: m[1], operator: "NotEquals", values: [m[2].trim()] };
  m = req.match(EQUALS_RE);
  if (m) return { key: m[1], operator: "Equals", values: [m[2].trim()] };
  m = req.match(NOT_EXISTS_RE);
  if (m) return { key: m[1], operator: "DoesNotExist", values: [] };
  m = req.match(EXISTS_RE);
  if (m) return { key: m[1], operator: "Exists", values: [] };
  return null;
}

/**
 * Parse a Kubernetes label selector string (e.g. "env=prod,tier in (web,api)")
 * into a list of requirements, ANDed together. Supports the standard
 * equality-, set-, and existence-based forms: key=value, key==value,
 * key!=value, key in (v1,v2), key notin (v1,v2), key, !key.
 */
export function parseLabelSelector(selector: string): LabelRequirement[] {
  if (!selector.trim()) return [];
  const requirements: LabelRequirement[] = [];
  for (const part of splitRequirements(selector)) {
    const trimmed = part.trim();
    if (!trimmed) continue;
    const req = parseRequirement(trimmed);
    // An unparseable requirement can never be satisfied, so the selector as a
    // whole should filter everything out rather than silently ignoring it.
    requirements.push(req ?? { key: "", operator: "Exists", values: [] });
  }
  return requirements;
}

/** Check whether a label set satisfies every requirement (AND semantics). */
export function matchesLabelSelector(
  labels: Record<string, string> | undefined,
  requirements: LabelRequirement[],
): boolean {
  const l = labels || {};
  return requirements.every((req) => {
    const value = l[req.key];
    switch (req.operator) {
      case "Exists":
        return value !== undefined;
      case "DoesNotExist":
        return value === undefined;
      case "Equals":
        return value === req.values[0];
      case "NotEquals":
        return value !== req.values[0];
      case "In":
        return value !== undefined && req.values.includes(value);
      case "NotIn":
        return value === undefined || !req.values.includes(value);
    }
  });
}
