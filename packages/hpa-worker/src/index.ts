export { HorizontalPodAutoscaler } from "../../control-plane-worker/src/hpactrl.ts";

export default {
  fetch(): Response {
    return new Response(null, { status: 404 });
  },
};
