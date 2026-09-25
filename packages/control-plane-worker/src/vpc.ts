type VPCBinding = {
  connect: (opts: { hostname: string; port: number }) => {
    opened: Promise<unknown>;
    close: () => Promise<void>;
  };
};

export function vpcBinding(env: Env): VPCBinding | null {
  const vpc = (env as Env & { VPC?: VPCBinding }).VPC;
  if (!vpc || typeof vpc.connect !== "function") return null;
  return vpc;
}

export async function vpcConnectAvailable(env: Env): Promise<boolean> {
  return vpcBinding(env) != null;
}

export async function vpcDial(env: Env, hostname: string, port: number): Promise<boolean> {
  const vpc = vpcBinding(env);
  if (!vpc) return false;
  try {
    const socket = vpc.connect({ hostname, port });
    await socket.opened;
    await socket.close();
    return true;
  } catch {
    return false;
  }
}

export async function vpcFetch(env: Env, request: Request): Promise<Response | null> {
  const url = new URL(request.url);
  const parts = url.pathname.split("/").filter(Boolean);
  if (parts[0] !== "dial" || parts.length < 4) return null;
  const host = parts[2];
  const port = Number(parts[3]);
  if (!host || !Number.isFinite(port)) return null;
  if (!(await vpcDial(env, host, port))) return null;
  return new Response("vpc connect opened", { status: 501 });
}
