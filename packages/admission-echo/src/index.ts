export default {
  async fetch(request: Request): Promise<Response> {
    const review = (await request.json()) as {
      apiVersion?: string;
      request?: { uid?: string; object?: { metadata?: { annotations?: Record<string, string> } } };
    };
    const admit = review.request?.object?.metadata?.annotations?.["k8flare.io/admit"] ?? "";
    const uid = review.request?.uid ?? "";
    if (admit === "deny") {
      return Response.json({
        apiVersion: review.apiVersion ?? "admission.k8s.io/v1",
        kind: "AdmissionReview",
        response: { uid, allowed: false, status: { message: "denied by admission-echo" } },
      });
    }
    const response: Record<string, unknown> = { uid, allowed: true };
    if (admit === "mutate") {
      response.patchType = "JSONPatch";
      response.patch = btoa(JSON.stringify([{ op: "add", path: "/metadata/annotations/k8flare.io~1mutated", value: "true" }]));
    }
    return Response.json({
      apiVersion: review.apiVersion ?? "admission.k8s.io/v1",
      kind: "AdmissionReview",
      response,
    });
  },
};
