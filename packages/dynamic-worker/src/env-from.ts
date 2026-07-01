import { dwStub, decodeKineValue } from "./helpers.ts";
import type { EnvFromRef } from "./types.ts";

/**
 * Resolve envFrom references (Secrets and ConfigMaps) from kine storage.
 *
 * Secret values are base64-decoded; ConfigMap values are used as-is.
 * All refs are resolved in parallel via Promise.all.
 */
export async function resolveEnvFrom(
  env: any,
  namespace: string,
  envFromList: EnvFromRef[] | undefined,
): Promise<Record<string, string>> {
  if (!envFromList || envFromList.length === 0) return {};

  const fetchPromises = envFromList.map(async (ref) => {
    let resourceKey: string;
    if (ref.secretRef) {
      resourceKey = `/registry/secrets/${namespace}/${ref.secretRef.name}`;
    } else if (ref.configMapRef) {
      resourceKey = `/registry/configmaps/${namespace}/${ref.configMapRef.name}`;
    } else {
      return {};
    }

    const resp = await dwStub(env).fetch(
      new Request("http://do.internal/key" + resourceKey),
    );
    if (!resp.ok) {
      if (ref.optional) return {};
      throw new Error(`envFrom: "${resourceKey}" not found`);
    }
    const body: any = await resp.json();
    if (!body.kv?.value) {
      if (ref.optional) return {};
      throw new Error(`envFrom: "${resourceKey}" empty`);
    }
    const resource = JSON.parse(decodeKineValue(body.kv.value));
    const prefix = ref.secretRef?.prefix || ref.configMapRef?.prefix || ref.prefix || "";
    const result: Record<string, string> = {};
    for (const [k, v] of Object.entries(resource.data || {})) {
      result[prefix + k] = ref.secretRef ? atob(v as string) : (v as string);
    }
    return result;
  });

  const results = await Promise.all(fetchPromises);
  return Object.assign({}, ...results);
}
