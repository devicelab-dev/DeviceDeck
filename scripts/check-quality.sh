#!/usr/bin/env bash
# check-quality.sh — the fast quality gate that runs on every code change
# (pre-commit hook, `make quality`, or by hand). It checks only what changed
# against HEAD, so it stays quick enough to run on each commit.
#
# Order is cheapest-first, and it stops at the first failure:
#   1. gofmt      — formatting is non-negotiable and instant
#   2. token-slice — the panic class no linter catches (scripts/lint-token-slice.sh)
#   3. golangci-lint --new-from-rev=HEAD — the full linter set, but only on
#      lines this change touches, so a clean gate stays clean without forcing
#      a sweep of pre-existing findings
#   4. go build   — the change must compile
#
# Run FULL (whole tree, not just the diff) with:  scripts/check-quality.sh --all
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

MODE="${1:-diff}"
fail() { echo "✗ $1" >&2; exit 1; }

# 1. gofmt — list any Go file that is not gofmt-clean.
unformatted=$(gofmt -l . | grep -vE '^\.claude/|^\.git/' || true)
[ -z "$unformatted" ] || fail $'gofmt: these files need formatting (run `gofmt -w .`):\n'"$unformatted"

# 2. token-slice guard — the redaction panic class.
bash scripts/lint-token-slice.sh || fail "token-slice guard tripped (see output above)"

# 3. golangci-lint. Diff mode (default) only flags findings on changed lines;
#    --all sweeps the whole tree.
if [ "$MODE" = "--all" ]; then
  golangci-lint run ./... || fail "golangci-lint found issues"
else
  golangci-lint run --new-from-rev=HEAD ./... || fail "golangci-lint found issues on changed lines"
fi

# 4. It must compile.
go build ./... || fail "go build failed"

echo "✓ quality gate passed"
