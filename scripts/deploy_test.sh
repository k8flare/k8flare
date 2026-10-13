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
EOF
cat > "$tmp/bin/wrangler" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$DEPLOY_TEST_LOG"
case "$*" in
  *"versions deployments status"*)
    if [[ "${DEPLOY_TEST_STATUS_FAIL:-}" == "1" ]]; then
      printf '%s\n' 'status request failed' >&2
      exit 1
    fi
    if [[ "${DEPLOY_TEST_SPLIT:-}" == "1" ]]; then
      printf '%s\n' '{"id":"deployment-id","created_on":"2026-10-13T00:00:00.000Z","versions":[{"version_id":"newer-version","percentage":10},{"version_id":"old-version","percentage":90}]}'
      exit 0
    fi
    printf '%s\n' '{"id":"deployment-id","created_on":"2026-10-13T00:00:00.000Z","versions":[{"version_id":"old-version","percentage":100}]}'
    ;;
  *"versions deploy new-version@${DEPLOY_TEST_FAIL_DEPLOY_AT:-none}%"*)
    printf '%s\n' 'deploy request failed' >&2
    exit 1
    ;;
  *"versions upload"*)
    if [[ "${DEPLOY_TEST_MIGRATION:-}" == "1" ]]; then
      printf '%s\n' 'This Worker has a pending Durable Object migration, which cannot be applied by `wrangler versions upload`.' >&2
      exit 1
    fi
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
chmod +x "$tmp/bin/pnpm" "$tmp/bin/wrangler" "$tmp/bin/curl" "$tmp/bin/sleep"

run() {
  : > "$tmp/log"
  : > "$tmp/curl-count"
  PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_ROLLOUT_SOAK_SECONDS=0 "$@"
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
assert_contains 'exec wrangler versions deployments status --json -c wrangler.jsonc'
assert_contains 'versions upload -c wrangler.jsonc'
assert_contains 'versions deploy new-version@10% old-version@90% -y -c wrangler.jsonc'
assert_contains 'versions deploy new-version@50% old-version@50% -y -c wrangler.jsonc'
assert_contains 'versions deploy new-version@100% old-version@0% -y -c wrangler.jsonc'
assert_contains 'versions upload -c wrangler.edge.jsonc'
assert_contains 'versions deploy new-version@100% -y -c wrangler.edge.jsonc'
assert_contains 'versions upload -c wrangler.hpa.jsonc'
assert_contains 'versions deploy new-version@100% -y -c wrangler.hpa.jsonc'
assert_contains 'versions upload -c wrangler.attachdetach.jsonc'
assert_contains 'versions deploy new-version@100% -y -c wrangler.attachdetach.jsonc'
assert_before 'versions deploy new-version@100% old-version@0% -y -c wrangler.jsonc' 'versions upload -c wrangler.edge.jsonc'
assert_before 'versions deploy new-version@100% -y -c wrangler.edge.jsonc' 'versions upload -c wrangler.hpa.jsonc'
assert_before 'versions deploy new-version@100% -y -c wrangler.hpa.jsonc' 'versions upload -c wrangler.attachdetach.jsonc'

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_TEST_STATUS_FAIL=1 bash "$root/scripts/deploy.sh" 2>"$tmp/status.err"; then
  exit 1
fi
grep -Fq 'status request failed' "$tmp/status.err"
grep -Fq 'DEPLOY_ROLLOUT=immediate' "$tmp/status.err"
if grep -Fq 'deploy -c' "$tmp/log"; then
  exit 1
fi

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_FAIL_CURL_AT=2 bash "$root/scripts/deploy.sh"; then
  exit 1
fi
assert_contains 'versions deploy old-version@100% -y -c wrangler.jsonc'

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_MIGRATION=1 bash "$root/scripts/deploy.sh" 2>"$tmp/migration.err"; then
  exit 1
fi
grep -Fq 'Durable Object migration is pending' "$tmp/migration.err"

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_SPLIT=1 bash "$root/scripts/deploy.sh" 2>"$tmp/split.err"; then
  exit 1
fi
grep -Fq 'already split between versions' "$tmp/split.err"
if grep -Eq 'versions (upload|deploy) ' "$tmp/log"; then
  exit 1
fi

: > "$tmp/log"
: > "$tmp/curl-count"
if PATH="$tmp/bin:$PATH" DEPLOY_TEST_LOG="$tmp/log" DEPLOY_TEST_CURL_COUNT="$tmp/curl-count" DEPLOY_ROLLOUT_SOAK_SECONDS=0 DEPLOY_TEST_FAIL_DEPLOY_AT=50 bash "$root/scripts/deploy.sh"; then
  exit 1
fi
assert_contains 'versions deploy new-version@10% old-version@90% -y -c wrangler.jsonc'
assert_contains 'versions deploy old-version@100% -y -c wrangler.jsonc'

run env DEPLOY_ROLLOUT=immediate bash "$root/scripts/deploy.sh"
assert_contains 'exec wrangler deploy -c wrangler.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.edge.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.hpa.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.attachdetach.jsonc'
