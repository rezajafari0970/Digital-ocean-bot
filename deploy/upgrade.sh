#!/usr/bin/env bash
set -euo pipefail
[ "$(id -u)" -eq 0 ] || exit 1
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
APP=/opt/digital-ocean-bot
systemctl stop digital-ocean-bot-api digital-ocean-bot-worker || true
cp -a "$SRC/migrations" "$APP/"
cd "$SRC"
go build -trimpath -ldflags='-s -w' -o "$APP/bin/digital-ocean-bot-api.new" ./cmd/api
go build -trimpath -ldflags='-s -w' -o "$APP/bin/digital-ocean-bot-worker.new" ./cmd/worker
mv "$APP/bin/digital-ocean-bot-api.new" "$APP/bin/digital-ocean-bot-api"
mv "$APP/bin/digital-ocean-bot-worker.new" "$APP/bin/digital-ocean-bot-worker"
chown root:digitaloceanbot "$APP/bin/"*; chmod 0750 "$APP/bin/"*
systemctl start digital-ocean-bot-api digital-ocean-bot-worker
