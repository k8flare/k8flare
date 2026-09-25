const origin = "https://k8flare.kooffice.workers.dev";

export default {
  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const dest = new URL(url.pathname + url.search, origin);
    if (url.search && /\/pods\/[^/]+\/(exec|attach|portforward)(?:\/|$)/.test(url.pathname)) {
      dest.pathname = `${url.pathname.replace(/\/$/, "")}/_q/${encodeURIComponent(url.search.slice(1))}`;
    }
    const headers = new Headers(request.headers);
    headers.delete("Host");
    if (url.search) headers.set("X-Stream-Query", url.search.slice(1));
    if ((request.headers.get("Upgrade") || "").toLowerCase() === "websocket") {
      return fetch(new Request(dest.toString(), request), { headers });
    }
    headers.set("X-Forwarded-Host", url.host);
    headers.set("X-Forwarded-Proto", url.protocol.replace(":", ""));
    const host = url.hostname.toLowerCase();
    if (host === "api.k8flare.com" || host === "k8flare.com") {
      headers.delete("Host");
    } else if (host.endsWith(".k8flare.com")) {
      headers.set("Host", host);
    }
    return fetch(new Request(dest.toString(), { method: request.method, headers, body: request.body, redirect: "manual" }));
  },
};
