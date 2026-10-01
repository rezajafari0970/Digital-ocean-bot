#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
python3 tools/update-project-continuity.py
python3 tools/build-symbol-index.py
python3 tools/build-schema-index.py
python3 tools/build-test-map.py
python3 tools/build-runtime-map.py
python3 tools/build-source-knowledge.py
python3 tools/build-semantic-knowledge.py
python3 tools/build-impact-graph.py
python3 tools/build-handoff-bundle.py
python3 tools/audit-handoff-readiness.py
python3 -m json.tool docs/PROJECT_MANIFEST.json >/dev/null
python3 -m json.tool docs/CURRENT_SNAPSHOT.json >/dev/null
git add docs/PROJECT_MANIFEST.json docs/CURRENT_SNAPSHOT.json PROJECT_STATE.md HANDOFF.md docs/*.md tools/update-project-continuity.py tools/check-project-continuity.sh tools/checkpoint-project.sh tools/build-handoff-bundle.py tools/build-symbol-index.py tools/build-schema-index.py tools/build-test-map.py tools/build-runtime-map.py tools/build-source-knowledge.py tools/build-semantic-knowledge.py tools/build-impact-graph.py tools/source-lookup.py tools/project-brain-query.py tools/audit-handoff-readiness.py docs/SESSION_HANDOFF_BUNDLE.md docs/HANDOFF_READINESS_AUDIT.md
if git diff --cached --quiet; then echo 'continuity: no staged changes'; exit 0; fi
msg="${1:-docs: refresh project continuity checkpoint}"
git commit -m "$msg"
git push origin "$(git branch --show-current)"
python3 tools/update-project-continuity.py
# Keep generated snapshot aligned with the checkpoint commit itself.
git add docs/PROJECT_MANIFEST.json docs/CURRENT_SNAPSHOT.json
if ! git diff --cached --quiet; then
  git commit --amend --no-edit
  git push --force-with-lease origin "$(git branch --show-current)"
fi
echo "continuity checkpoint=$(git rev-parse HEAD)"
