// Standalone worker used only as a service-binding target so the parent
// worker (s2-loader-parent) can test forwarding a service binding into a
// Worker-Loader-loaded worker's env (S2 item 3b).
export default {
  async fetch(req: Request): Promise<Response> {
    const url = new URL(req.url);
    return Response.json({ ok: true, from: "child-service", path: url.pathname });
  },
};
