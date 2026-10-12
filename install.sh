#!/usr/bin/env bash
set -Eeuo pipefail
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export DEBIAN_FRONTEND=noninteractive
umask 077
REPOSITORY=https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config.git
MODE=install
SOURCE=
while (($#)); do
 case "$1" in
  --update) MODE=update ;;
  --source) shift; SOURCE=${1:?Missing source directory} ;;
  *) echo "Usage: install.sh [--update] [--source /absolute/trusted/source]" >&2; exit 64 ;;
 esac
 shift
done
[ "$(id -u)" = 0 ] || { echo 'Run with sudo or as root.' >&2; exit 1; }
[ -d /run/systemd/system ] || { echo 'A systemd Ubuntu server is required.' >&2; exit 1; }
. /etc/os-release
[ "$ID" = ubuntu ] || { echo 'Ubuntu is required.' >&2; exit 1; }
case "$VERSION_ID" in 22.04|24.04|26.04) ;; *) echo 'Supported Ubuntu LTS: 22.04, 24.04, 26.04.' >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|aarch64) ;; *) echo 'amd64 or arm64 is required.' >&2; exit 1 ;; esac
install -d -m 0755 /run/lock
exec 9>/run/lock/datacenter-bot-installer.lock
flock -n 9 || { echo 'Another installer/update is running.' >&2; exit 1; }
if [ "$MODE" = update ] && [ ! -f /etc/digital-ocean-bot/env ]; then
 echo 'No existing installation; run the install command first.' >&2; exit 1
fi
echo '[1/7] Checking prerequisites'
if ! command -v git >/dev/null || ! command -v curl >/dev/null; then
 apt-get -o DPkg::Lock::Timeout=180 update
 apt-get -o DPkg::Lock::Timeout=180 install -y --no-install-recommends git curl ca-certificates
fi
CACHE=/var/lib/datacenter-bot-installer
install -d -m 0700 "$CACHE" "$CACHE/revisions"
if [ -z "$SOURCE" ]; then
 echo '[2/7] Fetching the approved stable release'
 stage=$(mktemp -d "$CACHE/fetch.XXXXXXXX")
 trap 'rm -rf -- "${stage:-}"' EXIT
 git -c core.hooksPath=/dev/null clone --quiet --depth 1 --branch stable "$REPOSITORY" "$stage"
 revision=$(git -C "$stage" rev-parse --verify HEAD)
 [[ "$revision" =~ ^[0-9a-f]{40}$ ]] || exit 1
 SOURCE="$CACHE/revisions/$revision"
 if [ ! -e "$SOURCE" ]; then mv "$stage" "$SOURCE"; else
  test "$(git -C "$SOURCE" rev-parse HEAD)" = "$revision"
  test -z "$(git -C "$SOURCE" status --porcelain)"
 fi
else
 [[ "$SOURCE" = /* ]] || { echo 'Source must be an absolute trusted path.' >&2; exit 1; }
 SOURCE=$(realpath "$SOURCE")
 echo '[2/7] Using explicitly supplied local source'
fi
test -f "$SOURCE/deploy/unattended.sh"
bash "$SOURCE/deploy/unattended.sh" "$MODE"
