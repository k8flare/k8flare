#!/usr/bin/env bash
set -euo pipefail

# Cloudflare Tunnel + VPC Service Setup for k8flare
#
# This script sets up the infrastructure needed for Workers to communicate
# with kubelet via Cloudflare Tunnel and Workers VPC Service binding.
#
# Architecture Overview:
#
#   [Cloudflare Worker]
#       │  env.KUBELET_VPC.fetch(...)
#       ▼
#   [VPC Service Binding]
#       │  Cloudflare internal network
#       ▼
#   [Cloudflare Tunnel (cloudflared)]
#       │  Private connection
#       ▼
#   [k3s agent node — kubelet :10250]
#
# This enables the Worker to proxy kubelet API calls (e.g., pod logs, exec)
# through Cloudflare's private network without exposing kubelet to the internet.
#
# Prerequisites:
#   - cloudflared CLI installed (https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)
#   - wrangler CLI available (npx wrangler)
#   - jq installed
#   - Cloudflare account with Workers VPC access
#   - CLOUDFLARE_API_TOKEN env var set (for wrangler and cloudflared)
#
# Usage:
#   export CLOUDFLARE_ACCOUNT_ID=<YOUR_ACCOUNT_ID>
#   export NODE_INTERNAL_IP=192.168.1.100
#   ./scripts/setup-tunnel.sh
#
# For step-by-step guided mode (prints commands without executing):
#   DRY_RUN=1 ./scripts/setup-tunnel.sh

# --- Configuration ---

ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-<YOUR_ACCOUNT_ID>}"
TUNNEL_NAME="${TUNNEL_NAME:-k3s-kubelet}"
SERVICE_NAME="${SERVICE_NAME:-kubelet-vpc}"
KUBELET_PORT="${KUBELET_PORT:-10250}"
NODE_INTERNAL_IP="${NODE_INTERNAL_IP:-}"
DRY_RUN="${DRY_RUN:-0}"
WRANGLER_CONFIG="${WRANGLER_CONFIG:-wrangler.jsonc}"

# Script directory (for resolving relative paths)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- Helper Functions ---

info() {
  echo -e "\033[1;34m[INFO]\033[0m $*"
}

warn() {
  echo -e "\033[1;33m[WARN]\033[0m $*"
}

error() {
  echo -e "\033[1;31m[ERROR]\033[0m $*" >&2
}

run_cmd() {
  if [ "$DRY_RUN" = "1" ]; then
    echo "  \$ $*"
  else
    "$@"
  fi
}

check_command() {
  if ! command -v "$1" &>/dev/null; then
    error "$1 is not installed. $2"
    return 1
  fi
}

# --- Preflight Checks ---

info "Checking prerequisites..."

check_command jq "Install from https://jqlang.github.io/jq/download/" || exit 1

if ! command -v cloudflared &>/dev/null; then
  warn "cloudflared not found locally. It must be installed on the k3s agent node."
  warn "Download: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
fi

# Verify wrangler is available
if ! npx wrangler --version &>/dev/null 2>&1; then
  error "wrangler is not available. Run: npm install wrangler"
  exit 1
fi

echo ""
info "Configuration:"
echo "  ACCOUNT_ID:      ${ACCOUNT_ID}"
echo "  TUNNEL_NAME:     ${TUNNEL_NAME}"
echo "  SERVICE_NAME:    ${SERVICE_NAME}"
echo "  KUBELET_PORT:    ${KUBELET_PORT}"
echo "  NODE_INTERNAL_IP: ${NODE_INTERNAL_IP:-<not set>}"
echo "  DRY_RUN:         ${DRY_RUN}"
echo ""

# ============================================================
# Step 1: Create Cloudflare Tunnel (remotely managed)
# ============================================================

echo "============================================================"
info "Step 1: Create Cloudflare Tunnel"
echo "============================================================"
echo ""
echo "A remotely-managed tunnel allows cloudflared to connect to"
echo "Cloudflare's network using a token (no local config files needed)."
echo ""

# Check if tunnel already exists
EXISTING_TUNNEL=""
if [ "$DRY_RUN" = "0" ]; then
  EXISTING_TUNNEL=$(cloudflared tunnel list --output json 2>/dev/null \
    | jq -r ".[] | select(.name == \"${TUNNEL_NAME}\") | .id" 2>/dev/null || true)
fi

if [ -n "$EXISTING_TUNNEL" ]; then
  TUNNEL_ID="$EXISTING_TUNNEL"
  info "Tunnel '${TUNNEL_NAME}' already exists: ${TUNNEL_ID}"
else
  info "Creating tunnel: ${TUNNEL_NAME}"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  \$ cloudflared tunnel create ${TUNNEL_NAME}"
    TUNNEL_ID="<TUNNEL_ID>"
  else
    TUNNEL_OUTPUT=$(cloudflared tunnel create "${TUNNEL_NAME}" 2>&1)
    echo "$TUNNEL_OUTPUT"
    # Extract tunnel ID from output
    TUNNEL_ID=$(echo "$TUNNEL_OUTPUT" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)
    if [ -z "$TUNNEL_ID" ]; then
      error "Failed to extract tunnel ID from output."
      error "Create the tunnel manually: cloudflared tunnel create ${TUNNEL_NAME}"
      exit 1
    fi
    info "Tunnel created with ID: ${TUNNEL_ID}"
  fi
fi

echo ""

# Get tunnel token for running on the agent node
info "Retrieving tunnel token..."
if [ "$DRY_RUN" = "1" ]; then
  echo "  \$ cloudflared tunnel token ${TUNNEL_NAME}"
  TUNNEL_TOKEN="<TUNNEL_TOKEN>"
else
  TUNNEL_TOKEN=$(cloudflared tunnel token "${TUNNEL_ID}" 2>/dev/null || true)
  if [ -z "$TUNNEL_TOKEN" ]; then
    warn "Could not retrieve tunnel token automatically."
    warn "Run manually: cloudflared tunnel token ${TUNNEL_ID}"
  fi
fi

echo ""

# ============================================================
# Step 2: Install and run cloudflared on k3s agent node
# ============================================================

echo "============================================================"
info "Step 2: Install cloudflared on k3s Agent Node"
echo "============================================================"
echo ""
echo "Run the following commands on your k3s agent node (Linux):"
echo ""
echo "  # Download cloudflared"
echo "  curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 \\"
echo "    -o /usr/local/bin/cloudflared"
echo "  chmod +x /usr/local/bin/cloudflared"
echo ""
echo "  # Run as a systemd service (recommended for production)"
if [ -n "$TUNNEL_TOKEN" ] && [ "$TUNNEL_TOKEN" != "<TUNNEL_TOKEN>" ]; then
  echo "  cloudflared service install ${TUNNEL_TOKEN}"
else
  echo "  cloudflared service install <TUNNEL_TOKEN>"
fi
echo ""
echo "  # Or run manually for testing"
if [ -n "$TUNNEL_TOKEN" ] && [ "$TUNNEL_TOKEN" != "<TUNNEL_TOKEN>" ]; then
  echo "  cloudflared tunnel run --token ${TUNNEL_TOKEN}"
else
  echo "  cloudflared tunnel run --token <TUNNEL_TOKEN>"
fi
echo ""
echo "  # Verify the tunnel is connected"
echo "  cloudflared tunnel info ${TUNNEL_NAME}"
echo ""

# ============================================================
# Step 3: Configure Tunnel to route traffic to kubelet
# ============================================================

echo "============================================================"
info "Step 3: Configure Tunnel Routing to Kubelet"
echo "============================================================"
echo ""

if [ -z "$NODE_INTERNAL_IP" ]; then
  warn "NODE_INTERNAL_IP is not set."
  echo "  Set it to the k3s agent's internal IP address, e.g.:"
  echo "  export NODE_INTERNAL_IP=192.168.1.100"
  echo ""
  echo "  You can find this by running on the agent node:"
  echo "    hostname -I | awk '{print \$1}'"
  echo ""
  NODE_INTERNAL_IP="<NODE_INTERNAL_IP>"
fi

echo "The tunnel must be configured (via Cloudflare dashboard or API) to"
echo "route traffic to the kubelet endpoint on the agent node."
echo ""
echo "In the Cloudflare Zero Trust dashboard:"
echo "  1. Go to Networks > Tunnels > ${TUNNEL_NAME}"
echo "  2. Add a private network route: ${NODE_INTERNAL_IP}/32"
echo ""
echo "Or via cloudflared CLI on the agent node:"
echo "  cloudflared tunnel route ip add ${NODE_INTERNAL_IP}/32 ${TUNNEL_ID:-<TUNNEL_ID>}"
echo ""

# ============================================================
# Step 4: Create VPC Service for kubelet
# ============================================================

echo "============================================================"
info "Step 4: Create VPC Service"
echo "============================================================"
echo ""
echo "Workers VPC Service bindings allow Workers to reach private"
echo "network resources exposed by Cloudflare Tunnels."
echo ""

info "Creating VPC service: ${SERVICE_NAME}"
echo ""
echo "Run the following command:"
echo ""
echo "  npx wrangler service create ${SERVICE_NAME} \\"
echo "    --tunnel-id ${TUNNEL_ID:-<TUNNEL_ID>}"
echo ""
echo "Note: VPC Service API may vary as the feature is in beta."
echo "Refer to the latest docs:"
echo "  https://developers.cloudflare.com/workers/runtime-apis/bindings/service-bindings/rpc/"
echo "  https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/private-net/cloudflared/"
echo ""

# ============================================================
# Step 5: Update wrangler.jsonc with VPC Service binding
# ============================================================

echo "============================================================"
info "Step 5: Update wrangler.jsonc"
echo "============================================================"
echo ""

WRANGLER_PATH="${PROJECT_DIR}/${WRANGLER_CONFIG}"

if [ ! -f "$WRANGLER_PATH" ]; then
  error "wrangler.jsonc not found at: ${WRANGLER_PATH}"
  exit 1
fi

echo "Add the following VPC service binding to ${WRANGLER_CONFIG}:"
echo ""
echo '  // VPC Service binding for kubelet communication via Cloudflare Tunnel'
echo '  "services": ['
echo '    {'
echo '      "binding": "KUBELET_VPC",'
echo "      \"service\": \"${SERVICE_NAME}\","
echo '      "entrypoint": "default"'
echo '    }'
echo '  ]'
echo ""
echo "The Worker code (worker.mjs) already uses env.KUBELET_VPC to proxy"
echo "kubelet requests (pod logs, exec, etc.) when the binding is available."
echo ""

# Check if binding is already configured
if grep -q "KUBELET_VPC" "$WRANGLER_PATH" 2>/dev/null; then
  info "KUBELET_VPC binding already present in ${WRANGLER_CONFIG}."
else
  warn "KUBELET_VPC binding not found in ${WRANGLER_CONFIG}."
  echo "  Add the services block shown above to your wrangler.jsonc."
fi

echo ""

# ============================================================
# Step 6: Deploy and Test
# ============================================================

echo "============================================================"
info "Step 6: Deploy and Test"
echo "============================================================"
echo ""
echo "1. Deploy the Worker:"
echo "   npx wrangler deploy"
echo ""
echo "2. Verify the tunnel is connected:"
echo "   cloudflared tunnel info ${TUNNEL_NAME}"
echo ""
echo "3. Test kubelet connectivity (pod logs):"
echo "   kubectl logs <pod-name> --server=https://your-k8flare.workers.dev"
echo ""
echo "4. If using curl directly:"
echo '   curl -H "Authorization: Bearer $K3S_TOKEN" \'
echo "     https://your-k8flare.workers.dev/api/v1/namespaces/default/pods/<pod>/log"
echo ""

# ============================================================
# Summary
# ============================================================

echo "============================================================"
info "Setup Summary"
echo "============================================================"
echo ""
echo "  Tunnel Name:     ${TUNNEL_NAME}"
echo "  Tunnel ID:       ${TUNNEL_ID:-<not created>}"
echo "  VPC Service:     ${SERVICE_NAME}"
echo "  Kubelet Port:    ${KUBELET_PORT}"
echo "  Node IP:         ${NODE_INTERNAL_IP}"
echo ""
echo "Traffic flow:"
echo "  Worker -> env.KUBELET_VPC.fetch() -> VPC Service -> Tunnel -> cloudflared -> kubelet:${KUBELET_PORT}"
echo ""
info "Done. Review the steps above and complete any manual actions."
