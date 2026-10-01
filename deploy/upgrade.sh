#!/usr/bin/env bash
set -euo pipefail
[ "$(id -u)" -eq 0 ] || exit 1
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
APP=/opt/digital-ocean-bot
BUILD_COMMIT="$(git -C "$SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime=$BUILD_TIME"
systemctl stop digital-ocean-bot-api digital-ocean-bot-worker || true
cp -a "$SRC/migrations" "$APP/"
cd "$SRC"
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/digital-ocean-bot-api.new" ./cmd/api
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/digital-ocean-bot-worker.new" ./cmd/worker
mv "$APP/bin/digital-ocean-bot-api.new" "$APP/bin/digital-ocean-bot-api"
mv "$APP/bin/digital-ocean-bot-worker.new" "$APP/bin/digital-ocean-bot-worker"
chown root:digitaloceanbot "$APP/bin/"*; chmod 0750 "$APP/bin/"*
API_SHA="$(sha256sum "$APP/bin/digital-ocean-bot-api" | awk '{print $1}')"
WORKER_SHA="$(sha256sum "$APP/bin/digital-ocean-bot-worker" | awk '{print $1}')"
printf '{"commit":"%s","build_time":"%s","api_sha256":"%s","worker_sha256":"%s"}\n' "$BUILD_COMMIT" "$BUILD_TIME" "$API_SHA" "$WORKER_SHA" > "$APP/build-manifest.json"
systemctl start digital-ocean-bot-api digital-ocean-bot-worker
