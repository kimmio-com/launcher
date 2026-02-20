#!/usr/bin/env bash
set -euo pipefail

# Build a Windows desktop-style distribution folder.
#
# Usage:
#   scripts/build-windows-app.sh
#   APP_NAME="Kimmio Launcher" ICON_ICO="/abs/path/icon.ico" scripts/build-windows-app.sh
#
# Optional env vars:
#   APP_NAME        Default: "Kimmio Launcher"
#   EXE_NAME        Default: "KimmioLauncher.exe"
#   VERSION         Default: inferred from tag (or 0.0.0-<commit>)
#   OUT_DIR         Default: "dist"
#   ICON_ICO        Optional .ico path; auto-detected from AppIcons if omitted

APP_NAME="${APP_NAME:-Kimmio Launcher}"
EXE_NAME="${EXE_NAME:-KimmioLauncher.exe}"
VERSION="${VERSION:-}"
OUT_DIR="${OUT_DIR:-dist}"
ICON_ICO="${ICON_ICO:-}"
GIT_COMMIT="${GIT_COMMIT:-}"
BUILD_WINDOWS_INSTALLER="${BUILD_WINDOWS_INSTALLER:-0}"

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

if [[ -z "$ICON_ICO" ]]; then
  if [[ -f "$APPICONS_DIR/app.ico" ]]; then
    ICON_ICO="$APPICONS_DIR/app.ico"
  elif [[ -f "$APPICONS_DIR/icon.ico" ]]; then
    ICON_ICO="$APPICONS_DIR/icon.ico"
  fi
fi

PKG_DIR="$OUT_DIR/windows/${APP_NAME}"
BIN_PATH="$PKG_DIR/$EXE_NAME"

mkdir -p "$ROOT_DIR/$PKG_DIR"

echo "Building Windows binary..."
(
  cd "$ROOT_DIR"
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.buildMode=prod -X main.appVersion=$VERSION -X main.gitCommit=$GIT_COMMIT" -o "$BIN_PATH" ./cmd/launcher
)

if [[ -n "$ICON_ICO" ]]; then
  if [[ ! -f "$ICON_ICO" ]]; then
    echo "ICON_ICO not found: $ICON_ICO"
    exit 1
  fi
  cp "$ICON_ICO" "$ROOT_DIR/$PKG_DIR/AppIcon.ico"
elif [[ -f "$APPICONS_DIR/sizes/256.png" ]] && command -v magick >/dev/null 2>&1; then
  magick "$APPICONS_DIR/sizes/256.png" "$ROOT_DIR/$PKG_DIR/AppIcon.ico"
elif [[ -f "$ROOT_DIR/cmd/launcher/static/favicon.ico" ]]; then
  cp "$ROOT_DIR/cmd/launcher/static/favicon.ico" "$ROOT_DIR/$PKG_DIR/AppIcon.ico"
fi

cat > "$ROOT_DIR/$PKG_DIR/README.txt" <<EOF
$APP_NAME
Version: $VERSION

How to run:
1. Open this folder.
2. Double-click $EXE_NAME

Optional shortcut:
- Run create-shortcut.ps1 to create a Desktop shortcut using AppIcon.ico.
EOF

cat > "$ROOT_DIR/$PKG_DIR/create-shortcut.ps1" <<'EOF'
param(
  [string]$AppExe = ".\KimmioLauncher.exe",
  [string]$AppName = "Kimmio Launcher",
  [string]$IconPath = ".\AppIcon.ico"
)

$desktop = [Environment]::GetFolderPath("Desktop")
$shortcutPath = Join-Path $desktop "$AppName.lnk"
$wshShell = New-Object -ComObject WScript.Shell
$shortcut = $wshShell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = (Resolve-Path $AppExe).Path
$shortcut.WorkingDirectory = Split-Path -Parent (Resolve-Path $AppExe).Path
if (Test-Path $IconPath) {
  $shortcut.IconLocation = (Resolve-Path $IconPath).Path
}
$shortcut.Save()
Write-Host "Shortcut created:" $shortcutPath
EOF

cat > "$ROOT_DIR/$PKG_DIR/run.bat" <<EOF
@echo off
cd /d "%~dp0"
start "" "%EXE_NAME%"
EOF

if [[ -n "$LICENSE_FILE" ]]; then
  cp "$LICENSE_FILE" "$ROOT_DIR/$PKG_DIR/LICENSE.txt"
else
  echo "Warning: no license file found in project root; skipping license copy for Windows package."
fi

ZIP_NAME="$OUT_DIR/${APP_NAME// /-}-windows-amd64.zip"
(
  cd "$ROOT_DIR/$OUT_DIR/windows"
  if command -v zip >/dev/null 2>&1; then
    zip -rq "../$(basename "$ZIP_NAME")" "$APP_NAME"
  else
    echo "zip tool not found; leaving unpacked folder at $ROOT_DIR/$PKG_DIR"
  fi
)

echo "Done: $ROOT_DIR/$PKG_DIR"
if [[ -f "$ROOT_DIR/$ZIP_NAME" ]]; then
  echo "Zip package: $ROOT_DIR/$ZIP_NAME"
fi

if [[ "$BUILD_WINDOWS_INSTALLER" == "1" ]] && command -v iscc >/dev/null 2>&1; then
  echo "Building Windows installer (Inno Setup)..."
  iscc "/DAppVersion=$VERSION" "/DAppExeName=$EXE_NAME" "/DSourceDir=$PKG_DIR" "/DOutputDir=$OUT_DIR" "$SCRIPT_DIR/windows-installer.iss"
  if [[ -f "$ROOT_DIR/$OUT_DIR/Kimmio-Launcher-Setup-windows-amd64.exe" ]]; then
    echo "Installer: $ROOT_DIR/$OUT_DIR/Kimmio-Launcher-Setup-windows-amd64.exe"
  fi
fi
