#!/usr/bin/env bash
# Characterization only: PASS means the recorded defect was reproduced.
# Never source production credentials here. Caller must supply a disposable bulk_test DB.
set -euo pipefail
: "${BULK_TEST_DATABASE_URL:?Supply an isolated bulk_test PostgreSQL URL}"
python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ['BULK_TEST_DATABASE_URL'])
assert u.scheme in ('postgres','postgresql') and 'bulk_test' in u.path, 'isolated bulk_test DB required'
PY
ROOT=$(cd "$(dirname "$0")/.." && pwd)
GO_BIN=${GO_BIN:-/usr/local/go/bin/go}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/dob-phase123-audit.XXXXXX")
WORK="$TMP/worktree"
cleanup() {
  git -C "$ROOT" worktree remove --force "$WORK" >/dev/null 2>&1 || true
  rm -rf -- "$TMP"
}
trap cleanup EXIT
BASELINE=337b6cf44bc09939e1371b9a5607f7d086e5584d
git -C "$ROOT" worktree add --detach "$WORK" "$BASELINE" >/dev/null
for pkg in internal/worker cmd/worker internal/workflow; do
  name=${pkg//\//_}
  cp "$ROOT/tools/phase123-audit-fixtures/$name.go.txt" "$WORK/$pkg/audit_phase123_test.go"
done
cd "$WORK"
"$GO_BIN" test -race -p 1 ./internal/worker ./cmd/worker ./internal/workflow -run TestAudit -count=1 -v
echo 'AUDIT_CHARACTERIZATION_PASS: all eight recorded behaviors reproduced; this is not a product fix gate.'
