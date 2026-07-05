/**
 * RFC 7396 JSON Merge Patch. A `null` value in the patch deletes the key;
 * any other value replaces it. Nested objects are merged recursively,
 * arrays and scalars are replaced wholesale (per the RFC).
 */
export function applyMergePatch(target: any, patch: any): any {
  if (patch === null || typeof patch !== "object" || Array.isArray(patch)) {
    return patch;
  }
  const result =
    typeof target === "object" && target !== null && !Array.isArray(target) ? { ...target } : {};
  for (const key of Object.keys(patch)) {
    if (patch[key] === null) {
      delete result[key];
    } else {
      result[key] = applyMergePatch(result[key], patch[key]);
    }
  }
  return result;
}

/** Resolve an RFC 6901 JSON Pointer token (~1 -> /, ~0 -> ~). */
function unescapeToken(token: string): string {
  return token.replace(/~1/g, "/").replace(/~0/g, "~");
}

function splitPointer(pointer: string): string[] {
  if (pointer === "") return [];
  if (!pointer.startsWith("/")) throw new Error(`invalid JSON pointer "${pointer}"`);
  return pointer.slice(1).split("/").map(unescapeToken);
}

function getByPointer(doc: any, tokens: string[]): any {
  let cur = doc;
  for (const t of tokens) {
    if (cur === undefined || cur === null) return undefined;
    cur = Array.isArray(cur) ? cur[t === "-" ? cur.length - 1 : Number(t)] : cur[t];
  }
  return cur;
}

function setByPointer(doc: any, tokens: string[], value: any): void {
  let cur = doc;
  for (let i = 0; i < tokens.length - 1; i++) {
    cur = Array.isArray(cur) ? cur[Number(tokens[i])] : cur[tokens[i]];
  }
  const last = tokens[tokens.length - 1];
  if (Array.isArray(cur)) {
    if (last === "-") cur.push(value);
    else cur.splice(Number(last), 0, value);
  } else {
    cur[last] = value;
  }
}

function removeByPointer(doc: any, tokens: string[]): void {
  let cur = doc;
  for (let i = 0; i < tokens.length - 1; i++) {
    cur = Array.isArray(cur) ? cur[Number(tokens[i])] : cur[tokens[i]];
  }
  const last = tokens[tokens.length - 1];
  if (Array.isArray(cur)) cur.splice(Number(last), 1);
  else delete cur[last];
}

/**
 * RFC 6902 JSON Patch. Supports add/remove/replace/move/copy/test, applied
 * in order against a deep clone of `target`. Throws on a failed "test" op
 * or an out-of-range path, matching RFC 6902's all-or-nothing semantics.
 */
export function applyJSONPatch(
  target: any,
  ops: Array<{ op: string; path: string; value?: any; from?: string }>,
): any {
  let doc = JSON.parse(JSON.stringify(target));
  for (const step of ops) {
    const tokens = splitPointer(step.path);
    switch (step.op) {
      case "add":
      case "replace":
        if (tokens.length === 0) {
          doc = step.value;
        } else if (step.op === "replace") {
          setByPointer(doc, [...tokens.slice(0, -1), tokens[tokens.length - 1]], step.value);
        } else {
          setByPointer(doc, tokens, step.value);
        }
        break;
      case "remove":
        removeByPointer(doc, tokens);
        break;
      case "move": {
        const fromTokens = splitPointer(step.from || "");
        const value = getByPointer(doc, fromTokens);
        removeByPointer(doc, fromTokens);
        setByPointer(doc, tokens, value);
        break;
      }
      case "copy": {
        const fromTokens = splitPointer(step.from || "");
        const value = JSON.parse(JSON.stringify(getByPointer(doc, fromTokens)));
        setByPointer(doc, tokens, value);
        break;
      }
      case "test": {
        const actual = getByPointer(doc, tokens);
        if (JSON.stringify(actual) !== JSON.stringify(step.value)) {
          throw new Error(`test op failed at "${step.path}"`);
        }
        break;
      }
      default:
        throw new Error(`unsupported JSON Patch op "${step.op}"`);
    }
  }
  return doc;
}
