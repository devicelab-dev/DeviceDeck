#!/bin/sh
set -e

# DeviceDeck installer
# Usage: curl -fsSL https://open.devicelab.dev/install/devicedeck | bash
# Pin a version: curl -fsSL https://open.devicelab.dev/install/devicedeck | bash -s -- --version 0.1.0
#
# Installs the devicedeck binary and its two Swift sidecars into
# ~/.devicedeck/bin. No sudo. macOS only (the sidecars talk to CoreSimulator).
# DEVICEDECK_HOME overrides the install folder; it must match what the server
# resolves, so both agree on one folder for binaries, drivers and build cache.
# DEVICEDECK_ARCHIVE=<path> installs a local archive instead of downloading one,
# for testing a build before it is released.

INSTALL_DIR="${DEVICEDECK_HOME:-$HOME/.devicedeck}"
BIN_DIR="$INSTALL_DIR/bin"
API_URL="https://open.devicelab.dev/api/devicedeck/updates"
DOWNLOAD_BASE="https://open.devicelab.dev/download"

# Cleanup temp files on exit
cleanup() {
    if [ -n "$TEMP_DIR" ] && [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
    fi
}
trap cleanup EXIT INT TERM

info() {
    printf "  %s\n" "$1"
}

fail() {
    printf "\n  Error: %s\n\n" "$1" >&2
    exit 1
}

check_done() {
    printf " ✓\n"
}

format_size() {
    size="$1"
    if [ "$size" -lt 1024 ] 2>/dev/null; then
        echo "${size}B"
    elif [ "$size" -lt 1048576 ] 2>/dev/null; then
        echo "$((size / 1024))KB"
    else
        echo "$((size / 1048576))MB"
    fi
}

# --- Parse arguments ---

VERSION=""
while [ $# -gt 0 ]; do
    case "$1" in
        --version)
            [ -n "${2:-}" ] || fail "--version needs a value, e.g. --version 0.1.0"
            VERSION="${2#v}"
            shift 2
            ;;
        --version=*)
            VERSION="${1#*=}"
            VERSION="${VERSION#v}"
            shift
            ;;
        *)
            fail "Unknown option: $1"
            ;;
    esac
done

# --- Pre-flight checks ---

command -v curl >/dev/null 2>&1 || fail "curl is required but not installed"

printf "\n"
info "Installing DeviceDeck..."
printf "\n"

# --- Step 1: Detect platform ---

OS="$(uname -s)"
case "$OS" in
    Darwin|darwin) OS="darwin" ;;
    *)             fail "Unsupported operating system: $OS
DeviceDeck hosts iOS Simulators, so the host needs macOS.
Clients (Playwright, AI agents, browsers) can run on any OS." ;;
esac

# Release archives use uname -m names (arm64, x86_64), matching the Makefile.
ARCH="$(uname -m)"
case "$ARCH" in
    arm64)  ARCH="arm64" ;;
    x86_64)
        # A Rosetta shell on Apple Silicon reports x86_64; install the native build.
        if [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = "1" ]; then
            ARCH="arm64"
        fi
        ;;
    *)
        fail "Unsupported architecture: $ARCH
Supported: arm64 (Apple Silicon), x86_64 (Intel)" ;;
esac

info "Platform: $OS $ARCH"

# --- Step 2: Get version ---

if [ -n "${DEVICEDECK_ARCHIVE:-}" ]; then
    VERSION="${VERSION:-local}"
elif [ -z "$VERSION" ]; then
    RESPONSE="$(curl --fail --silent --show-error --location --max-time 15 "$API_URL" 2>/dev/null)" || fail "Could not reach update API at $API_URL

Please check your internet connection and try again."
    VERSION="$(printf '%s' "$RESPONSE" | grep -o '"latest_version"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"')"
    if [ -z "$VERSION" ]; then
        fail "Could not determine latest version from API response"
    fi
fi

info "Version:  $VERSION"
printf "\n"

# --- Step 3: Download ---

ARCHIVE="devicedeck-${VERSION}-${OS}-${ARCH}.tar.gz"
DOWNLOAD_URL="${DOWNLOAD_BASE}/devicedeck/${VERSION}/${ARCHIVE}"

TEMP_DIR="$(mktemp -d)"

if [ -n "${DEVICEDECK_ARCHIVE:-}" ]; then
    info "Using local archive: $DEVICEDECK_ARCHIVE"
    cp "$DEVICEDECK_ARCHIVE" "$TEMP_DIR/$ARCHIVE" || fail "Cannot read $DEVICEDECK_ARCHIVE"
else
    printf "  Downloading...\n"
    curl --fail --location --progress-bar --max-time 300 \
        -o "$TEMP_DIR/$ARCHIVE" "$DOWNLOAD_URL" || fail "Download failed
URL: $DOWNLOAD_URL

Please check your internet connection and try again."
fi

# Verify download
if [ ! -s "$TEMP_DIR/$ARCHIVE" ]; then
    fail "Downloaded file is empty"
fi

ACTUAL_SIZE=$(wc -c < "$TEMP_DIR/$ARCHIVE" | tr -d ' ')
info "Downloaded: $(format_size "$ACTUAL_SIZE")"

# Verify checksum (a local archive is checked against a .sha256 beside it)
printf "  Verifying checksum..."
SHA_OK=""
if [ -n "${DEVICEDECK_ARCHIVE:-}" ]; then
    [ -f "${DEVICEDECK_ARCHIVE}.sha256" ] && cp "${DEVICEDECK_ARCHIVE}.sha256" "$TEMP_DIR/${ARCHIVE}.sha256" && SHA_OK=1
elif curl --fail --silent --location --max-time 15 \
    -o "$TEMP_DIR/${ARCHIVE}.sha256" "${DOWNLOAD_URL}.sha256" 2>/dev/null; then
    SHA_OK=1
fi
if [ -n "$SHA_OK" ]; then
    EXPECTED_SHA=$(awk '{print $1}' "$TEMP_DIR/${ARCHIVE}.sha256")
    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL_SHA=$(sha256sum "$TEMP_DIR/$ARCHIVE" | awk '{print $1}')
    else
        ACTUAL_SHA=$(shasum -a 256 "$TEMP_DIR/$ARCHIVE" | awk '{print $1}')
    fi
    if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
        fail "Checksum verification failed
  Expected: $EXPECTED_SHA
  Got:      $ACTUAL_SHA"
    fi
    check_done
else
    printf " skipped (checksum not available)\n"
fi

# --- Step 4: Install ---

printf "  Extracting to %s..." "$INSTALL_DIR"

# Extract to temp dir first
mkdir -p "$TEMP_DIR/extract"
tar -xzf "$TEMP_DIR/$ARCHIVE" -C "$TEMP_DIR/extract" || fail "Failed to extract archive"

# The archive has one top folder, devicedeck-<version>-darwin-<arch>/
SRC="$(find "$TEMP_DIR/extract" -mindepth 1 -maxdepth 1 -type d -name 'devicedeck-*' | head -1)"
[ -n "$SRC" ] && [ -f "$SRC/bin/devicedeck" ] || fail "Unexpected archive layout (no bin/devicedeck)"

umask 022
mkdir -p "$BIN_DIR"

# Replace each binary with a rename, never an in-place copy. Overwriting a
# signed executable's bytes (while it runs, or after macOS has cached its
# signature) gets every later launch of that file killed; a rename gives the
# new binary a fresh file, and a running server keeps its old one.
for f in devicedeck devicedeck-hid devicedeck-video; do
    [ -f "$SRC/bin/$f" ] || fail "Unexpected archive layout (no bin/$f)"
    cp "$SRC/bin/$f" "$BIN_DIR/.$f.new"
    chmod +x "$BIN_DIR/.$f.new"
    mv -f "$BIN_DIR/.$f.new" "$BIN_DIR/$f"
done
for f in LICENSE ATTRIBUTION.md README.md; do
    [ -f "$SRC/$f" ] && cp -f "$SRC/$f" "$INSTALL_DIR/"
done

# macOS: remove quarantine attribute (signed builds pass Gatekeeper anyway)
xattr -dr com.apple.quarantine "$BIN_DIR" 2>/dev/null || true

check_done

# --- Step 5: Add to PATH ---

printf "  Adding to PATH..."

if [ "$INSTALL_DIR" = "$HOME/.devicedeck" ]; then
    PATH_ENTRY='export PATH="$HOME/.devicedeck/bin:$PATH"'
else
    PATH_ENTRY="export PATH=\"$BIN_DIR:\$PATH\""
fi

add_to_profile() {
    profile_file="$1"
    if [ -f "$profile_file" ]; then
        if ! grep -qF "$PATH_ENTRY" "$profile_file" 2>/dev/null; then
            printf '\n# DeviceDeck\n%s\n' "$PATH_ENTRY" >> "$profile_file"
        fi
    else
        printf '# DeviceDeck\n%s\n' "$PATH_ENTRY" > "$profile_file"
    fi
}

# macOS Terminal starts bash as a login shell, which reads .bash_profile, not .bashrc.
case "${SHELL:-}" in
    */zsh)
        add_to_profile "$HOME/.zshrc"
        ;;
    */bash)
        add_to_profile "$HOME/.bash_profile"
        ;;
    *)
        if [ -f "$HOME/.zshrc" ]; then
            add_to_profile "$HOME/.zshrc"
        elif [ -f "$HOME/.bash_profile" ]; then
            add_to_profile "$HOME/.bash_profile"
        else
            add_to_profile "$HOME/.zshrc"
        fi
        ;;
esac

# Export for current session
export PATH="$BIN_DIR:$PATH"

check_done

# --- Step 6: Done ---

printf "\n"
info "DeviceDeck $VERSION installed successfully!"
printf "\n"
info "Restart your shell or run:"
printf "\n"
printf '      %s\n' "$PATH_ENTRY"
printf "\n"
info "Then: devicedeck           (open http://127.0.0.1:8787)"
info "      devicedeck doctor    (checks Xcode, adb and the rest)"
printf "\n"
info "Built by DeviceLab.dev - run your flows on real devices: https://devicelab.dev"
info "Like it? Star us: https://github.com/devicelab-dev/DeviceDeck"
printf "\n"
