#!/usr/bin/env bash
# packaging/macos/package.sh
#
# Assembles a signed, notarized .app bundle and .dmg for loom.
#
# Usage:
#   ./package.sh <binary-path> <arch> <version>
#   e.g. ./package.sh ../../dist/loom-darwin-arm64 arm64 0.2.0
#
# Required env vars:
#   APPLE_CERTIFICATE_P12       base64-encoded .p12 certificate
#   APPLE_CERTIFICATE_PASSWORD  password for the .p12
#   APPLE_APP_PASSWORD          app-specific password for notarytool
#
# Hardcoded (edit here if they change):
#   APPLE_DEVELOPER_ID, APPLE_TEAM_ID, APPLE_ID

set -euo pipefail

BINARY="${1:?usage: $0 <binary> <arch> <version>}"
ARCH="${2:?usage: $0 <binary> <arch> <version>}"
VERSION="${3:?usage: $0 <binary> <arch> <version>}"
VERSION="${VERSION#v}"  # strip leading 'v'

APPLE_DEVELOPER_ID="Developer ID Application: Kenneth Chau (S7LTAD7MEA)"
APPLE_TEAM_ID="S7LTAD7MEA"
APPLE_ID="ken@gizzar.com"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST_DIR="$(cd "$(dirname "$BINARY")" && pwd)"
APP_NAME="AgentDaemon"
APP_DIR="$DIST_DIR/$APP_NAME.app"
DMG_NAME="loom-darwin-$ARCH.dmg"
KEYCHAIN_NAME="loom-build.keychain"

# ── Import certificate into a temporary keychain ──────────────────────────────
echo "==> Importing certificate..."
echo "$APPLE_CERTIFICATE_P12" | base64 --decode > /tmp/cert.p12
security create-keychain -p "" "$KEYCHAIN_NAME" 2>/dev/null || true
security default-keychain -s "$KEYCHAIN_NAME"
security unlock-keychain -p "" "$KEYCHAIN_NAME"
security import /tmp/cert.p12 -k "$KEYCHAIN_NAME" \
    -P "$APPLE_CERTIFICATE_PASSWORD" \
    -T /usr/bin/codesign \
    -T /usr/bin/security
security set-key-partition-list -S apple-tool:,apple: -s -k "" "$KEYCHAIN_NAME"
rm -f /tmp/cert.p12

# ── Assemble .app bundle ──────────────────────────────────────────────────────
echo "==> Assembling $APP_NAME.app ($ARCH $VERSION)..."
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS"
mkdir -p "$APP_DIR/Contents/Resources"

cp "$BINARY" "$APP_DIR/Contents/MacOS/loom"
chmod +x "$APP_DIR/Contents/MacOS/loom"

sed "s/{{VERSION}}/$VERSION/g" "$SCRIPT_DIR/Info.plist" > "$APP_DIR/Contents/Info.plist"

# ── Code sign ─────────────────────────────────────────────────────────────────
echo "==> Signing..."
codesign \
    --deep \
    --force \
    --options runtime \
    --entitlements "$SCRIPT_DIR/entitlements.plist" \
    --sign "$APPLE_DEVELOPER_ID" \
    --timestamp \
    "$APP_DIR"

codesign --verify --deep --strict "$APP_DIR"
echo "    Signature OK"

# ── Notarize ──────────────────────────────────────────────────────────────────
echo "==> Notarizing (this takes a minute)..."
ditto -c -k --keepParent "$APP_DIR" /tmp/AgentDaemon-notarize.zip

xcrun notarytool submit /tmp/AgentDaemon-notarize.zip \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_APP_PASSWORD" \
    --team-id "$APPLE_TEAM_ID" \
    --wait \
    --timeout 10m

rm -f /tmp/AgentDaemon-notarize.zip

echo "==> Stapling..."
xcrun stapler staple "$APP_DIR"
xcrun stapler validate "$APP_DIR"
echo "    Staple OK"

# ── Create DMG ────────────────────────────────────────────────────────────────
echo "==> Creating DMG..."
STAGING=$(mktemp -d)
cp -R "$APP_DIR" "$STAGING/"
ln -s /Applications "$STAGING/Applications"

hdiutil create \
    -volname "Loom $VERSION" \
    -srcfolder "$STAGING" \
    -ov \
    -format UDZO \
    "$DIST_DIR/$DMG_NAME"

# Sign the DMG itself
codesign \
    --sign "$APPLE_DEVELOPER_ID" \
    --timestamp \
    "$DIST_DIR/$DMG_NAME"

rm -rf "$STAGING"
echo "==> Done: $DIST_DIR/$DMG_NAME"

# ── Cleanup keychain ──────────────────────────────────────────────────────────
security delete-keychain "$KEYCHAIN_NAME" 2>/dev/null || true
