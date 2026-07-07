# S15 round 2: same node image as ../Dockerfile but with the agent binary
# prebuilt on the host (linux/amd64 cross-compile) so wrangler's docker
# build doesn't run the full k8s Go build under qemu on ARM Macs.
#
# Base moved from alpine:3.21 to debian:bookworm-slim for per-Pod
# Cloudflare Mesh membership (spikes/s17-mesh-nodevm/FINDINGS.md gate 1,
# closed 2026-07-06, and the per-Pod-Mesh entry that adopted it here):
# the `cloudflare-warp` client only ships Ubuntu/Debian/RHEL builds, no
# Alpine/musl, no static binary. The apt sequence below is gate 1/2's own
# proven recipe (spikes/s17-mesh-nodevm/Dockerfile), reused verbatim.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl gpg iptables ipset conntrack && \
    curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --yes --dearmor --output /usr/share/keyrings/cloudflare-warp-archive-keyring.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg] https://pkg.cloudflareclient.com/ bookworm main" > /etc/apt/sources.list.d/cloudflare-client.list && \
    apt-get update && apt-get install -y --no-install-recommends cloudflare-warp && \
    rm -rf /var/lib/apt/lists/*
RUN curl -fsSL -o /usr/local/bin/k3s \
      "https://github.com/k3s-io/k3s/releases/download/v1.36.2%2Bk3s1/k3s" \
    && chmod +x /usr/local/bin/k3s
COPY k8flare-agent /usr/local/bin/k8flare-agent
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /usr/local/bin/k8flare-agent
ENTRYPOINT ["/entrypoint.sh"]
