import { loadWasmWorker } from "@k8flare/loader-kit";
import { componentToken } from "./componenttoken.ts";

const DEPLOY_TIMEOUT_MS = 120_000;

type ManifestsEnv = Env & { MANIFESTS_R2?: R2Bucket; DISABLE?: string };

async function userManifests(bucket: R2Bucket | undefined): Promise<Record<string, string>> {
  const manifests: Record<string, string> = {};
  if (!bucket) return manifests;
  const listed = await bucket.list();
  for (const object of listed.objects) {
    const body = await bucket.get(object.key);
    if (body) manifests[object.key] = await body.text();
  }
  return manifests;
}

export async function deployAddons(env: ManifestsEnv): Promise<boolean> {
  try {
    const worker = await loadWasmWorker(env.LOADER, env.ASSETS, "addons", {
      APISERVER: env.APISERVER,
      API_TOKEN: await componentToken(env, "addons"),
      DISABLE: env.DISABLE ?? "",
    }, env.APISERVER);
    const resp = await worker.fetch("https://addons.internal/deploy", {
      method: "POST",
      body: JSON.stringify(await userManifests(env.MANIFESTS_R2)),
      signal: AbortSignal.timeout(DEPLOY_TIMEOUT_MS),
    });
    const text = await resp.text();
    if (resp.status !== 200) console.log(`addons: deploy status=${resp.status} ${text.slice(0, 200)}`);
    return resp.status === 200;
  } catch (err) {
    console.log(`addons: deploy failed: ${String(err)}`);
    return false;
  }
}
