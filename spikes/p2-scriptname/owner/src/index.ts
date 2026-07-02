export class Ping {
  constructor(_state: DurableObjectState, _env: unknown) {}

  async fetch(_req: Request): Promise<Response> {
    return new Response("pong-from-owner-do");
  }
}

export default {
  async fetch(_req: Request): Promise<Response> {
    return new Response("owner-worker-direct");
  },
};
