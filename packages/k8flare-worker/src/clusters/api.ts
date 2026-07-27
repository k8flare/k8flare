import type { Env } from "../env.ts";
import { authorizeAdmin } from "./adminauth.ts";
import { buildKubeconfig } from "./kubeconfig.ts";
import { clusterSecrets } from "./tokens.ts";

// What is left of the cluster management API after P3 of
// docs/cluster-api-design.md: cluster issuance, token rotation and teardown
// are Kubernetes objects now (`kubectl apply -f cluster.yaml`, reconciled by
// pkg/controllers/clusterop), so the only route that survives is the
// BOOTSTRAP one -- an operator with a fresh deployment has no kubectl yet,
// and cannot fetch a Cluster's Secret without one:
//
//   GET /clusters/default/kubeconfig  -> kubeconfig YAML for the management cluster
//
// Every other /clusters route answers 410 Gone with the kubectl equivalent.

const BOOTSTRAP_PATH = "/clusters/default/kubeconfig";

const RETIRED = {
  error: "the cluster management API is retired; manage clusters with kubectl",
  create: 'kubectl apply -f cluster.yaml  (apiVersion: k8flare.com/v1alpha1, kind: Cluster)',
  list: "kubectl get clusters",
  credentials: "kubectl get secret cluster-<name> -n k8flare-system",
  rotate: 'kubectl annotate cluster <name> k8flare.com/rotate-token="$(date +%s)"',
  delete: "kubectl delete cluster <name>",
  bootstrap: `GET ${BOOTSTRAP_PATH} (this deployment's own kubeconfig)`,
};

export async function handleClustersAPI(req: Request, env: Env): Promise<Response> {
  const url = new URL(req.url);
  if (url.pathname !== BOOTSTRAP_PATH || req.method !== "GET") {
    return Response.json(RETIRED, { status: 410 });
  }

  if (!(await authorizeAdmin(req, env))) {
    return Response.json(
      { error: "a valid default-cluster token (K3S_TOKEN) or Cloudflare Access is required" },
      { status: 403 },
    );
  }

  // clusterSecrets, not the raw vault: the management cluster deliberately
  // has NO vault entry (minting one silently invalidates K3S_TOKEN -- see
  // internalapi.ts's handleVault), so its credential is whatever the door
  // actually accepts, which is the K3S_TOKEN secret or, on a deployment
  // that sets no secrets, the dev fallback token.
  const secrets = await clusterSecrets(env, "default");
  if (secrets.length === 0) {
    return Response.json({ error: "the default cluster has no token" }, { status: 409 });
  }
  return new Response(buildKubeconfig(url.origin, "default", secrets[0]), {
    headers: { "Content-Type": "application/yaml" },
  });
}
