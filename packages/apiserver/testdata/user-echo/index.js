const name = "user-echo";
const groupVersion = "user-echo.k8flare.dev/v1";

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === `/apis/${groupVersion}`) return apiResources();
    if (url.pathname.startsWith("/apis")) return new Response("not found", { status: 404 });
    const body = await request.json();
    if (body.kind === "AdmissionReview") return deny(body);
    if (body.kind === "ConversionReview") return convert(body);
    return reconcile(body, env);
  },
};

function apiResources() {
  return Response.json({
    kind: "APIResourceList",
    apiVersion: "v1",
    groupVersion,
    resources: [{ name: "user-echoes", singularName: "user-echo", namespaced: true, kind: "UserEcho", verbs: ["get"] }],
  });
}

function deny(review) {
  return Response.json({
    apiVersion: review.apiVersion,
    kind: "AdmissionReview",
    response: { uid: review.request.uid, allowed: false, status: { message: `denied by ${name}` } },
  });
}

function convert(review) {
  const convertedObjects = review.request.objects.map((obj) => ({
    ...obj,
    apiVersion: review.request.desiredAPIVersion,
    metadata: { ...obj.metadata, annotations: { ...obj.metadata.annotations, "k8flare.io/converted-by": name } },
  }));
  return Response.json({
    apiVersion: review.apiVersion,
    kind: "ConversionReview",
    response: { uid: review.request.uid, convertedObjects, result: { status: "Success" } },
  });
}

async function reconcile(change, env) {
  const [, , group, plural, namespace, objectName] = change.key.split("/");
  const url = `https://apiserver.internal/apis/${group}/v1/namespaces/${namespace}/${plural}/${objectName}`;
  const headers = { Authorization: `Bearer ${env.API_TOKEN}`, "Content-Type": "application/json" };
  const got = await env.APISERVER.fetch(url, { headers });
  if (got.status === 404) return new Response("ok");
  if (!got.ok) return new Response("get", { status: 502 });
  const obj = await got.json();
  if (obj.metadata.annotations?.["k8flare.io/reconciled-by"] === name) return new Response("ok");
  obj.metadata.annotations = { ...obj.metadata.annotations, "k8flare.io/reconciled-by": name };
  const put = await env.APISERVER.fetch(url, { method: "PUT", headers, body: JSON.stringify(obj) });
  return put.ok ? new Response("ok") : new Response("put", { status: 502 });
}
