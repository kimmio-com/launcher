#!/usr/bin/env bash
set -euo pipefail

APP_NAME="${APP_NAME:-Kimmio Launcher}"
OUT_DIR="${OUT_DIR:-dist}"
BACKEND_BIN_NAME="${BACKEND_BIN_NAME:-luncher}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

BACKEND_BIN_PATH="$ROOT_DIR/$OUT_DIR/${APP_NAME}.app/Contents/MacOS/$BACKEND_BIN_NAME"
DOCK_APP_PATH="$ROOT_DIR/$OUT_DIR/${APP_NAME} Dock.app"
TMP_SCRIPT="$(mktemp)"

if [[ ! -f "$BACKEND_BIN_PATH" ]]; then
  echo "Backend binary not found: $BACKEND_BIN_PATH"
  echo "Build macOS app first: ./scripts/build-macos-app.sh"
  exit 1
fi

cat > "$TMP_SCRIPT" <<JXA
ObjC.import('stdlib');

function run() {
  const app = Application.currentApplication();
  app.includeStandardAdditions = true;

  const defaultPort = "7331";
  const portFile = $.NSHomeDirectory().js + "/Library/Application Support/KimmioLauncher/launcher-port";

  function sh(command) {
    return app.doShellScript(command);
  }

  function readPort() {
    try {
      const p = sh("cat " + quote(portFile) + " | tr -d '\\\\r\\\\n'");
      return p && p.length > 0 ? p : defaultPort;
    } catch (e) {
      return defaultPort;
    }
  }

  function quote(v) {
    return "'" + String(v).replace(/'/g, "'\\\\''") + "'";
  }

  function isListening(port) {
    try {
      sh("lsof -nP -iTCP:" + port + " -sTCP:LISTEN -t | head -n 1");
      return true;
    } catch (e) {
      return false;
    }
  }

  let activePort = readPort();
  if (!isListening(activePort)) {
    sh(quote("$BACKEND_BIN_PATH") + " >/tmp/kimmio-launcher.log 2>&1 &");
    delay(1);
    activePort = readPort();
  }

  app.openLocation("http://localhost:" + activePort);
  app.activate();
}
JXA

rm -rf "$DOCK_APP_PATH"
osacompile -l JavaScript -o "$DOCK_APP_PATH" "$TMP_SCRIPT"
rm -f "$TMP_SCRIPT"

if [[ -f "$ROOT_DIR/$OUT_DIR/${APP_NAME}.app/Contents/Resources/AppIcon.icns" ]]; then
  cp "$ROOT_DIR/$OUT_DIR/${APP_NAME}.app/Contents/Resources/AppIcon.icns" "$DOCK_APP_PATH/Contents/Resources/applet.icns"
fi

if command -v codesign >/dev/null 2>&1; then
  codesign --force --deep --sign - "$DOCK_APP_PATH" >/dev/null 2>&1 || true
fi

echo "Done: $DOCK_APP_PATH"
echo "Pin this app to Dock and use it as your launcher icon."
