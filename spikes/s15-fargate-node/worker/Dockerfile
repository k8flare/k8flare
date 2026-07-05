# S15 round 2: same node image as ../Dockerfile but with the agent binary
# prebuilt on the host (linux/amd64 cross-compile) so wrangler's docker
# build doesn't run the full k8s Go build under qemu on ARM Macs.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates iptables ip6tables ipset conntrack-tools curl
RUN curl -fsSL -o /usr/local/bin/k3s \
      "https://github.com/k3s-io/k3s/releases/download/v1.36.2%2Bk3s1/k3s" \
    && chmod +x /usr/local/bin/k3s
COPY k8flare-agent /usr/local/bin/k8flare-agent
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /usr/local/bin/k8flare-agent
ENTRYPOINT ["/entrypoint.sh"]
