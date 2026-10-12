#!/usr/bin/env bash
set -Eeuo pipefail
# All entry points use the same backup, retention and rollback boundary.
SRC="${SRC:-$(cd "$(dirname "$0")/.." && pwd)}"
exec bash "$SRC/install.sh" --update --source "$SRC"
