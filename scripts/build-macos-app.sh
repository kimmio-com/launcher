#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-Kimmio Launcher}"
BUNDLE_ID="${BUNDLE_ID:-com.kimmio.launcher}"
VERSION="${VERSION:-}"
BIN_NAME="${BIN_NAME:-launcher}"
APP_EXEC_NAME="${APP_EXEC_NAME:-launcher-app}"
TARGET_ARCH="${TARGET_ARCH:-arm64}"
OUT_DIR="${OUT_DIR:-dist}"
ICON_PNG="${ICON_PNG:-}"
SHOW_DOCK_ICON="${SHOW_DOCK_ICON:-1}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
APPICONS_DIR="$ROOT_DIR/AppIcons"
LICENSE_FILE=""
GIT_COMMIT="${GIT_COMMIT:-}"

for candidate in "LICENSE" "LICENSE.md" "license.txt" "COPYING" "COPYING.md"; do
  if [[ -f "$ROOT_DIR/$candidate" ]]; then
    LICENSE_FILE="$ROOT_DIR/$candidate"
    break
  fi
done

if [[ -z "$GIT_COMMIT" ]]; then
  GIT_COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
fi
if [[ -z "$VERSION" ]]; then
  TAG="$(git -C "$ROOT_DIR" describe --tags --exact-match 2>/dev/null || true)"
  if [[ -n "$TAG" ]]; then
    VERSION="${TAG#v}"
  else
    VERSION="0.0.0-${GIT_COMMIT}"
  fi
fi

if [[ -z "$ICON_PNG" ]]; then
  if [[ -f "$APPICONS_DIR/sizes/1024.png" ]]; then
    ICON_PNG="$APPICONS_DIR/sizes/1024.png"
  elif [[ -f "$APPICONS_DIR/appstore.png" ]]; then
    ICON_PNG="$APPICONS_DIR/appstore.png"
  fi
fi

APP_DIR="$ROOT_DIR/$OUT_DIR/${APP_NAME}.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RES_DIR="$CONTENTS_DIR/Resources"
PLIST_PATH="$CONTENTS_DIR/Info.plist"
ICON_NAME="AppIcon"

mkdir -p "$ROOT_DIR/$OUT_DIR" "$MACOS_DIR" "$RES_DIR"

echo "Building macOS binary..."
(
  cd "$ROOT_DIR"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$TARGET_ARCH" go build -trimpath -ldflags="-s -w -X main.buildMode=prod -X main.appVersion=$VERSION -X main.gitCommit=$GIT_COMMIT" -o "$MACOS_DIR/$BIN_NAME" ./cmd/launcher
)
chmod +x "$MACOS_DIR/$BIN_NAME"

cat > "$MACOS_DIR/$APP_EXEC_NAME" <<EOF
#!/usr/bin/env bash
set -euo pipefail
HERE="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="\$HERE/$BIN_NAME"

# If already in a terminal, run directly.
if [ -t 1 ]; then
  exec "\$BIN_PATH" "\$@"
fi

# Finder/icon launch: open Terminal and run backend there.
osascript - "\$BIN_PATH" <<'OSA'
on run argv
  set targetCmd to quoted form of item 1 of argv
  tell application "Terminal"
    activate
    do script targetCmd
  end tell
end run
OSA
EOF
chmod +x "$MACOS_DIR/$APP_EXEC_NAME"

if [[ -n "$ICON_PNG" ]]; then
  if [[ ! -f "$ICON_PNG" ]]; then
    echo "ICON_PNG not found: $ICON_PNG"
    exit 1
  fi

  ICONSET_DIR="$ROOT_DIR/$OUT_DIR/${ICON_NAME}.iconset"
  rm -rf "$ICONSET_DIR"
  mkdir -p "$ICONSET_DIR"

  # Build iconset sizes required by iconutil.
  sips -z 16 16 "$ICON_PNG" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null
  sips -z 32 32 "$ICON_PNG" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null
  sips -z 32 32 "$ICON_PNG" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null
  sips -z 64 64 "$ICON_PNG" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null
  sips -z 128 128 "$ICON_PNG" --out "$ICONSET_DIR/icon_128x128.png" >/dev/null
  sips -z 256 256 "$ICON_PNG" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null
  sips -z 256 256 "$ICON_PNG" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null
  sips -z 512 512 "$ICON_PNG" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null
  sips -z 512 512 "$ICON_PNG" --out "$ICONSET_DIR/icon_512x512.png" >/dev/null
  sips -z 1024 1024 "$ICON_PNG" --out "$ICONSET_DIR/icon_512x512@2x.png" >/dev/null

  iconutil -c icns "$ICONSET_DIR" -o "$RES_DIR/$ICON_NAME.icns"
  rm -rf "$ICONSET_DIR"
  HAS_ICON=1
else
  HAS_ICON=0
fi

echo "Writing Info.plist..."
cat > "$PLIST_PATH" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>${APP_NAME}</string>
  <key>CFBundleDisplayName</key><string>${APP_NAME}</string>
  <key>CFBundleIdentifier</key><string>${BUNDLE_ID}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>${APP_EXEC_NAME}</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
PLIST

if [[ "$SHOW_DOCK_ICON" != "1" ]]; then
  cat >> "$PLIST_PATH" <<'PLIST'
  <key>LSUIElement</key><true/>
PLIST
fi

if [[ "$HAS_ICON" == "1" ]]; then
  cat >> "$PLIST_PATH" <<PLIST
  <key>CFBundleIconFile</key><string>${ICON_NAME}</string>
PLIST
fi

cat >> "$PLIST_PATH" <<'PLIST'
</dict>
</plist>
PLIST

if command -v codesign >/dev/null 2>&1; then
  codesign --force --deep --sign - "$APP_DIR" >/dev/null 2>&1 || true
fi

if [[ -n "$LICENSE_FILE" ]]; then
  cp "$LICENSE_FILE" "$RES_DIR/LICENSE.txt"
else
  echo "Warning: no license file found in project root; skipping license copy for macOS app."
fi

echo "Done: $APP_DIR"
echo "Open it with: open \"$APP_DIR\""
