# Quality gate

DeviceDeck's non-negotiables (CLAUDE.md) are enforced by tooling, not by memory.
This is what runs, and when.

## On every code change

A **pre-commit hook** runs `scripts/check-quality.sh` before each commit. It
checks only what changed against `HEAD`, so it stays fast enough to run every
time. Install it once per clone:

```sh
make hooks
```

The gate, cheapest-first, stopping at the first failure:

1. **gofmt** — every Go file must be formatted (`gofmt -w .` to fix).
2. **token-slice guard** (`scripts/lint-token-slice.sh`) — catches the
   `value[:8]+"…"` / `token[:6]` slice-out-of-range panic class that no linter
   flags. Escape a deliberate slice with a `// allow-token-slice` line comment.
3. **golangci-lint** `--new-from-rev=HEAD` — the full linter set, but only on
   the lines this change touches, so a clean gate stays clean without forcing a
   sweep of pre-existing findings.
4. **go build** — the change must compile.

Run it by hand any time:

```sh
make quality          # fast: only what changed vs HEAD
make quality ALL=1    # sweep the whole tree
```

## The full gate

`make lint` runs the linters across the **whole tree** (gofmt as a hard
failure, token-slice, `go vet`, `golangci-lint run ./...`). This is the CI-grade
check; the pre-commit hook is its fast per-change subset.

`/code-quality` (the composed Claude command) runs the linters **plus** the
docs audit, KISS/DRY/guardrail check, `/test` coverage, and `/verify-build`.
Run it before handing work back.

## Coverage

100% statement coverage is required on changed hand-written Go files
(`*.pb.go` / `*_generated.go` excluded). Find the gaps:

```sh
make cover-gaps       # lists files below 100%
```

## The linter config

`.golangci.yml` turns on the correctness-focused set (errorlint, nilerr,
makezero, durationcheck, bodyclose, copyloopvar, gosec, unconvert, plus a small
revive rule set), and the two mechanical halves of the KISS/DRY guardrail:
`nestif` (nesting deeper than 3 levels) and `dupl` (duplicated blocks).

Function **length** is deliberately *not* linted. Whether a function should be
split is a design judgment, not a static one — a length rule only ever produces
forced splits or `//nolint` lines. The `check-kiss-dry.sh` reminder and
`/code-quality`'s human review own that call; the linter owns nesting and
duplication, which are mechanical. A handful of gosec rules are excluded with rationale in the
file — they are false-positive-prone against DeviceDeck's shape (binary
responses, JSON-escaped HTML, operator-supplied file paths). Test files relax
the style-only linters but keep the correctness ones.
