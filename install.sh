#!/usr/bin/env bash
# DeviceDeck installer — downloads a release archive (the binary and its two
# Swift sidecars) into ~/.devicedeck and puts its bin folder on your PATH.
# Self-contained: no sudo, and no other tool needs to be installed. The
# Android driver ships inside the binary and is written under ~/.devicedeck on
# first run; the iOS runner is built there on first use.
#
#   curl -fsSL https://open.devicelab.dev/install/devicedeck | bash
#   curl -fsSL https://open.devicelab.dev/install/devicedeck | bash -s -- --version 0.1.0
#
# macOS only (the sidecars talk to CoreSimulator). Apple Silicon and Intel.
set -euo pipefail

REPO="devicelab-dev/DeviceDeck"
# DEVICEDECK_HOME must match what the server resolves, so both agree on one
# folder for binaries, drivers and build cache.
HOME_DIR="${DEVICEDECK_HOME:-$HOME/.devicedeck}"
BIN_DIR="$HOME_DIR/bin"

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

if [ -n "${DEVICEDECK_ARCHIVE:-}" ]; then
  TAG="v${REQ_VERSION:-local}"
elif [ -n "$REQ_VERSION" ]; then
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
# DEVICEDECK_ARCHIVE installs a local archive instead of downloading one,
# for testing a build before it is released.
if [ -n "${DEVICEDECK_ARCHIVE:-}" ]; then
  say "Using local archive ${DEVICEDECK_ARCHIVE}…"
  cp "$DEVICEDECK_ARCHIVE" "$TMP/dd.tar.gz" || err "cannot read $DEVICEDECK_ARCHIVE"
else
  say "Downloading ${ASSET}…"
  curl -fSL --progress-bar "$URL" -o "$TMP/dd.tar.gz" || err "download failed: $URL"
fi
tar xzf "$TMP/dd.tar.gz" -C "$TMP"
SRC="$(find "$TMP" -maxdepth 1 -type d -name 'devicedeck-*' | head -1)"
[ -n "$SRC" ] || err "unexpected archive layout"

[ -x "$SRC/bin/devicedeck" ] || err "unexpected archive layout (no bin/devicedeck)"

say "Installing to ${HOME_DIR}…"
mkdir -p "$BIN_DIR"
cp "$SRC"/bin/devicedeck "$SRC"/bin/devicedeck-hid "$SRC"/bin/devicedeck-video "$BIN_DIR"/
cp "$SRC"/LICENSE "$SRC"/ATTRIBUTION.md "$SRC"/README.md "$HOME_DIR"/ 2>/dev/null || true
# Signed + notarized builds pass Gatekeeper on their own, and curl does not set
# the quarantine bit anyway — this strip is a harmless fallback for unsigned/dev
# archives or a browser-downloaded tarball.
xattr -dr com.apple.quarantine "$BIN_DIR" 2>/dev/null || true

# add_to_path appends one PATH line to the shell's startup file, once.
add_to_path() {
  local profile="$1" line="export PATH=\"$BIN_DIR:\$PATH\""
  if ! grep -qsF "$BIN_DIR" "$profile"; then
    printf '\n# DeviceDeck\n%s\n' "$line" >> "$profile"
    say "Added $BIN_DIR to PATH in $profile"
  fi
}
case "$(basename "${SHELL:-zsh}")" in
  zsh)  add_to_path "$HOME/.zshrc" ;;
  bash) add_to_path "$HOME/.bash_profile" ;;
  *)    add_to_path "$HOME/.profile" ;;
esac

say "Installed $("$BIN_DIR/devicedeck" version 2>/dev/null || echo "devicedeck ${VERSION}")"
say "Open a new terminal (or: export PATH=\"$BIN_DIR:\$PATH\"), then run: devicedeck serve"
say "Then open http://127.0.0.1:8787"
