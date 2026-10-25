#!/usr/bin/env bash
# Operator launcher: keep installer/build descendants separate from live services.
set -euo pipefail
if (( EUID != 0 )); then echo 'Run as root.' >&2; exit 1; fi
if (( $# == 0 )); then echo 'Usage: run-bounded-update.sh /absolute/update-command [arguments]' >&2; exit 64; fi
[[ "$1" = /* && -x "$1" ]] || { echo 'An absolute executable is required.' >&2; exit 64; }
exec systemd-run --quiet --wait --pipe --collect \
  --unit="dob-bounded-update-$(date -u +%Y%m%dT%H%M%S)-$$" \
  -p MemoryMax=1G -p MemoryHigh=768M -p CPUQuota=100% \
  /usr/bin/env GOMEMLIMIT=384MiB GOMAXPROCS=2 GOTOOLCHAIN=local GOFLAGS=-p=1 "$@"
