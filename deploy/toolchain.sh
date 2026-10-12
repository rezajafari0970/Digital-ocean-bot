#!/usr/bin/env bash
dob_install_toolchain() {
 local arch digest archive dir
 case "$(uname -m)" in
  x86_64) arch=amd64; digest=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445 ;;
  aarch64) arch=arm64; digest=3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec ;;
  *) return 1 ;;
 esac
 dir=/usr/local/lib/datacenter-bot/go1.27.1
 if [ ! -x "$dir/bin/go" ]; then
  install -d -m 0755 /usr/local/lib/datacenter-bot
  archive=$(mktemp /usr/local/lib/datacenter-bot/go.XXXXXXXX)
  if ! curl --fail --silent --show-error --location --retry 3 --connect-timeout 20 --max-time 600 "https://go.dev/dl/go1.27.1.linux-$arch.tar.gz" -o "$archive"; then rm -f "$archive"; return 1; fi
  if ! printf '%s  %s\n' "$digest" "$archive" | sha256sum --check --status; then rm -f "$archive"; echo 'Go archive checksum mismatch' >&2; return 1; fi
  local unpack
  unpack=$(mktemp -d /usr/local/lib/datacenter-bot/go-unpack.XXXXXXXX)
  tar -xzf "$archive" -C "$unpack"
  mv "$unpack/go" "$dir"
  rm -rf -- "$unpack" "$archive"
 fi
 test "$("$dir/bin/go" version)" = "go version go1.27.1 linux/$arch"
 export PATH="$dir/bin:$PATH"
 export GOTOOLCHAIN=local CGO_ENABLED=0 GOMAXPROCS=2
 export GOCACHE=/var/cache/datacenter-bot/go-build GOPATH=/var/cache/datacenter-bot/go
 install -d -m 0700 "$GOCACHE" "$GOPATH"
}
