#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

rollout_steps=${DEPLOY_ROLLOUT_STEPS:-10,50,100}
soak_seconds=${DEPLOY_ROLLOUT_SOAK_SECONDS:-30}
ready_retries=12
ready_retry_seconds=5
main_config=cloudflare.config.ts
satellite_configs=(wrangler.edge.jsonc wrangler.hpa.jsonc wrangler.attachdetach.jsonc)

wrangler() {
  pnpm exec wrangler "$@"
}

cf() {
  pnpm exec cf "$@"
}

if [[ -z "${CLOUDFLARE_ACCOUNT_ID:-}" ]]; then
  CLOUDFLARE_ACCOUNT_ID=$(sed -n 's/^[[:space:]]*CLOUDFLARE_ACCOUNT_ID: bindings.text("\([^"]*\)"),$/\1/p' "$main_config")
  export CLOUDFLARE_ACCOUNT_ID
fi

worker_name() {
  node -e 'const source = require("node:fs").readFileSync(process.argv[1], "utf8"); const match = source.match(/^\t\tname:\s*"([^"]+)"/m); if (!match) process.exit(1); console.log(match[1])' "$1"
}

rollout_hostname() {
  if [[ -n "${DEPLOY_ROLLOUT_HOSTNAME:-}" ]]; then
    printf '%s\n' "$DEPLOY_ROLLOUT_HOSTNAME"
    return
  fi
  node -e 'const source = require("node:fs").readFileSync(process.argv[1], "utf8"); const match = source.match(/domains:[^\n]*: \[\s*"([^"]+)"/); if (!match) process.exit(1); console.log(match[1])' "$main_config"
}

current_version() {
  local deployments
  deployments=$(cf workers deployments list --worker "$(worker_name "$1")")
  node -e 'const deployment = JSON.parse(process.argv[1]).deployments[0]; const serving = deployment.versions.filter((candidate) => candidate.percentage > 0); if (serving.length === 0) process.exit(1); if (serving.length > 1) process.exit(3); console.log(serving[0].version_id)' "$deployments"
}

upload_main() {
  local error
  local tag
  local name
  error=$(mktemp)
  tag="deploy-$(date +%s)-$$"
  name=$(worker_name "$main_config")
  if ! cf workers versions create --tag "$tag" >"$error" 2>&1; then
    if grep -Fiq 'migration' "$error"; then
      cat "$error" >&2
      rm -f "$error"
      printf '%s\n' 'Durable Object migration is pending; gradual rollout cannot apply it. Re-run with DEPLOY_ROLLOUT=immediate.' >&2
      exit 1
    fi
    cat "$error" >&2
    rm -f "$error"
    printf 'versions create failed for %s\n' "$main_config" >&2
    exit 1
  fi
  rm -f "$error"
  cf workers versions list --worker-id "$name" --per-page 25 | node -e 'const versions = JSON.parse(require("node:fs").readFileSync(0, "utf8")).filter((candidate) => candidate.annotations?.["workers/tag"] === process.argv[1]); if (versions.length === 0) process.exit(1); console.log(versions.sort((a, b) => b.number - a.number)[0].id)' "$tag"
}

deploy_main_versions() {
  local split
  split=$(node -e 'console.log(JSON.stringify(process.argv.slice(1).map((entry) => { const [version_id, percentage] = entry.split("@"); return { version_id, percentage: Number(percentage) }; })))' "$@")
  cf workers deployments create --worker "$(worker_name "$main_config")" --strategy percentage --versions "$split"
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
  cf deploy
  for config in "${satellite_configs[@]}"; do
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

new_version=$(upload_main)
IFS=, read -r -a steps <<< "$rollout_steps"
for step in "${steps[@]}"; do
  if ! deploy_main_versions "${new_version}@${step}" "${old_version}@$((100 - step))" || ! ready "$new_version"; then
    printf '%s\n' 'Main Worker rollout step or readiness gate failed; rolling back to the previous version.' >&2
    deploy_main_versions "${old_version}@100"
    exit 1
  fi
  if [[ "$step" != "${steps[${#steps[@]} - 1]}" ]]; then
    sleep "$soak_seconds"
  fi
done

for config in "${satellite_configs[@]}"; do
  deploy_satellite "$config"
done
