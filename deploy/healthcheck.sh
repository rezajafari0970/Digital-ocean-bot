#!/usr/bin/env bash
set -euo pipefail
curl --fail --silent --show-error http://127.0.0.1:18080/healthz >/dev/null
curl --fail --silent --show-error http://127.0.0.1:18080/readyz >/dev/null
systemctl is-active --quiet digital-ocean-bot-api
systemctl is-active --quiet digital-ocean-bot-worker
