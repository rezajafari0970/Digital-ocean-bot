#!/usr/bin/env bash
set -Eeuo pipefail
APP=/opt/digital-ocean-bot
ETC=/etc/digital-ocean-bot
source "$SRC/deploy/service-topology.sh"
dob_acquire_deploy_lock
DOB_PENDING="$ETC/deployment.pending"
trap dob_rollback_runtime ERR
false
