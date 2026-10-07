#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOTMPDIR=/root/buildtmp/dob-modular-recovery
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
go test -race -p 1 ./internal/worker -run 'TestRecovery|TestAdmission' -count=1 -v
go test -race ./internal/app -run TestRecoveryCleanupBoundedAndPreservesOriginalError -count=1 -v
python3 tools/recovery-ledger-rollback-test.py
bash tools/worker-isolation-acceptance.sh
echo RECOVERY_LEDGER_ACCEPTANCE_PASS
