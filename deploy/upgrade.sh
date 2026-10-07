#!/usr/bin/env bash
set -Eeuo pipefail
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
[ "$(id -u)" -eq 0 ] || exit 1
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
APP=/opt/digital-ocean-bot
source "$SRC/deploy/service-topology.sh"
dob_preflight_service_topology
dob_acquire_deploy_lock
python3 "$SRC/deploy/bootstrap.py" "$ETC" unused unused "$(id -g digitaloceanbot)" --validate-only
BUILD_COMMIT="$(git -C "$SRC" rev-parse HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime=$BUILD_TIME"
dob_stage_runtime
dob_prepare_stage_permissions
dob_backup_runtime
trap dob_rollback_runtime ERR
dob_stop_runtime_readers
dob_publish_runtime
dob_install_service_topology
systemctl enable "${SERVICES[@]}"
systemctl start "${SERVICES[@]}"
dob_verify_service_topology
trap - ERR
echo "Deployment verified. Rollback artifacts: $DOB_BACKUP"
