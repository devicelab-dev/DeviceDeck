#!/usr/bin/env bash
# Sign (Developer ID + hardened runtime) and notarize the three DeviceDeck
# binaries staged in a dist dir, in place, before they are tarred.
#
#   scripts/macos-sign-notarize.sh dist/devicedeck-0.1.0-darwin-arm64
#
# Graceful no-op: if DEVELOPER_ID is unset it leaves the binaries unsigned and
# exits 0, so `make release` (local/dev) still works without Apple credentials.
#
# Credentials (set by the release workflow from GitHub secrets, or locally):
#   DEVELOPER_ID          "Developer ID Application: Name (TEAMID)"   (required to sign)
#   NOTARY_PROFILE        a stored notarytool keychain profile        (local convenience)
#   -- or the App Store Connect API key trio --
#   APPLE_API_KEY_PATH    path to the .p8 key file
#   APPLE_API_KEY_ID      the key id
#   APPLE_API_ISSUER      the issuer id
set -euo pipefail

DIST="${1:?usage: macos-sign-notarize.sh <dist-dir>}"
BINS=(devicedeck-hid devicedeck-video devicedeck)   # sidecars first, main last
: "${DEVELOPER_ID:=}"

if [ -z "$DEVELOPER_ID" ]; then
  echo "note: DEVELOPER_ID unset — leaving binaries unsigned (local/dev build)"
  exit 0
fi

echo "==> Signing (Developer ID, hardened runtime)"
for b in "${BINS[@]}"; do
  codesign --force --options runtime --timestamp --sign "$DEVELOPER_ID" "$DIST/$b"
  codesign --verify --strict --verbose=2 "$DIST/$b"
done

echo "==> Notarizing (submitting to Apple; this can take a few minutes)"
ZIP="$(mktemp -d)/devicedeck-notarize.zip"
# Zip just the three signed binaries — that is what Apple checks.
( cd "$DIST" && ditto -c -k --sequesterRsrc "${BINS[@]}" "$ZIP" )

if [ -n "${NOTARY_PROFILE:-}" ]; then
  xcrun notarytool submit "$ZIP" --keychain-profile "$NOTARY_PROFILE" --wait
else
  : "${APPLE_API_KEY_PATH:?need NOTARY_PROFILE, or APPLE_API_KEY_PATH/ID/ISSUER}"
  : "${APPLE_API_KEY_ID:?need APPLE_API_KEY_ID}"
  : "${APPLE_API_ISSUER:?need APPLE_API_ISSUER}"
  xcrun notarytool submit "$ZIP" \
    --key "$APPLE_API_KEY_PATH" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER" --wait
fi

# Bare CLI binaries cannot be stapled (stapler only takes .app/.pkg/.dmg), so
# Gatekeeper verifies the notarization online on first launch. The ticket is
# now registered with Apple; nothing else to embed.
echo "==> Signed + notarized: ${BINS[*]}"
