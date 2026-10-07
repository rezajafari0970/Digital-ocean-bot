#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
[ "$(id -u)" -eq 0 ] || exit 1
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
APP=/opt/digital-ocean-bot
SERVICES=(digital-ocean-bot-vultr-browser-manager digital-ocean-bot-api digital-ocean-bot-worker)
if systemctl is-enabled --quiet digital-ocean-bot-worker-panels 2>/dev/null || systemctl is-active --quiet digital-ocean-bot-worker-panels 2>/dev/null; then
  SERVICES+=(digital-ocean-bot-worker-panels)
fi
BUILD_COMMIT="$(git -C "$SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime=$BUILD_TIME"
cd "$SRC"
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/digital-ocean-bot-api.new" ./cmd/api
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/digital-ocean-bot-worker.new" ./cmd/worker
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/vultr-browser-session.new" ./cmd/vultr-browser-session
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/vultr-browser-manager.new" ./cmd/vultr-browser-manager
go build -trimpath -ldflags="$LDFLAGS" -o "$APP/bin/vultr-input-bridge.new" ./cmd/vultr-input-bridge
# Finish all builds and verify service-readable migrations before stopping services.
install -d -o root -g digitaloceanbot -m 0750 "$APP/migrations"
install -o root -g digitaloceanbot -m 0640 "$SRC/migrations/"*.sql "$APP/migrations/"
for migration in "$APP/migrations/"*.sql; do runuser -u digitaloceanbot -- test -r "$migration"; done
systemctl stop "${SERVICES[@]}"
mv "$APP/bin/digital-ocean-bot-api.new" "$APP/bin/digital-ocean-bot-api"
mv "$APP/bin/digital-ocean-bot-worker.new" "$APP/bin/digital-ocean-bot-worker"
mv "$APP/bin/vultr-browser-session.new" "$APP/bin/vultr-browser-session"
mv "$APP/bin/vultr-browser-manager.new" "$APP/bin/vultr-browser-manager"
mv "$APP/bin/vultr-input-bridge.new" "$APP/bin/vultr-input-bridge"
chown root:digitaloceanbot "$APP/bin/"*; chmod 0750 "$APP/bin/"*
API_SHA="$(sha256sum "$APP/bin/digital-ocean-bot-api" | awk '{print $1}')"
WORKER_SHA="$(sha256sum "$APP/bin/digital-ocean-bot-worker" | awk '{print $1}')"
printf '{"commit":"%s","build_time":"%s","api_sha256":"%s","worker_sha256":"%s"}\n' "$BUILD_COMMIT" "$BUILD_TIME" "$API_SHA" "$WORKER_SHA" > "$APP/build-manifest.json"
systemctl start "${SERVICES[@]}"
