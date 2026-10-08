#!/usr/bin/env bash
# Builds Tunnelkey-<ver>.pkg for macOS 12+ (Apple silicon and Intel).
#
# Steps:
#   1. wails build            -> build/bin/Tunnelkey.app (universal by default)
#   2. go build + lipo        -> tunnelkey-helper (universal)
#   3. OpenVPN from the official source tarball, statically linked against
#      OpenSSL 3 (LTS), LZ4 and LZO built from source; one binary per arch,
#      merged with lipo. No Homebrew or other non-system dylibs (checked
#      with otool -L).
#   4. pkgbuild + productbuild -> build/out/Tunnelkey-<ver>.pkg
#
# Requirements: Xcode command line tools, Go (desktop/go.mod), Node.js + npm,
# Wails CLI v2.12 (go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0).
#
# Environment:
#   ARCHS="arm64 x86_64"  architectures to build (default both)
#   SKIP_GUI=1            reuse an existing build/bin/Tunnelkey.app
#   Signing / notarization (all optional; skipped when unset):
#   APPLE_APP_IDENTITY        "Developer ID Application: ProIT services (TEAMID)"
#   APPLE_INSTALLER_IDENTITY  "Developer ID Installer: ProIT services (TEAMID)"
#   APPLE_NOTARY_PROFILE      notarytool keychain profile, or
#   APPLE_ID + APPLE_TEAM_ID + APPLE_APP_PASSWORD  notarytool credentials
set -euo pipefail

# --- pinned third-party sources ----------------------------------------------
OPENVPN_VERSION=2.6.23
OPENVPN_URL="https://build.openvpn.net/downloads/releases/openvpn-${OPENVPN_VERSION}.tar.gz"
OPENVPN_SHA256=4041c709162bec1325abf5aa8cf27a255cc477c634b15ee310411c701fc40a96

OPENSSL_VERSION=3.5.9
OPENSSL_URL="https://github.com/openssl/openssl/releases/download/openssl-${OPENSSL_VERSION}/openssl-${OPENSSL_VERSION}.tar.gz"
OPENSSL_SHA256=603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a

LZ4_VERSION=1.10.0
LZ4_URL="https://github.com/lz4/lz4/releases/download/v${LZ4_VERSION}/lz4-${LZ4_VERSION}.tar.gz"
LZ4_SHA256=537512904744b35e232912055ccf8ec66d768639ff3abe5788d90d792ec5f48b

LZO_VERSION=2.10
LZO_URL="https://www.oberhumer.com/opensource/lzo/download/lzo-${LZO_VERSION}.tar.gz"
LZO_SHA256=c0f892943208266f9b6543b3ae308fab6284c5c90e627931446fb49b4221a072

export MACOSX_DEPLOYMENT_TARGET=12.0

# --- paths -------------------------------------------------------------------
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
desktop="$(dirname "$here")"
repo="$(dirname "$desktop")"
darwin="$desktop/build/darwin"
cache="$desktop/build/cache/macos"
work="$cache/work"
out="$desktop/build/out"

LABEL=app.tunnelkey.helper
PKG_ID=com.proitservices.tunnelkey.pkg
OVPN_PAYLOAD_DIR="Library/Application Support/Tunnelkey/openvpn"

read -r -a archs <<<"${ARCHS:-arm64 x86_64}"
native_arch="$(uname -m)"
jobs="$(sysctl -n hw.ncpu)"

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[[ "$(uname -s)" == "Darwin" ]] || die "build-macos.sh must run on macOS"
for tool in go clang lipo otool pkgbuild productbuild shasum curl; do
	command -v "$tool" >/dev/null || die "$tool not found"
done

version="$(sed -n 's/.*"productVersion"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$desktop/wails.json" | head -n1)"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "wails.json info.productVersion '$version' must be x.y.z"
log "Tunnelkey $version macOS (${archs[*]})"

mkdir -p "$cache" "$work" "$out" "$desktop/build/bin"

# fetch URL FILE SHA256: download into the cache and verify.
fetch() {
	local url="$1" file="$cache/$2" sha="$3"
	if [[ -f "$file" ]] && [[ "$(shasum -a 256 "$file" | cut -d' ' -f1)" == "$sha" ]]; then
		return 0
	fi
	log "downloading $url"
	curl -fL --retry 3 -o "$file.part" "$url"
	mv "$file.part" "$file"
	local got
	got="$(shasum -a 256 "$file" | cut -d' ' -f1)"
	[[ "$got" == "$sha" ]] || die "SHA-256 mismatch for $2: got $got, want $sha"
}

# triple ARCH: autoconf host triple.
triple() {
	case "$1" in
	arm64) echo aarch64-apple-darwin ;;
	x86_64) echo x86_64-apple-darwin ;;
	*) die "unsupported arch $1" ;;
	esac
}

# goarch ARCH: Go architecture name.
goarch() {
	case "$1" in
	arm64) echo arm64 ;;
	x86_64) echo amd64 ;;
	*) die "unsupported arch $1" ;;
	esac
}

# host_args ARCH: --host only when cross compiling (keeps configure run tests
# enabled for the native architecture).
host_args() {
	if [[ "$1" != "$native_arch" ]]; then
		echo "--host=$(triple "$1")"
	fi
}

# extract TARBALL DEST: fresh copy of a source tree.
extract() {
	rm -rf "$2"
	mkdir -p "$2"
	tar -xzf "$cache/$1" -C "$2" --strip-components=1
}

# build_openvpn ARCH: static openvpn for one architecture -> $work/$arch/openvpn
build_openvpn() {
	local arch="$1"
	local dir="$work/$arch"
	local prefix="$dir/prefix"
	local cflags="-arch $arch -mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET -O2"
	local host
	host="$(host_args "$arch")"
	rm -rf "$dir"
	mkdir -p "$prefix/include" "$prefix/lib"

	# Keep Homebrew/MacPorts out of every configure step.
	local -a cleanenv=(env -u PKG_CONFIG_PATH -u CPATH -u LIBRARY_PATH -u C_INCLUDE_PATH
		PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig" PATH="/usr/bin:/bin:/usr/sbin:/sbin")

	log "[$arch] OpenSSL $OPENSSL_VERSION (static)"
	extract "openssl-$OPENSSL_VERSION.tar.gz" "$dir/openssl"
	local ossl_target=darwin64-arm64-cc
	[[ "$arch" == "x86_64" ]] && ossl_target=darwin64-x86_64-cc
	(
		cd "$dir/openssl"
		"${cleanenv[@]}" ./Configure "$ossl_target" no-shared no-tests no-docs no-module \
			--prefix="$prefix" --libdir=lib --openssldir=/private/etc/ssl \
			"-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET"
		"${cleanenv[@]}" make -j"$jobs" build_libs
		"${cleanenv[@]}" make install_dev
	)

	log "[$arch] LZ4 $LZ4_VERSION (static)"
	extract "lz4-$LZ4_VERSION.tar.gz" "$dir/lz4"
	(
		cd "$dir/lz4"
		"${cleanenv[@]}" CC=clang CFLAGS="$cflags" make -C lib -j"$jobs" liblz4.a
		cp lib/liblz4.a "$prefix/lib/"
		cp lib/lz4.h lib/lz4hc.h lib/lz4frame.h "$prefix/include/"
	)

	log "[$arch] LZO $LZO_VERSION (static)"
	extract "lzo-$LZO_VERSION.tar.gz" "$dir/lzo"
	(
		cd "$dir/lzo"
		# shellcheck disable=SC2086 # $host is empty or a single word
		"${cleanenv[@]}" ./configure $host --prefix="$prefix" --disable-shared --enable-static \
			CC=clang CFLAGS="$cflags"
		"${cleanenv[@]}" make -j"$jobs"
		"${cleanenv[@]}" make install
	)
	# Only static libraries may be found by the openvpn link step.
	find "$prefix/lib" -name '*.dylib' -delete

	log "[$arch] OpenVPN $OPENVPN_VERSION"
	extract "openvpn-$OPENVPN_VERSION.tar.gz" "$dir/openvpn-src"
	(
		cd "$dir/openvpn-src"
		# shellcheck disable=SC2086 # $host is empty or a single word
		"${cleanenv[@]}" ./configure $host \
			--disable-dependency-tracking \
			--disable-plugin-auth-pam \
			--disable-plugin-down-root \
			--disable-unit-tests \
			--with-crypto-library=openssl \
			CC=clang CFLAGS="$cflags" \
			OPENSSL_CFLAGS="-I$prefix/include" OPENSSL_LIBS="-L$prefix/lib -lssl -lcrypto" \
			LZ4_CFLAGS="-I$prefix/include" LZ4_LIBS="-L$prefix/lib -llz4" \
			LZO_CFLAGS="-I$prefix/include" LZO_LIBS="-L$prefix/lib -llzo2"
		"${cleanenv[@]}" make -j"$jobs"
	)
	cp "$dir/openvpn-src/src/openvpn/openvpn" "$dir/openvpn"
	strip -x "$dir/openvpn"
}

# check_linkage BINARY: only system libraries may be referenced.
check_linkage() {
	local bin="$1" arch libs bad
	for arch in $(lipo -archs "$bin"); do
		libs="$(otool -arch "$arch" -L "$bin" | tail -n +2 | awk '{print $1}')"
		printf '    %s [%s]:\n%s\n' "$(basename "$bin")" "$arch" "$(printf '%s\n' "$libs" | sed 's/^/      /')"
		bad="$(printf '%s\n' "$libs" | grep -Ev '^(/usr/lib/|/System/Library/)' || true)"
		[[ -z "$bad" ]] || die "$(basename "$bin") [$arch] links non-system libraries: $bad"
	done
}

# --- 1. GUI ------------------------------------------------------------------
app="$desktop/build/bin/Tunnelkey.app"
ldflags="-s -w -X main.version=$version"
if [[ "${SKIP_GUI:-0}" != "1" ]]; then
	command -v wails >/dev/null || die "wails not found (go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0)"
	if [[ ${#archs[@]} -gt 1 ]]; then
		platform=darwin/universal
	else
		platform="darwin/$(goarch "${archs[0]}")"
	fi
	log "wails build ($platform)"
	(cd "$desktop" && wails build -clean -platform "$platform" -trimpath -ldflags "$ldflags")
	# vite empties frontend/dist; keep the committed placeholder for go:embed.
	touch "$desktop/frontend/dist/gitkeep"
fi
[[ -d "$app" ]] || die "$app missing"

# --- 2. helper ---------------------------------------------------------------
log "go build (tunnelkey-helper)"
helper="$work/$LABEL"
helper_parts=()
for arch in "${archs[@]}"; do
	part="$work/tunnelkey-helper-$arch"
	(cd "$desktop" && CGO_ENABLED=0 GOOS=darwin GOARCH="$(goarch "$arch")" \
		go build -trimpath -ldflags "$ldflags" -o "$part" ./cmd/tunnelkey-helper)
	helper_parts+=("$part")
done
lipo -create "${helper_parts[@]}" -output "$helper"

# --- 3. OpenVPN --------------------------------------------------------------
fetch "$OPENVPN_URL" "openvpn-$OPENVPN_VERSION.tar.gz" "$OPENVPN_SHA256"
fetch "$OPENSSL_URL" "openssl-$OPENSSL_VERSION.tar.gz" "$OPENSSL_SHA256"
fetch "$LZ4_URL" "lz4-$LZ4_VERSION.tar.gz" "$LZ4_SHA256"
fetch "$LZO_URL" "lzo-$LZO_VERSION.tar.gz" "$LZO_SHA256"

ovpn_parts=()
for arch in "${archs[@]}"; do
	build_openvpn "$arch"
	ovpn_parts+=("$work/$arch/openvpn")
done
openvpn_bin="$work/openvpn"
lipo -create "${ovpn_parts[@]}" -output "$openvpn_bin"

log "checking linkage"
check_linkage "$openvpn_bin"
check_linkage "$helper"
"$openvpn_bin" --version | head -n 2 || true

# --- 4. payload ----------------------------------------------------------------
stage="$work/stage"
root="$stage/root"
rm -rf "$stage"
mkdir -p "$root/Applications" "$root/Library/PrivilegedHelperTools" "$root/Library/LaunchDaemons" \
	"$root/$OVPN_PAYLOAD_DIR/licenses"

ditto "$app" "$root/Applications/Tunnelkey.app"
install -m 0755 "$helper" "$root/Library/PrivilegedHelperTools/$LABEL"
install -m 0644 "$darwin/$LABEL.plist" "$root/Library/LaunchDaemons/$LABEL.plist"
install -m 0755 "$openvpn_bin" "$root/$OVPN_PAYLOAD_DIR/openvpn"
install -m 0755 "$darwin/uninstall.sh" "$root/Library/Application Support/Tunnelkey/uninstall.sh"
# License texts of the bundled, statically linked components.
install -m 0644 "$work/${archs[0]}/openvpn-src/COPYING" "$root/$OVPN_PAYLOAD_DIR/licenses/openvpn-COPYING.txt"
install -m 0644 "$work/${archs[0]}/openvpn-src/COPYRIGHT.GPL" "$root/$OVPN_PAYLOAD_DIR/licenses/openvpn-COPYRIGHT.GPL.txt"
install -m 0644 "$work/${archs[0]}/openssl/LICENSE.txt" "$root/$OVPN_PAYLOAD_DIR/licenses/openssl-LICENSE.txt"
install -m 0644 "$work/${archs[0]}/lz4/lib/LICENSE" "$root/$OVPN_PAYLOAD_DIR/licenses/lz4-LICENSE.txt"
install -m 0644 "$work/${archs[0]}/lzo/COPYING" "$root/$OVPN_PAYLOAD_DIR/licenses/lzo-COPYING.txt"
printf 'OpenVPN %s, OpenSSL %s, LZ4 %s, LZO %s\nSources: %s\n %s\n %s\n %s\n' \
	"$OPENVPN_VERSION" "$OPENSSL_VERSION" "$LZ4_VERSION" "$LZO_VERSION" \
	"$OPENVPN_URL" "$OPENSSL_URL" "$LZ4_URL" "$LZO_URL" >"$root/$OVPN_PAYLOAD_DIR/licenses/SOURCES.txt"

# --- signing (optional) --------------------------------------------------------
if [[ -n "${APPLE_APP_IDENTITY:-}" ]]; then
	log "codesign ($APPLE_APP_IDENTITY)"
	sign() { codesign --force --timestamp --options runtime --sign "$APPLE_APP_IDENTITY" "$@"; }
	sign --identifier net.openvpn.openvpn "$root/$OVPN_PAYLOAD_DIR/openvpn"
	sign --identifier "$LABEL" "$root/Library/PrivilegedHelperTools/$LABEL"
	sign "$root/Applications/Tunnelkey.app"
	codesign --verify --strict --verbose=2 "$root/Applications/Tunnelkey.app"
else
	log "APPLE_APP_IDENTITY not set: binaries are not signed"
fi

# --- 5. packages -------------------------------------------------------------
scripts="$stage/scripts"
mkdir -p "$scripts"
install -m 0755 "$darwin/pkg/scripts/preinstall" "$scripts/preinstall"
install -m 0755 "$darwin/pkg/scripts/postinstall" "$scripts/postinstall"

# The app must land in /Applications, never be "relocated" to another copy
# with the same bundle id.
component_plist="$stage/component.plist"
pkgbuild --analyze --root "$root" "$component_plist"
i=0
while /usr/libexec/PlistBuddy -c "Print :$i" "$component_plist" >/dev/null 2>&1; do
	/usr/libexec/PlistBuddy -c "Set :$i:BundleIsRelocatable false" "$component_plist"
	i=$((i + 1))
done

component_pkg="$stage/Tunnelkey-component.pkg"
pkgbuild --root "$root" \
	--component-plist "$component_plist" \
	--scripts "$scripts" \
	--identifier "$PKG_ID" \
	--version "$version" \
	--install-location / \
	"$component_pkg"

resources="$stage/resources"
mkdir -p "$resources"
cp "$repo/LICENSE" "$resources/LICENSE.txt"
sed -e "s/@VERSION@/$version/" -e "s/@COMPONENT_PKG@/$(basename "$component_pkg")/" \
	"$darwin/pkg/distribution.xml" >"$stage/distribution.xml"

pkg="$out/Tunnelkey-$version.pkg"
productbuild_args=(--distribution "$stage/distribution.xml" --resources "$resources" --package-path "$stage")
if [[ -n "${APPLE_INSTALLER_IDENTITY:-}" ]]; then
	productbuild_args+=(--sign "$APPLE_INSTALLER_IDENTITY" --timestamp)
fi
log "productbuild -> $(basename "$pkg")"
productbuild "${productbuild_args[@]}" "$pkg"

# --- notarization (optional) -------------------------------------------------
if [[ -n "${APPLE_INSTALLER_IDENTITY:-}" ]]; then
	if [[ -n "${APPLE_NOTARY_PROFILE:-}" ]]; then
		log "notarizing (keychain profile)"
		xcrun notarytool submit "$pkg" --keychain-profile "$APPLE_NOTARY_PROFILE" --wait
		xcrun stapler staple "$pkg"
	elif [[ -n "${APPLE_ID:-}" && -n "${APPLE_TEAM_ID:-}" && -n "${APPLE_APP_PASSWORD:-}" ]]; then
		log "notarizing (Apple ID)"
		xcrun notarytool submit "$pkg" --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" \
			--password "$APPLE_APP_PASSWORD" --wait
		xcrun stapler staple "$pkg"
	else
		log "no notarization credentials: skipping notarization"
	fi
fi

log "done"
ls -l "$pkg"
shasum -a 256 "$pkg" | tee "$out/SHA256SUMS-macos.txt"
