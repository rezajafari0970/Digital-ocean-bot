#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
python3 -m json.tool docs/PROJECT_MANIFEST.json >/dev/null
printf 'branch=%s\n' "$(git branch --show-current)"
printf 'head=%s\n' "$(git rev-parse HEAD)"
printf 'manifest_head=%s\n' "$(python3 -c 'import json;print(json.load(open("docs/PROJECT_MANIFEST.json"))["head_at_generation"])')"
printf 'up_migrations=%s\n' "$(find migrations -name '*.up.sql' | wc -l)"
printf 'go_files=%s\n' "$(find cmd internal -name '*.go' | wc -l)"
printf 'tests=%s\n' "$(find internal -name '*_test.go' | wc -l)"
printf 'api=%s worker=%s\n' "$(systemctl is-active digital-ocean-bot-api 2>/dev/null || true)" "$(systemctl is-active digital-ocean-bot-worker 2>/dev/null || true)"
git status --short --branch
