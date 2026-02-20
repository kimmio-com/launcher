#!/usr/bin/env bash
set -euo pipefail

# Build a Linux desktop-style distribution (AppDir + optional AppImage).
#
# Usage:
#   scripts/build-linux-app.sh
#   APP_NAME="Kimmio Launcher" ICON_PNG="/abs/path/icon-512.png" scripts/build-linux-app.sh
#
# Optional env vars:
#   APP_NAME        Default: "Kimmio Launcher"
#   BIN_NAME        Default: "kimmio-launcher"
#   VERSION         Default: inferred from tag (or 0.0.0-<commit>)
#   OUT_DIR         Default: "dist"
#   ICON_PNG        Optional .png path; auto-detected from AppIcons if omitted

APP_NAME="${APP_NAME:-Kimmio Launcher}"
BIN_NAME="${BIN_NAME:-kimmio-launcher}"
VERSION="${VERSION:-}"
OUT_DIR="${OUT_DIR:-dist}"
ICON_PNG="${ICON_PNG:-}"
GIT_COMMIT="${GIT_COMMIT:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
APPICONS_DIR="$ROOT_DIR/AppIcons"
LICENSE_FILE=""

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
  if [[ -f "$APPICONS_DIR/sizes/512.png" ]]; then
    ICON_PNG="$APPICONS_DIR/sizes/512.png"
  elif [[ -f "$APPICONS_DIR/appstore.png" ]]; then
    ICON_PNG="$APPICONS_DIR/appstore.png"
  fi
fi

SAFE_NAME="${APP_NAME// /-}"
APPDIR="$ROOT_DIR/$OUT_DIR/linux/${SAFE_NAME}.AppDir"
BIN_PATH="$APPDIR/usr/bin/$BIN_NAME"
DESKTOP_FILE="$APPDIR/${BIN_NAME}.desktop"
APPIMAGE_PATH="$ROOT_DIR/$OUT_DIR/${SAFE_NAME}-${VERSION}-linux-amd64.AppImage"
TAR_PATH="$ROOT_DIR/$OUT_DIR/${SAFE_NAME}-${VERSION}-linux-amd64.tar.gz"

rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/512x512/apps"

echo "Building Linux binary..."
(
  cd "$ROOT_DIR"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.appVersion=$VERSION -X main.gitCommit=$GIT_COMMIT" -o "$BIN_PATH" ./cmd/launcher
)
chmod +x "$BIN_PATH"

cat > "$APPDIR/AppRun" <<EOF
#!/usr/bin/env bash
HERE="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
exec "\$HERE/usr/bin/$BIN_NAME" "\$@"
EOF
chmod +x "$APPDIR/AppRun"

cat > "$DESKTOP_FILE" <<EOF
[Desktop Entry]
Type=Application
Name=$APP_NAME
Exec=$BIN_NAME
Icon=AppIcon
Terminal=false
Categories=Utility;
EOF
cp "$DESKTOP_FILE" "$APPDIR/usr/share/applications/${BIN_NAME}.desktop"

if [[ -n "$ICON_PNG" ]]; then
  if [[ ! -f "$ICON_PNG" ]]; then
    echo "ICON_PNG not found: $ICON_PNG"
    exit 1
  fi
  cp "$ICON_PNG" "$APPDIR/AppIcon.png"
  cp "$ICON_PNG" "$APPDIR/usr/share/icons/hicolor/512x512/apps/AppIcon.png"
  cp "$ICON_PNG" "$APPDIR/.DirIcon"
fi

if [[ -n "$LICENSE_FILE" ]]; then
  cp "$LICENSE_FILE" "$APPDIR/LICENSE.txt"
else
  echo "Warning: no license file found in project root; skipping license copy for Linux package."
fi

if command -v appimagetool >/dev/null 2>&1; then
  echo "Creating AppImage..."
  appimagetool "$APPDIR" "$APPIMAGE_PATH"
  echo "AppImage: $APPIMAGE_PATH"
else
  echo "appimagetool not found; creating tar.gz package instead..."
  (
    cd "$ROOT_DIR/$OUT_DIR/linux"
    tar -czf "$TAR_PATH" "$(basename "$APPDIR")"
  )
  echo "Archive: $TAR_PATH"
fi

echo "Done: $APPDIR"
