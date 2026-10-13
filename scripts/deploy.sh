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

rollout_hostname() {
  if [[ -n "${DEPLOY_ROLLOUT_HOSTNAME:-}" ]]; then
    printf '%s\n' "$DEPLOY_ROLLOUT_HOSTNAME"
    return
  fi
  node -e 'const source = require("node:fs").readFileSync(process.argv[1], "utf8"); const match = source.match(/"routes":\s*\[\s*\{\s*"pattern":\s*"([^"]+)",\s*"custom_domain":\s*true/); if (!match) process.exit(1); console.log(match[1].replace(/\/\*$/, ""))' "$main_config"
}

current_version() {
  local deployment
  deployment=$(wrangler versions deployments status --json -c "$1")
  node -e 'const deployment = JSON.parse(process.argv[1]); const serving = deployment.versions.filter((candidate) => candidate.percentage > 0); if (serving.length === 0) process.exit(1); if (serving.length > 1) process.exit(3); console.log(serving[0].version_id)' "$deployment"
}

upload() {
  local config=$1
  local error
  local output
  local version
  error=$(mktemp)
  output=$(mktemp)
  if ! WRANGLER_OUTPUT_FILE_PATH="$output" wrangler versions upload -c "$config" >"$error" 2>&1; then
    if grep -Fq 'pending Durable Object migration' "$error"; then
      rm -f "$error" "$output"
      printf '%s\n' 'Durable Object migration is pending; gradual rollout cannot apply it. Re-run with DEPLOY_ROLLOUT=immediate.' >&2
      exit 1
    fi
    rm -f "$error" "$output"
    printf 'versions upload failed for %s\n' "$config" >&2
    exit 1
  fi
  rm -f "$error"
  version=$(node -e 'const entries = require("node:fs").readFileSync(process.argv[1], "utf8").trim().split("\n").map(JSON.parse); const entry = entries.find((candidate) => candidate.type === "version-upload" && candidate.version_id); if (!entry) process.exit(1); console.log(entry.version_id)' "$output")
  rm -f "$output"
  printf '%s\n' "$version"
}

ready() {
  local version=$1
  local name
  local host
  name=$(worker_name "$main_config")
  host=$(rollout_hostname)
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
  version=$(upload "$config")
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

old_version=$(current_version "$main_config") && status=0 || status=$?
if [[ "$status" == 3 ]]; then
  printf '%s\n' 'The main Worker is already split between versions. Finish or roll back that rollout before deploying.' >&2
  exit 1
fi
if [[ "$status" != 0 ]]; then
  printf '%s\n' 'Could not read the deployed main Worker version. For a first deploy, re-run with DEPLOY_ROLLOUT=immediate.' >&2
  exit 1
fi

new_version=$(upload "$main_config")
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
