#!/bin/bash
set -euo pipefail

# Install containerd
dnf install -y containerd

# Enable and start containerd
systemctl enable --now containerd

# Install kubelet (from k3s release)
curl -sfL https://github.com/k3s-io/k3s/releases/download/v1.34.5%2Bk3s1/k3s-arm64 -o /usr/local/bin/k3s
chmod +x /usr/local/bin/k3s
ln -sf /usr/local/bin/k3s /usr/local/bin/kubelet
ln -sf /usr/local/bin/k3s /usr/local/bin/kubectl
ln -sf /usr/local/bin/k3s /usr/local/bin/crictl

# Install cloudflared
curl -sfL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64 -o /usr/local/bin/cloudflared
chmod +x /usr/local/bin/cloudflared

# The agent binary will be installed separately (scp or S3)
echo "EC2 user data setup complete"
