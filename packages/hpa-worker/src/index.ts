import { runSync } from "../../control-plane-worker/src/hpactrl.ts";

export { HorizontalPodAutoscaler } from "../../control-plane-worker/src/hpactrl.ts";

export default {
  fetch(): Response {
    return new Response(null, { status: 404 });
  },
  async queue(batch: MessageBatch<unknown>, env: Env): Promise<void> {
    const synced = await runSync(env);
    if (synced) {
      console.log(`hpa: ${Object.entries(synced.objects).map(([k, v]) => `${k}=${v}`).join(" ")} drained=${synced.drained}`);
    }
    batch.ackAll();
  },
};
