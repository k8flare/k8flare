#!/bin/sh
# Gate-1 probe: start warp-svc headless, confirm the daemon accepts
# warp-cli, report TUN availability. Stops at the enrollment boundary.
set -x
ls -la /dev/net/tun 2>&1
warp-svc &
SVC_PID=$!
sleep 5
warp-cli --accept-tos status 2>&1
warp-cli --accept-tos registration show 2>&1
# Try MDM-less new registration to surface the exact error/requirement
warp-cli --accept-tos registration new 2>&1 || true
sleep 2
kill $SVC_PID 2>/dev/null
echo "PROBE_DONE"
