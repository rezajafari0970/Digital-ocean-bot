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
go test -race -p 1 ./internal/supervision ./internal/worker ./cmd/worker ./internal/provisioning ./internal/workflow ./internal/observability -count=1 -v
bash tools/recovery-ledger-acceptance.sh
python3 -m py_compile tools/phase3-supervision-process-fixture.py
# Execute the process fault contract before PASS. A frozen committed build is
# exercised again after checkpoint; both runs record the actual binary hash.
export DOB_ISOLATION_EVIDENCE_DIR=${DOB_ISOLATION_EVIDENCE_DIR:-/root/backups/dob-phase3-supervision-20261007/acceptance-process}
mkdir -p "$DOB_ISOLATION_EVIDENCE_DIR"
chmod 700 "$DOB_ISOLATION_EVIDENCE_DIR"
go build -trimpath -o "$DOB_ISOLATION_EVIDENCE_DIR/worker.fixture" ./cmd/worker
python3 tools/phase3-supervision-process-fixture.py
python3 - <<'CHECK'
import json,os,pathlib
r=json.loads((pathlib.Path(os.environ['DOB_ISOLATION_EVIDENCE_DIR'])/'phase3-systemd-fixture-result.json').read_text())
assert r['status']=='PASS' and r['peer_survived'] and r['panel_restart_ledger']['attempts']>=2
CHECK
echo PHASE3_SUPERVISION_ACCEPTANCE_PASS
