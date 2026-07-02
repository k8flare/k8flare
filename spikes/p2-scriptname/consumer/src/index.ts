// Phase 2 / Step 0 spike: does a `script_name` DO binding let this Worker
// reach a Durable Object class defined (and migrated) in a *different*
// Worker (see ../owner), under local `wrangler dev -c consumer -c owner`?
// Unverified by any S1-S8 platform-verification spike; blocks the Phase 2
// repo split (gateway/apiserver/runtime all reach the storage Worker's
// Cluster DO this way). Result: confirmed working — see repro in
// spikes/p2-scriptname/owner/wrangler.jsonc and the Step 0 commit message.
interface Env {
  PING: DurableObjectNamespace;
}

export default {
  async fetch(_req: Request, env: Env): Promise<Response> {
    const id = env.PING.idFromName("default");
    const stub = env.PING.get(id);
    const resp = await stub.fetch("http://do.internal/");
    const text = await resp.text();
    return new Response("consumer-got: " + text);
  },
};
