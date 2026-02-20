#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-Kimmio Launcher}"
OUT_DIR="${OUT_DIR:-dist}"
VERSION="${VERSION:-}"
TARGET_ARCH="${TARGET_ARCH:-arm64}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
APP_PATH="$ROOT_DIR/$OUT_DIR/${APP_NAME}.app"
DMG_ROOT="$ROOT_DIR/$OUT_DIR/.dmg-root"
DMG_NAME="Kimmio-Launcher-${VERSION}-macos-${TARGET_ARCH}.dmg"
DMG_PATH="$ROOT_DIR/$OUT_DIR/$DMG_NAME"

if [[ -z "$VERSION" ]]; then
  GIT_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
  TAG="$(git -C "$ROOT_DIR" describe --tags --exact-match 2>/dev/null || true)"
  if [[ -n "$TAG" ]]; then
    VERSION="${TAG#v}"
  else
    VERSION="0.0.0-${GIT_COMMIT}"
  fi
  DMG_NAME="Kimmio-Launcher-${VERSION}-macos-${TARGET_ARCH}.dmg"
  DMG_PATH="$ROOT_DIR/$OUT_DIR/$DMG_NAME"
fi

if ! command -v hdiutil >/dev/null 2>&1; then
  echo "hdiutil not found; skipping DMG build."
  exit 0
fi

if [[ ! -d "$APP_PATH" ]]; then
  echo "App bundle not found: $APP_PATH"
  echo "Build macOS app first: ./scripts/build-macos-app.sh"
  exit 1
fi

rm -rf "$DMG_ROOT"
mkdir -p "$DMG_ROOT"
cp -R "$APP_PATH" "$DMG_ROOT/"
ln -s /Applications "$DMG_ROOT/Applications"

echo "Building DMG..."
hdiutil create \
  -volname "$APP_NAME" \
  -srcfolder "$DMG_ROOT" \
  -ov -format UDZO \
  "$DMG_PATH" >/dev/null

rm -rf "$DMG_ROOT"
echo "DMG: $DMG_PATH"
