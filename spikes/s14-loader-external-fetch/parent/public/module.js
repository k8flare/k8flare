export default {
  async fetch(req) {
    return Response.json({ ok: true, source: "fetched-at-runtime", ts: Date.now() });
  }
};
