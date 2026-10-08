#!/usr/bin/env bash
set -euo pipefail
SRC=$(pwd)
TEST_STAGE_ROOT=$(mktemp -d)
APP="$TEST_STAGE_ROOT/runtime"
BUILD_COMMIT=$(git rev-parse HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-X main.buildCommit=$BUILD_COMMIT -X main.buildTime=$BUILD_TIME"
source deploy/service-topology.sh
dob_stage_runtime
trap 'dob_cleanup_stage; rm -rf -- "$TEST_STAGE_ROOT"' EXIT
artifact="$DOB_STAGE/bin/server-guardian-linux-amd64"
cp "$artifact" "$TEST_STAGE_ROOT/original"
for optimize in 0 1; do
export PYTHONOPTIMIZE="$optimize"
mv "$artifact" "$TEST_STAGE_ROOT/absent"
if dob_verify_staged_runtime >"$TEST_STAGE_ROOT/fault.log" 2>&1; then echo 'missing guardian accepted';exit 1;fi
grep -q 'required artifact missing' "$TEST_STAGE_ROOT/fault.log"
mv "$TEST_STAGE_ROOT/absent" "$artifact"
printf 'tamper' >> "$artifact"
if dob_verify_staged_runtime >"$TEST_STAGE_ROOT/fault.log" 2>&1; then echo 'tampered guardian accepted';exit 1;fi
grep -q 'guardian hash mismatch' "$TEST_STAGE_ROOT/fault.log"
cp "$DOB_STAGE/bin/server-guardian-linux-arm64" "$artifact"
if dob_verify_staged_runtime >"$TEST_STAGE_ROOT/fault.log" 2>&1; then echo 'wrong architecture accepted';exit 1;fi
grep -q 'guardian architecture mismatch' "$TEST_STAGE_ROOT/fault.log"
cp "$TEST_STAGE_ROOT/original" "$artifact"
dob_verify_staged_runtime
done
echo 'RELEASE_REAL_CROSS_BUILD_MISSING_TAMPER_WRONG_ARCH_PYTHONOPTIMIZE PASS'
