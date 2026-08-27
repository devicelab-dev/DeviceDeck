#!/usr/bin/env bash
# Redact absolute local filesystem paths (/Users/…) from the release binaries,
# in place, and FAIL the build if any remain.
#
# Belt-and-suspenders on top of `go build -trimpath`: trimpath removes the Go
# toolchain's own path annotations, but it cannot touch (a) embedded file
# *contents* pulled in by a dependency's go:embed, or (b) Swift build paths in
# the sidecars. This scrubs both, so the shipped archive has no home-directory
# reference for anyone to `strings` out. Runs before code signing, so the
# signature covers the scrubbed bytes.
set -euo pipefail
DIST="${1:?usage: redact-local-paths.sh <dist-dir>}"

python3 - "$DIST" <<'PY'
import os, re, sys
dist = sys.argv[1]
# An absolute macOS path run, up to a delimiter. Replacing each match with the
# same number of blanks preserves every offset in the Mach-O, so the binary
# stays structurally valid and re-signable.
pat = re.compile(rb"/Users/[^\s\"'`\x00]{0,256}")
bins = [b for b in ("devicedeck", "devicedeck-hid", "devicedeck-video")
        if os.path.exists(os.path.join(dist, b))]

for name in bins:
    p = os.path.join(dist, name)
    data = open(p, "rb").read()
    new, n = pat.subn(lambda m: b" " * len(m.group(0)), data)
    if n:
        open(p, "wb").write(new)
        print(f"  redacted {n} local-path reference(s) in {name}")

bad = 0
for name in bins:
    for h in pat.findall(open(os.path.join(dist, name), "rb").read()):
        print(f"  LEAK in {name}: {h[:80]!r}", file=sys.stderr)
        bad += 1
if bad:
    sys.exit(f"error: {bad} local-path reference(s) still present after redaction")
print(f"  verified: no /Users/ paths in {', '.join(bins)}")
PY
