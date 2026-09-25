declare module "@cloudflare/containers" {
  export class Container<E = unknown> {
    readonly ctx: DurableObjectState;
    readonly env: E;
    [Rpc.__DURABLE_OBJECT_BRAND]: never;
    defaultPort: number;
    sleepAfter: string;
    destroy(): Promise<void>;
    getState(): Promise<{ status: string }>;
    startAndWaitForPorts(opts: {
      ports: number;
      startOptions?: { envVars?: Record<string, string>; enableInternet?: boolean };
      cancellationOptions?: { instanceGetTimeoutMS?: number; portReadyTimeoutMS?: number };
    }): Promise<void>;
    containerFetch(request: Request, port: number): Promise<Response>;
  }
}
