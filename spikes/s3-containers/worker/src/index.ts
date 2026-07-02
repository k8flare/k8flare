import { Container, getContainer } from "@cloudflare/containers";

// s3-spike: default onActivityExpired (inherited, calls this.stop()).
export class EchoContainer extends Container {
  defaultPort = 8080;
  sleepAfter = "20s";

  override onStart() {
    console.log(`[EchoContainer] onStart id=${this.ctx.id}`);
  }
  override onStop(params: { exitCode: number; reason: string }) {
    console.log(`[EchoContainer] onStop exitCode=${params.exitCode} reason=${params.reason}`);
  }
  override onError(error: unknown) {
    console.log(`[EchoContainer] onError ${error}`);
    throw error;
  }
}

// s3-spike: onActivityExpired overridden WITHOUT calling stop()/destroy().
// Per docs + dist/lib/container.d.ts comment ("By default, this method calls
// this.stop()"), the expectation is that overriding it and not calling stop()
// keeps the container running past sleepAfter indefinitely.
export class EchoContainerNoStop extends Container {
  defaultPort = 8080;
  sleepAfter = "20s";
  expiredCount = 0;

  override onStart() {
    console.log(`[EchoContainerNoStop] onStart id=${this.ctx.id}`);
  }
  override onStop(params: { exitCode: number; reason: string }) {
    console.log(`[EchoContainerNoStop] onStop exitCode=${params.exitCode} reason=${params.reason}`);
  }
  override async onActivityExpired() {
    this.expiredCount++;
    console.log(`[EchoContainerNoStop] onActivityExpired fired (count=${this.expiredCount}), deliberately NOT calling stop()`);
    // no this.stop() / this.destroy() call on purpose
  }
}

// s3-spike: enableInternet=false, no allowedHosts. Used to check whether the
// egress-restriction proxy actually engages for a container that has no
// outbound handlers/allow-deny lists configured (EchoContainer/NoStop above
// have neither set, so per dist/lib/container.js's constructor,
// `usingInterception` stays false for them -- this class isolates whether
// that matters).
export class EchoContainerRestricted extends Container {
  defaultPort = 8080;
  sleepAfter = "20s";
  enableInternet = false;
}

export default {
  async fetch(
    request: Request,
    env: {
      ECHO_CONTAINER: DurableObjectNamespace<EchoContainer>;
      ECHO_CONTAINER_NOSTOP: DurableObjectNamespace<EchoContainerNoStop>;
      ECHO_CONTAINER_RESTRICTED: DurableObjectNamespace<EchoContainerRestricted>;
    },
  ): Promise<Response> {
    const url = new URL(request.url);

    if (url.pathname === "/state") {
      const which = url.searchParams.get("which");
      const stub = which === "nostop" ? getContainer(env.ECHO_CONTAINER_NOSTOP) : which === "restricted" ? getContainer(env.ECHO_CONTAINER_RESTRICTED) : getContainer(env.ECHO_CONTAINER);
      const state = await stub.getState();
      return Response.json({ which: which ?? "default", state, at: new Date().toISOString() });
    }

    if (url.pathname.startsWith("/nostop")) {
      const stub = getContainer(env.ECHO_CONTAINER_NOSTOP);
      const inner = new URL(request.url);
      inner.pathname = inner.pathname.replace(/^\/nostop/, "") || "/";
      return stub.fetch(new Request(inner, request));
    }

    if (url.pathname.startsWith("/restricted")) {
      const stub = getContainer(env.ECHO_CONTAINER_RESTRICTED);
      const inner = new URL(request.url);
      inner.pathname = inner.pathname.replace(/^\/restricted/, "") || "/";
      return stub.fetch(new Request(inner, request));
    }

    // default -> EchoContainer (default onActivityExpired behavior)
    const stub = getContainer(env.ECHO_CONTAINER);
    return stub.fetch(request);
  },
};
