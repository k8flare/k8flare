// Shared helpers for gen-k8s-js-mirror.ts and gen-clientgo-lean-mirror.ts:
// resolving a pinned upstream Go module's on-disk directory, copying it
// into a local mirror, and drift-checking a file against a committed
// sha256 pin before an overlay swaps it out.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";

/**
 * Resolves module@version's on-disk module-cache directory via
 * `go mod download -json`, deliberately independent of whatever go.mod's
 * own `replace` for this module currently points at (the caller is often
 * regenerating that replace target's own contents, which would make
 * asking `go list -m` about the replaced module self-referential).
 */
export function resolveModuleDir(module: string, version: string): string {
  const out = execFileSync("go", ["mod", "download", "-json", `${module}@${version}`], {
    encoding: "utf8",
  });
  const dir = (JSON.parse(out) as { Dir?: string }).Dir;
  if (!dir || !fs.existsSync(dir)) {
    throw new Error(`failed to resolve module dir for ${module}@${version}`);
  }
  return dir;
}

/** Reads "<module> <version>" from an upstream-module.txt pin file. */
export function readUpstreamModule(pinFile: string): { module: string; version: string } {
  const [module, version] = fs.readFileSync(pinFile, "utf8").trim().split(/\s+/);
  if (!module || !version) {
    throw new Error(`malformed upstream-module.txt: ${pinFile}`);
  }
  return { module, version };
}

export function sha256File(file: string): string {
  return createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

/**
 * Fails loudly (throws) if file's sha256 no longer matches the pin
 * recorded in pinFile -- an upstream bump changed a file an overlay
 * assumes it has already reviewed, instead of silently re-patching
 * content the overlay doesn't account for (CLAUDE.md rule: k8s bumps
 * must be reviewed, not mechanically pinned).
 */
export function checkPin(label: string, file: string, pinFile: string, guidance: string): void {
  const expected = fs.readFileSync(pinFile, "utf8").trim().split(/\s+/)[0];
  const actual = sha256File(file);
  if (actual !== expected) {
    throw new Error(
      `${label}: upstream ${file} changed since it was last reviewed (expected sha256 ${expected}, got ${actual}). ${guidance} See docs/k8s-version-bump.md.`,
    );
  }
}

/**
 * Copies src (a full module directory) to dst, replacing dst entirely.
 * APFS (macOS): clonefile-backed copy-on-write, ~instant and ~0 extra
 * disk. Linux (CI): --reflink=auto degrades to a plain copy on
 * filesystems without reflink support (e.g. ext4 on GitHub Actions
 * runners) instead of erroring.
 */
export function replaceMirrorDir(src: string, dst: string): void {
  fs.rmSync(dst, { recursive: true, force: true });
  fs.mkdirSync(path.dirname(dst), { recursive: true });
  if (os.platform() === "darwin") {
    execFileSync("cp", ["-Rc", src, dst]);
  } else {
    execFileSync("cp", ["-R", "--reflink=auto", src, dst]);
  }
  execFileSync("chmod", ["-R", "u+w", dst]);
}

export function countFiles(dir: string): number {
  let n = 0;
  for (const entry of fs.readdirSync(dir, { recursive: true, withFileTypes: true })) {
    if (entry.isFile()) n++;
  }
  return n;
}
