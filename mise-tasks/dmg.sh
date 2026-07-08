#!/usr/bin/env bash

#MISE description="Build a universal macOS .app bundle and .dmg installer"

# Produces dist/macos/<project>_<version>_macos_universal.dmg containing a
# drag-to-Applications "NATS Desktop.app". Requires the macOS toolchain
# (clang, lipo, sips, iconutil, hdiutil), so it must run on macOS.

set -euo pipefail

APP_NAME="NATS Desktop"
BINARY_NAME="nats-desktop"
BUNDLE_ID="com.thedataflows.nats-desktop"
MAIN_PKG="./cmd/nats-desktop"
ICON_SRC="assets/icons/nats-plain-512px.png"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Version mirrors the goreleaser ldflag format: <tag>_<short-commit>.
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
SHORT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_VERSION="${VERSION}_${SHORT_COMMIT}"

# CFBundleShortVersionString must be dot-separated numbers; derive it from the
# tag (strip a leading "v" and any pre-release/build suffix), defaulting to 0.0.0.
SHORT_VERSION="$(printf '%s' "$VERSION" | sed -E 's/^v//; s/[^0-9.].*$//')"
[ -z "$SHORT_VERSION" ] && SHORT_VERSION="0.0.0"

DIST="dist/macos"
APP_DIR="$DIST/$APP_NAME.app"
DMG_PATH="$DIST/${BINARY_NAME}_${VERSION}_macos_universal.dmg"

rm -rf "$DIST"
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"

echo "==> Building universal binary ($BUILD_VERSION)"
build_slice() {
	local arch="$1" cc="$2" min="$3" out="$4"
	echo "  - darwin/$arch (min ${min})"
	CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="$cc" \
		MACOSX_DEPLOYMENT_TARGET="$min" \
		go build -trimpath \
		-ldflags "-s -w -X main.VERSION=${BUILD_VERSION}" \
		-o "$out" "$MAIN_PKG"
}

# arm64 macOS only exists from 11.0; amd64 keeps the 10.13 floor.
build_slice arm64 "clang -arch arm64" 11.0 "$DIST/${BINARY_NAME}-arm64"
build_slice amd64 "clang -arch x86_64" 10.13 "$DIST/${BINARY_NAME}-amd64"

lipo -create -output "$APP_DIR/Contents/MacOS/$BINARY_NAME" \
	"$DIST/${BINARY_NAME}-arm64" "$DIST/${BINARY_NAME}-amd64"
rm -f "$DIST/${BINARY_NAME}-arm64" "$DIST/${BINARY_NAME}-amd64"
chmod +x "$APP_DIR/Contents/MacOS/$BINARY_NAME"

echo "==> Generating icon.icns"
ICONSET="$DIST/icon.iconset"
mkdir -p "$ICONSET"
gen() { sips -z "$1" "$1" "$ICON_SRC" --out "$ICONSET/$2" >/dev/null; }
gen 16 icon_16x16.png
gen 32 icon_16x16@2x.png
gen 32 icon_32x32.png
gen 64 icon_32x32@2x.png
gen 128 icon_128x128.png
gen 256 icon_128x128@2x.png
gen 256 icon_256x256.png
gen 512 icon_256x256@2x.png
gen 512 icon_512x512.png
gen 1024 icon_512x512@2x.png
iconutil -c icns "$ICONSET" -o "$APP_DIR/Contents/Resources/icon.icns"
rm -rf "$ICONSET"

echo "==> Writing Info.plist"
cat >"$APP_DIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>${APP_NAME}</string>
	<key>CFBundleDisplayName</key>
	<string>${APP_NAME}</string>
	<key>CFBundleExecutable</key>
	<string>${BINARY_NAME}</string>
	<key>CFBundleIdentifier</key>
	<string>${BUNDLE_ID}</string>
	<key>CFBundleIconFile</key>
	<string>icon</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleShortVersionString</key>
	<string>${SHORT_VERSION}</string>
	<key>CFBundleVersion</key>
	<string>${SHORT_VERSION}</string>
	<key>LSMinimumSystemVersion</key>
	<string>10.13</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

echo "==> Building DMG"
STAGING="$DIST/dmg"
rm -rf "$STAGING"
mkdir -p "$STAGING"
cp -R "$APP_DIR" "$STAGING/"
ln -s /Applications "$STAGING/Applications"

rm -f "$DMG_PATH"
hdiutil create \
	-volname "$APP_NAME" \
	-srcfolder "$STAGING" \
	-fs HFS+ \
	-format UDZO \
	-ov \
	"$DMG_PATH" >/dev/null
rm -rf "$STAGING"

echo "==> Done: $DMG_PATH"
