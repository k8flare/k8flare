import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { unstable_readConfig } from "wrangler";

const root = resolve(dirname(new URL(import.meta.url).pathname), "..");
const configPath = resolve(root, process.argv[2] ?? "wrangler.jsonc");
const outPath = resolve(root, "packages/control-plane-worker/src/podkubelet/images.generated.ts");
const config = unstable_readConfig({ config: configPath });

const images = {};
for (const app of config.containers ?? []) {
  for (const [name, image] of Object.entries(app.images ?? {})) {
    if (typeof image !== "object" || !image.dockerfile) continue;
    const dockerfile = resolve(dirname(configPath), image.dockerfile);
    const context = image.build_context ? resolve(dirname(configPath), image.build_context) : dirname(dockerfile);
    const buildArgs = Object.entries(image.build_vars ?? {}).flatMap(([k, v]) => ["--build-arg", `${k}=${v}`]);
    const id = execFileSync("docker", ["build", "-q", "-f", dockerfile, ...buildArgs, context], { encoding: "utf8" }).trim();
    const [inspected] = JSON.parse(execFileSync("docker", ["image", "inspect", id], { encoding: "utf8" }));
    const cfg = inspected.Config ?? {};
    images[name] = {
      entrypoint: cfg.Entrypoint ?? [],
      cmd: cfg.Cmd ?? [],
      workingDir: cfg.WorkingDir ?? "",
      user: cfg.User ?? "",
      env: cfg.Env ?? [],
    };
  }
}

const sorted = Object.fromEntries(Object.entries(images).sort(([a], [b]) => a.localeCompare(b)));
mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, `import type { ImageMeta } from "./spec.ts";\n\nexport const declaredImages: Record<string, ImageMeta> = ${JSON.stringify(sorted, null, 2)};\n`);
console.log(`wrote ${Object.keys(sorted).length} image(s) to ${outPath}`);
