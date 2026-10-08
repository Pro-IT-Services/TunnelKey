#!/usr/bin/env bash
# Builds the Tunnelkey Linux packages (.deb and .rpm) into desktop/build/out/.
#
# Requirements (Ubuntu 24.04 / Debian 13):
#   sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
#   Go (see desktop/go.mod), Node.js + npm, Wails CLI v2.12:
#     go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
#   nfpm (optional; otherwise run through "go run" at the pinned version).
#
# Environment:
#   GOARCH        target architecture (default: host, amd64 or arm64)
#   NFPM_VERSION  nfpm version used through "go run" (default below)
#   SKIP_GUI=1    reuse an existing build/bin/Tunnelkey instead of running wails
set -euo pipefail

NFPM_VERSION="${NFPM_VERSION:-v2.46.3}"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
desktop="$(dirname "$here")"
cd "$desktop"

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

command -v go >/dev/null || die "go not found"

version="$(sed -n 's/.*"productVersion"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' wails.json | head -n1)"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "wails.json info.productVersion '$version' must be x.y.z"
arch="${GOARCH:-$(go env GOARCH)}"
case "$arch" in
amd64 | arm64) ;;
*) die "unsupported GOARCH $arch" ;;
esac
log "Tunnelkey $version linux/$arch"

ldflags="-s -w -X main.version=$version"
mkdir -p build/bin build/out

if [[ "${SKIP_GUI:-0}" != "1" ]]; then
	command -v wails >/dev/null || die "wails not found (go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0)"
	pkg-config --exists webkit2gtk-4.1 || die "webkit2gtk-4.1 development files missing (apt-get install libwebkit2gtk-4.1-dev)"
	log "wails build (GUI)"
	# webkit2_41: link against WebKitGTK 4.1 (libsoup3), as on Ubuntu 24.04+.
	wails build -clean -platform "linux/$arch" -tags webkit2_41 -trimpath -ldflags "$ldflags"
	# vite empties frontend/dist; keep the committed placeholder for go:embed.
	touch frontend/dist/gitkeep
fi
[[ -x build/bin/Tunnelkey ]] || die "build/bin/Tunnelkey missing"

log "go build (tunnelkey-helper)"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
	go build -trimpath -ldflags "$ldflags" -o build/bin/tunnelkey-helper ./cmd/tunnelkey-helper

if command -v nfpm >/dev/null; then
	nfpm=(nfpm)
else
	nfpm=(go run "github.com/goreleaser/nfpm/v2/cmd/nfpm@${NFPM_VERSION}")
fi

export VERSION="$version" ARCH="$arch"
for packager in deb rpm; do
	log "nfpm $packager"
	"${nfpm[@]}" package --config build/linux/nfpm.yaml --packager "$packager" --target build/out/
done

log "done"
(cd build/out && ls -l ./*.deb ./*.rpm && sha256sum ./*.deb ./*.rpm | tee SHA256SUMS-linux.txt)
