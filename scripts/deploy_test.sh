#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/bin"
cat > "$tmp/bin/pnpm" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DEPLOY_TEST_LOG"
if [[ "$1 $2" == "exec wrangler" ]]; then
  shift 2
  exec wrangler "$@"
fi
if [[ "$1 $2" == "exec cf" ]]; then
  shift 2
  exec cf "$@"
fi
EOF
cat > "$tmp/bin/cf" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DEPLOY_TEST_LOG"
case "$*" in
  "workers deployments list"*)
    if [[ "${DEPLOY_TEST_STATUS_FAIL:-}" == "1" ]]; then
      printf '%s\n' 'status request failed' >&2
      exit 1
    fi
    if [[ "${DEPLOY_TEST_SPLIT:-}" == "1" ]]; then
      printf '%s\n' '{"deployments":[{"id":"deployment-id","versions":[{"version_id":"newer-version","percentage":10},{"version_id":"old-version","percentage":90}]}]}'
      exit 0
    fi
    printf '%s\n' '{"deployments":[{"id":"deployment-id","versions":[{"version_id":"old-version","percentage":100}]},{"id":"older","versions":[{"version_id":"older-version","percentage":100}]}]}'
    ;;
  "workers deployments create"*"\"version_id\":\"new-version\",\"percentage\":${DEPLOY_TEST_FAIL_DEPLOY_AT:-none}}"*)
    printf '%s\n' 'deploy request failed' >&2
    exit 1
    ;;
  "workers versions create"*)
    if [[ "${DEPLOY_TEST_MIGRATION:-}" == "1" ]]; then
      printf '%s\n' 'Cannot upload: this Worker has a pending Durable Object migration.' >&2
      exit 1
    fi
    printf '%s\n' "$*" | sed -n 's/.*--tag \([^ ]*\).*/\1/p' > "$DEPLOY_TEST_TAG"
    ;;
  "workers versions list"*)
    printf '[{"id":"old-version","number":1,"annotations":{"workers/tag":"unrelated"}},{"id":"new-version","number":2,"annotations":{"workers/tag":"%s"}}]\n' "$(cat "$DEPLOY_TEST_TAG")"
    ;;
esac
EOF
cat > "$tmp/bin/wrangler" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DEPLOY_TEST_LOG"
case "$*" in
  *"versions upload"*)
    printf '%s\n' '{"type":"version-upload","version":1,"worker_name":"k8flare","worker_tag":null,"version_id":"new-version","preview_url":null,"preview_alias_url":null,"wrangler_environment":null,"worker_name_overridden":false,"timestamp":"2026-10-13T00:00:00.000Z"}' >> "$WRANGLER_OUTPUT_FILE_PATH"
    ;;
esac
EOF
cat > "$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DEPLOY_TEST_LOG"
count_file="$DEPLOY_TEST_CURL_COUNT"
count=0
if [[ -f "$count_file" ]]; then
  count=$(<"$count_file")
fi
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
if [[ -n "${DEPLOY_TEST_FAIL_CURL_AT:-}" && "$count" -ge "$DEPLOY_TEST_FAIL_CURL_AT" ]]; then
  exit 22
fi
EOF
cat > "$tmp/bin/sleep" <<'EOF'
#!/usr/bin/env bash
:
EOF
chmod +x "$tmp/bin/pnpm" "$tmp/bin/cf" "$tmp/bin/wrangler" "$tmp/bin/curl" "$tmp/bin/sleep"

run() {
  : > "$tmp/log"
  : > "$tmp/curl-count"
  PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_TAG="$tmp/tag" DEPLOY_ROLLOUT_SOAK_SECONDS=0 "$@"
}

assert_contains() {
  grep -Fqx "$1" "$tmp/log"
}

assert_before() {
  local earlier
  local later
  earlier=$(grep -Fnx "$1" "$tmp/log" | cut -d: -f1)
  later=$(grep -Fnx "$2" "$tmp/log" | cut -d: -f1)
  (( earlier < later ))
}

run bash "$root/scripts/deploy.sh"
assert_contains 'exec cf workers deployments list --worker k8flare'
assert_contains 'workers versions list --worker-id k8flare --per-page 25'
assert_contains 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"new-version","percentage":10},{"version_id":"old-version","percentage":90}]'
assert_contains 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"new-version","percentage":50},{"version_id":"old-version","percentage":50}]'
assert_contains 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"new-version","percentage":100},{"version_id":"old-version","percentage":0}]'
assert_contains 'versions upload -c wrangler.edge.jsonc'
assert_contains 'versions deploy new-version@100% -y -c wrangler.edge.jsonc'
assert_contains 'versions upload -c wrangler.hpa.jsonc'
assert_contains 'versions deploy new-version@100% -y -c wrangler.hpa.jsonc'
assert_contains 'versions upload -c wrangler.attachdetach.jsonc'
assert_contains 'versions deploy new-version@100% -y -c wrangler.attachdetach.jsonc'
assert_before 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"new-version","percentage":100},{"version_id":"old-version","percentage":0}]' 'versions upload -c wrangler.edge.jsonc'
assert_before 'versions deploy new-version@100% -y -c wrangler.edge.jsonc' 'versions upload -c wrangler.hpa.jsonc'
assert_before 'versions deploy new-version@100% -y -c wrangler.hpa.jsonc' 'versions upload -c wrangler.attachdetach.jsonc'

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_TAG="$tmp/tag" DEPLOY_TEST_STATUS_FAIL=1 bash "$root/scripts/deploy.sh" 2>"$tmp/status.err"; then
  exit 1
fi
grep -Fq 'status request failed' "$tmp/status.err"
grep -Fq 'DEPLOY_ROLLOUT=immediate' "$tmp/status.err"
if grep -Fq 'deploy -c' "$tmp/log" || grep -Fq 'workers deployments create' "$tmp/log"; then
  exit 1
fi

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_TAG="$tmp/tag" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_FAIL_CURL_AT=2 bash "$root/scripts/deploy.sh"; then
  exit 1
fi
assert_contains 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"old-version","percentage":100}]'

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_TAG="$tmp/tag" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_MIGRATION=1 bash "$root/scripts/deploy.sh" 2>"$tmp/migration.err"; then
  exit 1
fi
grep -Fq 'Durable Object migration is pending' "$tmp/migration.err"

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_TAG="$tmp/tag" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_SPLIT=1 bash "$root/scripts/deploy.sh" 2>"$tmp/split.err"; then
  exit 1
fi
grep -Fq 'already split between versions' "$tmp/split.err"
if grep -Eq 'versions (upload|deploy|create) |deployments create' "$tmp/log"; then
  exit 1
fi

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_TAG="$tmp/tag" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_FAIL_DEPLOY_AT=50 bash "$root/scripts/deploy.sh"; then
  exit 1
fi
assert_contains 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"new-version","percentage":10},{"version_id":"old-version","percentage":90}]'
assert_contains 'workers deployments create --worker k8flare --strategy percentage --versions [{"version_id":"old-version","percentage":100}]'

run env DEPLOY_ROLLOUT=immediate bash "$root/scripts/deploy.sh"
assert_contains 'exec cf deploy'
assert_contains 'exec wrangler deploy -c wrangler.edge.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.hpa.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.attachdetach.jsonc'
