#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/digital-ocean-bot}"
REPO="${REPO:-git@github.com:rezajafari0970/Digital-ocean-bot.git}"

command -v git >/dev/null
command -v go >/dev/null

mkdir -p "$APP_DIR"
echo "bootstrap target: $APP_DIR"
echo "repository: $REPO"
echo "bootstrap prerequisites verified"
