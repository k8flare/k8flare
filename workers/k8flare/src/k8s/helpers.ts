/** Decode a kine value (base64 string or Uint8Array) to a UTF-8 string. */
export function decodeKineValue(v: string | ArrayLike<number>): string {
  if (typeof v === "string") return atob(v);
  return new TextDecoder().decode(new Uint8Array(v as ArrayLike<number>));
}
