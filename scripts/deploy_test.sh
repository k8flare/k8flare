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
    printf '%s\n' '{"versions":[{"version_id":"old-version","percentage":100}]}'
    ;;
  *"versions list"*)
    printf '%s\n' '[{"id":"old-version"},{"id":"new-version"}]'
    ;;
  *"versions upload"*)
    if [[ "${DEPLOY_TEST_MIGRATION:-}" == "1" ]]; then
      printf '%s\n' 'This Worker has a pending Durable Object migration, which cannot be applied by `wrangler versions upload`.' >&2
      exit 1
    fi
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

run env DEPLOY_ROLLOUT=immediate bash "$root/scripts/deploy.sh"
assert_contains 'exec wrangler deploy -c wrangler.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.edge.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.hpa.jsonc'
assert_contains 'exec wrangler deploy -c wrangler.attachdetach.jsonc'
