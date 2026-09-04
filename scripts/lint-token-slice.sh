#!/bin/bash
# Guards against the unsafe fixed-length slice idiom no linter catches:
# `value[:16]+"…"` (log truncation) or `token[:N]` panics when the value is
# shorter than the bound. It needs runtime length knowledge, so golangci-lint,
# gosec, and staticcheck are all blind to it. Guard the length first
# (e.g. `if len(s) > n`) before slicing for a log preview.
#
# Flags, in non-generated .go files:
#   1. a fixed slice immediately concatenated with a string literal: s[:8]+"…"
#   2. any fixed slice of a *token/*Token variable
#
# A line proven safe by a length check may carry an `// allow-token-slice`
# comment to opt out.
#
# Usage: lint-token-slice.sh <file-or-dir>...   (exit 1 on findings)

found=0
for target in "$@"; do
    [ -e "$target" ] || continue
    matches=$(grep -rnE '\[:[0-9]+\][[:space:]]*\+[[:space:]]*"|[Tt]oken\[:[0-9]+\]' \
        --include='*.go' "$target" 2>/dev/null \
        | grep -v '_test\.go' | grep -v '\.pb\.go' \
        | grep -v 'allow-token-slice')
    if [ -n "$matches" ]; then
        echo "$matches"
        found=1
    fi
done

if [ "$found" -eq 1 ]; then
    echo "unsafe fixed-length slice on a possibly-short string (panics like 'slice bounds out of range [:16] with length 7') — guard the length first, or mark a proven-safe line with // allow-token-slice" >&2
    exit 1
fi
exit 0
