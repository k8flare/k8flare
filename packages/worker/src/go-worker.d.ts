declare module "#go-worker" {
  const goWorker: {
    fetch(req: Request, env: any, ctx: ExecutionContext): Promise<Response>;
  };
  export default goWorker;
}
