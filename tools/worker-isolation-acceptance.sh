#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOTMPDIR=/root/buildtmp/dob-modular-recovery
mkdir -p "$GOTMPDIR"
set -a
source /etc/digital-ocean-bot/env
set +a
BULK_TEST_DATABASE_URL="$(python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ['DATABASE_URL'])
print(urllib.parse.urlunsplit(u._replace(path='/dob_bulk_test_20261003')))
PY
)"
export BULK_TEST_DATABASE_URL
go test -race -p 1 ./cmd/worker ./internal/worker ./internal/observability -count=1 -v
go test -race ./internal/app -run 'TestPrepared(Readiness|Snapshot)' -count=1 -v
# Retain the native HTTP/PG uncertainty, ownership and finite-scope gates.
bash tools/modular-recovery-acceptance.sh
systemd-analyze verify deploy/digital-ocean-bot-worker.service deploy/digital-ocean-bot-worker-panels.service
bash -n deploy/install.sh deploy/upgrade.sh
git diff --check
echo WORKER_ISOLATION_ACCEPTANCE_PASS
