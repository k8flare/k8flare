# S15: cf-containers per-Pod node image — the ONE deploy-time-fixed image
# workers/nodes ships. Runs the unmodified k3s agent embed (cmd/agent:
# kubelet + containerd + flannel) inside a Cloudflare Container microVM;
# containerd inside pulls the Pod's arbitrary user image at runtime, which
# is how the image-allowlist limitation dissolves (the per-Pod managed-node shape: each Pod gets its own microVM with a kubelet under it).
#
# Build context is the repo root (needs the full Go module):
#   docker build -f spikes/s15-containers-node/Dockerfile -t k8flare/containers-node .
FROM golang:1.26-alpine AS build
RUN apk add --no-cache bash git jq perl-utils python3
WORKDIR /src
COPY . .
# .build mirrors are required by go.mod's replace lines
RUN bash scripts/gen-k8s-js-mirror.sh && bash scripts/gen-clientgo-lean-mirror.sh
RUN CGO_ENABLED=0 GOOS=linux go build -o /k8flare-agent ./cmd/agent

# k3s needs a real userland: iptables, ca-certs, kmod bits. Alpine works
# for k3s upstream images; start minimal and grow only on measured need.
FROM alpine:3.21
ARG TARGETARCH
RUN apk add --no-cache ca-certificates iptables ip6tables ipset conntrack-tools curl
# Official k3s binary (single static file): cmd/agent runs `k3s check-config`
# once so k3s self-extracts containerd/runc/CNI into the data dir, then the
# embed uses those -- see cmd/agent's prepareK3sDataDir. Version matches the
# repo's k3s-io pin (v1.36.2-k3s1).
RUN case "$TARGETARCH" in \
      arm64) K3S_SUFFIX="-arm64" ;; \
      *) K3S_SUFFIX="" ;; \
    esac \
    && curl -fsSL -o /usr/local/bin/k3s \
      "https://github.com/k3s-io/k3s/releases/download/v1.36.2%2Bk3s1/k3s${K3S_SUFFIX}" \
    && chmod +x /usr/local/bin/k3s
COPY --from=build /k8flare-agent /usr/local/bin/k8flare-agent
ENV K3S_TOKEN=""
COPY spikes/s15-containers-node/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
# SERVER_URL/NODE_NAME provided per-instance via Containers envVars.
ENTRYPOINT ["/entrypoint.sh"]
