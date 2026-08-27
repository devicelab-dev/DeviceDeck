#!/usr/bin/env bash
# DeviceDeck installer — downloads a release archive (the binary and its two
# Swift sidecars) and puts `devicedeck` on your PATH.
#
#   curl -fsSL https://open.devicelab.dev/install/devicedeck | bash
#   curl -fsSL https://open.devicelab.dev/install/devicedeck | bash -s -- --version 0.1.0
#
# macOS only (the sidecars talk to CoreSimulator). Apple Silicon and Intel.
set -euo pipefail

REPO="devicelab-dev/DeviceDeck"
BINDIR="${DEVICEDECK_BIN:-/usr/local/bin}"
LIBDIR="${DEVICEDECK_PREFIX:-/usr/local/opt}/devicedeck"

say() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
err() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = "Darwin" ] || err "DeviceDeck hosts iOS Simulators, so it needs macOS. (Clients can be any OS — this installs the host.)"

case "$(uname -m)" in
  arm64) ARCH="arm64" ;;
  x86_64) ARCH="x86_64" ;;
  *) err "unsupported architecture: $(uname -m)" ;;
esac

# --version <x> pins a release; with no flag the latest is resolved.
REQ_VERSION=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version) REQ_VERSION="${2:-}"; shift 2 ;;
    --version=*) REQ_VERSION="${1#*=}"; shift ;;
    *) err "unknown option: $1 (only --version <x> is supported)" ;;
  esac
done

if [ -n "$REQ_VERSION" ]; then
  TAG="v${REQ_VERSION#v}"
  say "Installing DeviceDeck ${TAG}…"
else
  say "Finding the latest release…"
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$TAG" ] || err "could not find a published release yet — see https://github.com/${REPO}/releases"
fi
VERSION="${TAG#v}"
ASSET="devicedeck-${VERSION}-darwin-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
say "Downloading ${ASSET}…"
curl -fSL --progress-bar "$URL" -o "$TMP/dd.tar.gz" || err "download failed: $URL"
tar xzf "$TMP/dd.tar.gz" -C "$TMP"
SRC="$(find "$TMP" -maxdepth 1 -type d -name 'devicedeck-*' | head -1)"
[ -n "$SRC" ] || err "unexpected archive layout"

say "Installing to ${LIBDIR}…"
mkdir -p "$LIBDIR"
cp "$SRC"/devicedeck "$SRC"/devicedeck-hid "$SRC"/devicedeck-video "$LIBDIR"/
# Signed + notarized builds pass Gatekeeper on their own, and curl does not set
# the quarantine bit anyway — this strip is a harmless fallback for unsigned/dev
# archives or a browser-downloaded tarball.
xattr -dr com.apple.quarantine "$LIBDIR" 2>/dev/null || true

# Symlink all three next to each other on PATH — the server looks for each
# sidecar beside its own executable, so they must stay co-located.
mkdir -p "$BINDIR"
for f in devicedeck devicedeck-hid devicedeck-video; do
  ln -sf "$LIBDIR/$f" "$BINDIR/$f"
done

say "Installed $("$BINDIR/devicedeck" version 2>/dev/null || echo "devicedeck ${VERSION}")"
say "Run: devicedeck serve   → then open http://127.0.0.1:8787"
