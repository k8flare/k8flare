#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

rollout_steps=${DEPLOY_ROLLOUT_STEPS:-10,50,100}
soak_seconds=${DEPLOY_ROLLOUT_SOAK_SECONDS:-30}
ready_retries=12
ready_retry_seconds=5
main_config=wrangler.jsonc
satellite_configs=(wrangler.edge.jsonc wrangler.hpa.jsonc wrangler.attachdetach.jsonc)

wrangler() {
  pnpm exec wrangler "$@"
}

worker_name() {
  node -e 'const source = require("node:fs").readFileSync(process.argv[1], "utf8"); const match = source.match(/^\s*"name":\s*"([^"]+)"/m); if (!match) process.exit(1); console.log(match[1])' "$1"
}

hostname() {
  if [[ -n "${DEPLOY_ROLLOUT_HOSTNAME:-}" ]]; then
    printf '%s\n' "$DEPLOY_ROLLOUT_HOSTNAME"
    return
  fi
  node -e 'const source = require("node:fs").readFileSync(process.argv[1], "utf8"); const match = source.match(/"routes":\s*\[\s*\{\s*"pattern":\s*"([^"]+)",\s*"custom_domain":\s*true/); if (!match) process.exit(1); console.log(match[1].replace(/\/\*$/, ""))' "$main_config"
}

current_version() {
  local deployment
  deployment=$(wrangler versions deployments status --json -c "$1")
  node -e 'const deployment = JSON.parse(process.argv[1]); const version = deployment.versions.find((candidate) => candidate.percentage > 0); if (!version) process.exit(1); console.log(version.version_id)' "$deployment"
}

new_version() {
  local versions
  versions=$(wrangler versions list --json -c "$1")
  node -e 'const versions = JSON.parse(process.argv[1]); const version = versions.at(-1); if (!version) process.exit(1); console.log(version.id)' "$versions"
}

upload() {
  local config=$1
  local output
  output=$(mktemp)
  if wrangler versions upload -c "$config" >"$output" 2>&1; then
    rm -f "$output"
    return
  fi
  if grep -Fq 'pending Durable Object migration' "$output"; then
    rm -f "$output"
    printf '%s\n' 'Durable Object migration is pending; gradual rollout cannot apply it. Re-run with DEPLOY_ROLLOUT=immediate.' >&2
    exit 1
  fi
  rm -f "$output"
  printf 'versions upload failed for %s\n' "$config" >&2
  exit 1
}

ready() {
  local version=$1
  local name
  local host
  name=$(worker_name "$main_config")
  host=$(hostname)
  for ((attempt = 1; attempt <= ready_retries; attempt++)); do
    if curl --fail --silent --show-error --header "Cloudflare-Workers-Version-Overrides: ${name}=\"${version}\"" "https://${host}/readyz"; then
      return
    fi
    if (( attempt < ready_retries )); then
      sleep "$ready_retry_seconds"
    fi
  done
  return 1
}

immediate_deploy() {
  local config
  for config in "$main_config" "${satellite_configs[@]}"; do
    wrangler deploy -c "$config"
  done
}

deploy_satellite() {
  local config=$1
  local deployment
  local version
  upload "$config"
  version=$(new_version "$config")
  if wrangler versions deploy "${version}@100%" -y -c "$config"; then
    return
  fi
  deployment=$(wrangler versions deployments status --json -c "$config" || true)
  printf '%s current deployment: %s\n' "$config" "$deployment" >&2
  exit 1
}

if [[ "${DEPLOY_ROLLOUT:-}" == immediate ]]; then
  immediate_deploy
  exit 0
fi

if ! old_version=$(current_version "$main_config"); then
  printf '%s\n' 'No deployed main Worker version was found; using an immediate deploy.' >&2
  immediate_deploy
  exit 0
fi

upload "$main_config"
new_version=$(new_version "$main_config")
IFS=, read -r -a steps <<< "$rollout_steps"
for step in "${steps[@]}"; do
  wrangler versions deploy "${new_version}@${step}%" "${old_version}@$((100 - step))%" -y -c "$main_config"
  if ! ready "$new_version"; then
    printf '%s\n' 'Main Worker readiness gate failed; rolling back to the previous version.' >&2
    wrangler versions deploy "${old_version}@100%" -y -c "$main_config"
    exit 1
  fi
  if [[ "$step" != "${steps[${#steps[@]} - 1]}" ]]; then
    sleep "$soak_seconds"
  fi
done

for config in "${satellite_configs[@]}"; do
  deploy_satellite "$config"
done
