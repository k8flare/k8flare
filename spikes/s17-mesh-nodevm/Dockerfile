# S17 gate 1: does the Cloudflare One (WARP) client install and run
# headless in a glibc container/VM? pkg.cloudflareclient.com ships
# Ubuntu/Debian/RHEL only (no Alpine/musl -- checked 2026-07-06), so the
# NodeVM base image must move to Debian if Mesh becomes the standard
# network. This image is the minimal probe: install the client, start
# warp-svc, and exercise warp-cli far enough to hit the enrollment
# boundary (actual enrollment needs a Zero Trust service token).
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      curl gpg ca-certificates && \
    curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg | gpg --yes --dearmor --output /usr/share/keyrings/cloudflare-warp-archive-keyring.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/cloudflare-warp-archive-keyring.gpg] https://pkg.cloudflareclient.com/ bookworm main" > /etc/apt/sources.list.d/cloudflare-client.list && \
    apt-get update && apt-get install -y --no-install-recommends cloudflare-warp && \
    rm -rf /var/lib/apt/lists/*
COPY probe.sh /probe.sh
RUN chmod +x /probe.sh
ENTRYPOINT ["/probe.sh"]
