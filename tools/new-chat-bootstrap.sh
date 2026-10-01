#!/usr/bin/env bash
set -euo pipefail
ROOT=/root/projects/Digital-ocean-bot-canonical-e2e
cd "$ROOT"
BRANCH=$(git branch --show-current)
[ "$BRANCH" = checkpoint/final-e2e-20260929 ] || { echo "BOOTSTRAP FAIL wrong branch: $BRANCH"; exit 1; }
python3 tools/continuity-finalization-gate.py
python3 tools/fresh-chat-mastery-gate.py
python3 - <<'PY'
import json,subprocess
p=json.load(open('docs/FRESH_SESSION_MASTERY_PROOF.json')); f=json.load(open('docs/CONTINUITY_FINAL_STATUS.json')); m=json.load(open('docs/MASTER_KNOWLEDGE_SCORE_V3.json'))
assert p['verified'] and p['score']==p['total']==40
assert f['ready'] and m['knowledge_ready'] and m['knowledge_mastery_percent']==100.0
print('NEW CHAT BOOTSTRAP READY')
print('fresh_session=40/40')
print('knowledge_mastery=100.0%')
print('head='+subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip())
print('next: read docs/NEW_CHAT_ENTRYPOINT.md then use deterministic retrieval before exact claims')
PY
