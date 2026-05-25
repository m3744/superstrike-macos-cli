#!/usr/bin/env bash
# Installs the Superstrike Control app for the current user: binary on PATH,
# icon, and desktop launcher (so it shows up in your app menu with the icon).
set -euo pipefail

cd "$(dirname "$0")/.."

echo "building…"
go build -o superstrike .

BIN="$HOME/.local/bin"
ICONS="$HOME/.local/share/icons/hicolor/scalable/apps"
APPS="$HOME/.local/share/applications"
mkdir -p "$BIN" "$ICONS" "$APPS"

install -m755 superstrike "$BIN/superstrike"
install -m644 internal/ui/icon.svg "$ICONS/superstrike.svg"
install -m644 packaging/superstrike.desktop "$APPS/superstrike.desktop"

# refresh caches if the tools exist
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$APPS" || true
command -v gtk-update-icon-cache  >/dev/null 2>&1 && gtk-update-icon-cache -f "$HOME/.local/share/icons/hicolor" 2>/dev/null || true

echo "installed:"
echo "  binary : $BIN/superstrike   (ensure ~/.local/bin is on your PATH)"
echo "  icon   : $ICONS/superstrike.svg"
echo "  launcher: $APPS/superstrike.desktop"
echo "Look for 'Superstrike Control' in your application menu."
