export function clusterName(env: Env): string {
  return env.CLUSTER_UID || "default";
}

export function clusterStub(env: Env) {
  return env.CLUSTER.get(env.CLUSTER.idFromName(clusterName(env)));
}

export function tunnelName(env: Env, node: string): string {
  return env.CLUSTER_UID ? `${env.CLUSTER_UID}:${node}` : node;
}
