#!/usr/bin/env bash
# Focused acceptance only. It does not authorize a release or fleet activation.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${ADVANCED_REALITY_TEST_XRAY:?BLOCKED: exact test core path required}"
: "${BULK_TEST_DATABASE_URL:?BLOCKED: isolated PostgreSQL database required}"
: "${ARE_EVIDENCE_DIR:?BLOCKED: new private evidence directory required}"
if [[ -e "$ARE_EVIDENCE_DIR" ]]; then
  echo 'Refusing to replace existing evidence' >&2
  exit 2
fi
umask 077
mkdir "$ARE_EVIDENCE_DIR"
export GOMAXPROCS="${GOMAXPROCS:-2}"
python3 - <<'PY'
import hashlib, json, os, pathlib
r = pathlib.Path.cwd()
files = sorted(set(list(r.glob('internal/advancedreality/*.go')) +
    list(r.glob('internal/adminapi/advanced_reality*.go')) +
    list(r.glob('cmd/advanced-reality-lab/*.go')) +
    [r/'internal/adminapi/server.go', r/'web/static/app.js', r/'web/static/index.html',
     r/'web/static/advanced-reality.js', r/'tools/advanced-reality-acceptance.sh',
     r/'tools/advanced-reality-host-observe.py']))
before = {str(p.relative_to(r)): hashlib.sha256(p.read_bytes()).hexdigest() for p in files}
before['TEST_CORE_BINARY'] = hashlib.sha256(pathlib.Path(os.environ['ADVANCED_REALITY_TEST_XRAY']).read_bytes()).hexdigest()
client = os.environ.get('ADVANCED_REALITY_TEST_CLIENT_XRAY', os.environ['ADVANCED_REALITY_TEST_XRAY'])
before['TEST_CLIENT_BINARY'] = hashlib.sha256(pathlib.Path(client).read_bytes()).hexdigest()
pathlib.Path(os.environ['ARE_EVIDENCE_DIR'], 'source-before.json').write_text(json.dumps(before, indent=2)+'\n')
PY
go test -p 1 -race -count=1 -json ./internal/advancedreality > "$ARE_EVIDENCE_DIR/core-and-contract.jsonl"
go test -p 1 -race -count=1 -json -run '^TestAdvancedReality' ./internal/adminapi > "$ARE_EVIDENCE_DIR/admin.jsonl"
go vet ./internal/advancedreality ./internal/adminapi ./cmd/advanced-reality-lab
go build -p 1 -o "$ARE_EVIDENCE_DIR/advanced-reality-lab" ./cmd/advanced-reality-lab
node --check web/static/advanced-reality.js
node --check web/static/app.js
python3 - <<'PY'
import hashlib, json, os, pathlib, subprocess
r = pathlib.Path.cwd()
files = sorted(set(list(r.glob('internal/advancedreality/*.go')) +
    list(r.glob('internal/adminapi/advanced_reality*.go')) +
    list(r.glob('cmd/advanced-reality-lab/*.go')) +
    [r/'internal/adminapi/server.go', r/'web/static/app.js', r/'web/static/index.html',
     r/'web/static/advanced-reality.js', r/'tools/advanced-reality-acceptance.sh',
     r/'tools/advanced-reality-host-observe.py']))
hashes = {str(p.relative_to(r)): hashlib.sha256(p.read_bytes()).hexdigest() for p in files}
before = json.loads(pathlib.Path(os.environ['ARE_EVIDENCE_DIR'], 'source-before.json').read_text())
core_hash = hashlib.sha256(pathlib.Path(os.environ['ADVANCED_REALITY_TEST_XRAY']).read_bytes()).hexdigest()
client = os.environ.get('ADVANCED_REALITY_TEST_CLIENT_XRAY', os.environ['ADVANCED_REALITY_TEST_XRAY'])
client_hash = hashlib.sha256(pathlib.Path(client).read_bytes()).hexdigest()
if before != {**hashes, 'TEST_CORE_BINARY': core_hash, 'TEST_CLIENT_BINARY': client_hash}:
    raise SystemExit('FAIL: source or test binary changed during acceptance')
receipt = {'focused_acceptance': 'PASS', 'fleet_activation': 'BLOCKED',
           'phone_compatibility': 'BLOCKED', 'source_files': hashes,
           'core_binary_sha256': core_hash, 'client_binary_sha256': client_hash}
pathlib.Path(os.environ['ARE_EVIDENCE_DIR'], 'receipt.json').write_text(json.dumps(receipt, indent=2)+'\n')
print('PASS: focused lab acceptance; fleet and phone gates remain BLOCKED')
PY
