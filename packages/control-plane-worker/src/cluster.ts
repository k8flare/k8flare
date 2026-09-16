export async function clusterHasNodes(env: Env): Promise<boolean> {
  const store = env.CLUSTER.get(env.CLUSTER.idFromName("default"));
  const resp = await store.fetch("http://cluster.internal/list?prefix=%2Fregistry%2Fnodes%2F&limit=1");
  if (!resp.ok) return true;
  const body = (await resp.json()) as { kvs?: unknown[] };
  return (body.kvs ?? []).length > 0;
}
