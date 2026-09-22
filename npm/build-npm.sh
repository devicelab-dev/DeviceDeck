#!/usr/bin/env bash
# Assemble DeviceDeck's npm packages from an existing release build.
#
#   make npm VERSION=0.1.1      (or: VERSION=0.1.1 ./npm/build-npm.sh)
#
# Consumes dist/<VERSION>/ — the signed, notarized archives `make release-all`
# produced. Nothing is compiled here: a channel with its own build would drift
# from the archive everyone else downloads.
#
# Produces npm/platforms/<pkg>/ for each Mac architecture and stamps the
# version into npm/devicedeck/, then prints the publish commands. It does not
# publish; that needs npm credentials and a deliberate decision.
set -euo pipefail

VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
    echo "Error: VERSION is required — make npm VERSION=0.1.1" >&2
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist/$VERSION"
NPM_DIR="$REPO_ROOT/npm"
PLATFORMS_DIR="$NPM_DIR/platforms"

if [ ! -d "$DIST_DIR" ]; then
    echo "Error: $DIST_DIR not found — run make release-all VERSION=$VERSION first" >&2
    exit 1
fi

# Release archives use uname -m names; npm selects packages by Node's
# process.arch. They differ for Intel, so the mapping is explicit:
# "<archive arch> <node arch>".
TARGETS=(
    "arm64 arm64"
    "x86_64 x64"
)

rm -rf "$PLATFORMS_DIR"
mkdir -p "$PLATFORMS_DIR"

for target in "${TARGETS[@]}"; do
    read -r arch node_arch <<< "$target"
    pkg="@devicelab/devicedeck-darwin-${node_arch}"
    archive="$DIST_DIR/devicedeck-${VERSION}-darwin-${arch}.tar.gz"
    [ -f "$archive" ] || { echo "Error: missing $archive" >&2; exit 1; }

    # Check the archive against its published checksum: the npm package must
    # carry exactly the bytes the download host serves.
    (cd "$DIST_DIR" && shasum -a 256 -c "$(basename "$archive").sha256" >/dev/null) \
        || { echo "Error: $archive does not match its .sha256" >&2; exit 1; }

    echo "Packaging $pkg"
    dest="$PLATFORMS_DIR/devicedeck-darwin-${node_arch}"
    mkdir -p "$dest"
    # The archive's layout is the one devicedeck expects: bin/devicedeck with
    # its two sidecars beside it. Strip the wrapping folder, keep the rest.
    tar -xzf "$archive" -C "$dest" --strip-components=1
    for b in devicedeck devicedeck-hid devicedeck-video; do
        [ -x "$dest/bin/$b" ] || { echo "Error: $pkg has no executable bin/$b" >&2; exit 1; }
    done

    cat > "$dest/package.json" <<JSON
{
  "name": "$pkg",
  "version": "$VERSION",
  "description": "DeviceDeck binaries for macOS ${node_arch}. Installed automatically as an optional dependency of devicedeck.",
  "homepage": "https://github.com/devicelab-dev/DeviceDeck#readme",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/devicelab-dev/DeviceDeck.git"
  },
  "license": "Apache-2.0",
  "author": "DeviceLab (https://devicelab.dev)",
  "os": ["darwin"],
  "cpu": ["$node_arch"],
  "files": ["bin/", "LICENSE", "ATTRIBUTION.md"],
  "preferUnplugged": true
}
JSON
done

# Stamp the version into the main package and the optional dependencies,
# which must resolve to exactly this build.
node - "$NPM_DIR/devicedeck/package.json" "$VERSION" <<'NODE'
const fs = require('node:fs');
const [file, version] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(file, 'utf8'));
pkg.version = version;
for (const dep of Object.keys(pkg.optionalDependencies)) pkg.optionalDependencies[dep] = version;
fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + '\n');
NODE

echo
echo "Built npm packages for $VERSION. Check them before publishing:"
echo "  (cd npm/devicedeck && npm pack --dry-run)"
echo
echo "Publish — platform packages first, so devicedeck never points at a version that does not exist:"
for target in "${TARGETS[@]}"; do
    read -r _ node_arch <<< "$target"
    echo "  npm publish npm/platforms/devicedeck-darwin-${node_arch} --access public"
done
echo "  npm publish npm/devicedeck --access public"
