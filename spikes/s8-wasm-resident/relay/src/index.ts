// S8 spike (d): minimal TS worker that relays every request to the Go WASM
// worker (stock variant) via a service binding, to check whether an open
// streaming response still stays open when it crosses a service-binding hop
// instead of being fetched directly.
export interface Env {
  GOWORKER: Fetcher;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    return env.GOWORKER.fetch(request);
  },
};
