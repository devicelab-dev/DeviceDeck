.PHONY: build test lint quality cover-gaps hooks vet clean sidecar sidecar-test release release-signed stage sign package

BINARY := devicedeck
PKG := github.com/devicelab-dev/DeviceDeck
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT)
DIST := dist/$(BINARY)-$(VERSION)-darwin-$(shell uname -m)

# -trimpath strips filesystem paths (module-cache and repo paths under
# /Users/…) from the binary, so panic stack traces and debug info carry
# module@version paths, not the author's home directory.
build:
	go build -trimpath -o $(BINARY) ./cmd/devicedeck

test:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

sidecar:
	swift build --package-path sidecar -c release

sidecar-test:
	swift test --package-path sidecar

# lint runs the full linter set across the whole tree. gofmt is checked as a
# hard failure (not just listed), then the token-slice guard, then golangci-lint.
lint:
	@unformatted=$$(gofmt -l . | grep -vE '^\.claude/|^\.git/' || true); \
		if [ -n "$$unformatted" ]; then echo "gofmt needs: $$unformatted"; exit 1; fi
	bash scripts/lint-token-slice.sh
	go vet ./...
	golangci-lint run ./...

# quality is the fast gate that runs on every code change: it checks only what
# changed against HEAD, so it stays quick. This is what the pre-commit hook and
# the /code-quality command call. Pass ALL=1 to sweep the whole tree instead.
quality:
	bash scripts/check-quality.sh $(if $(ALL),--all,)

# cover-gaps lists hand-written Go files that are below 100% statement coverage
# — the non-negotiable target for changed files. Generated files are excluded.
cover-gaps:
	@go test ./... -coverprofile=coverage.out >/dev/null 2>&1 || true
	@go tool cover -func=coverage.out | awk '$$3 != "100.0%" && $$1 !~ /\.pb\.go|_generated\.go/ && $$1 != "total:" {print}' \
		| sort -t% -k1 || echo "  all changed files at 100%"

# hooks installs the pre-commit quality gate into this clone's .git/hooks.
# Run once after cloning so the gate runs on every commit.
hooks:
	@printf '#!/usr/bin/env bash\nexec bash scripts/check-quality.sh\n' > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "installed .git/hooks/pre-commit → scripts/check-quality.sh"

# oldlint keeps the original quick vet available under a plain name.
vet:
	go vet ./...

# stage puts the three binaries side by side, which is the layout the server
# discovers by default: it looks for each sidecar next to its own executable
# before falling back to the Swift build directory. macOS only — the sidecars
# talk to CoreSimulator, so there is nothing to ship elsewhere.
stage: sidecar
	go build -trimpath -ldflags "$(LDFLAGS) -s -w" -o $(DIST)/$(BINARY) ./cmd/devicedeck
	cp sidecar/.build/release/devicedeck-hid sidecar/.build/release/devicedeck-video $(DIST)/
	# Scrub any absolute local paths (embedded dep contents, Swift build paths)
	# and fail the build if any survive. Runs before signing.
	./scripts/redact-local-paths.sh $(DIST)
	# The archive is a distribution, so it carries the terms with it:
	# Apache-2.0 asks that recipients get the licence, and the upstream
	# notices travel with the sidecars they describe.
	cp LICENSE ATTRIBUTION.md README.md $(DIST)/

# sign signs and notarizes the staged binaries in place (a no-op without
# DEVELOPER_ID, so it is safe to call on a dev machine).
sign:
	./scripts/macos-sign-notarize.sh $(DIST)

# package tars the staged (and possibly signed) dir for distribution.
package:
	cd dist && tar czf $(notdir $(DIST)).tar.gz $(notdir $(DIST))
	@echo "packaged $(DIST).tar.gz"

# release: unsigned archive (local/dev). release-signed: sign + notarize the
# binaries before tarring, so the shipped archive is Gatekeeper-clean.
release: stage package
release-signed: stage sign package

clean:
	rm -f $(BINARY) coverage.out
	rm -rf sidecar/.build dist
