#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOTMPDIR=/root/buildtmp/dob-modular-recovery
export DOB_ISOLATION_EVIDENCE_DIR=/root/backups/dob-phase123-remediation-20261007/acceptance-process
bash -n deploy/install.sh deploy/upgrade.sh deploy/service-topology.sh
python3 deploy/bootstrap_test.py
python3 tools/phase123-deploy-fixture.py > /root/backups/dob-phase123-remediation-20261007/deploy-fixture.json
bash tools/phase3-supervision-acceptance.sh
echo PHASE123_REMEDIATION_ACCEPTANCE_PASS
