export { AttachDetach } from "../../control-plane-worker/src/attachdetach.ts";

export default {
  fetch(): Response {
    return new Response(null, { status: 404 });
  },
};
