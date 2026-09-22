.PHONY: build test lint lint-js quality cover-gaps hooks vet clean sidecar sidecar-test release release-signed release-all stage sign package drivers

BINARY := devicedeck
PKG := github.com/devicelab-dev/DeviceDeck
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT)
# ARCH is the macOS architecture a release is built for: arm64 or x86_64 (the
# names the install script uses). It defaults to this Mac's, and release-all
# builds both on one machine.
ARCH ?= $(shell uname -m)
GOARCH := $(if $(filter x86_64,$(ARCH)),amd64,arm64)
DIST := dist/$(BINARY)-$(VERSION)-darwin-$(ARCH)
# RELEASE_DIR is the upload layout the install script downloads from:
# devicedeck/<version>/<archive> with a <archive>.sha256 beside each.
RELEASE_DIR := dist/$(BINARY)/$(VERSION)
SIDECAR_BIN = $(shell swift build --package-path sidecar -c release --arch $(ARCH) --show-bin-path)

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

# lint-js lints and type-checks the console and device page scripts
# (internal/web/static). Needs `npm install` once; the files are served as-is.
lint-js:
	npm run lint:js
	npm run typecheck:device

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

# drivers re-copies the Android driver APKs from the pinned maestro-runner
# module into the binary's embed folder. Run it after every maestro-runner
# bump; a test fails until the two match.
RUNNER_MOD := github.com/devicelab-dev/maestro-runner
drivers:
	@src="$$(go list -m -f '{{.Dir}}' $(RUNNER_MOD))/drivers/android"; \
	for apk in devicelab-android-driver.apk devicelab-android-driver-test.apk; do \
		install -m 0644 "$$src/$$apk" internal/home/android/$$apk; \
	done; echo "synced driver APKs from $$src"

# stage lays the archive out the way it is installed: the three binaries in
# bin/ (the server finds each sidecar next to its own executable), the
# licence files beside it. The Android driver is embedded in the binary, so
# nothing else ships. macOS only — the sidecars talk to CoreSimulator.
stage:
	swift build --package-path sidecar -c release --arch $(ARCH)
	GOOS=darwin GOARCH=$(GOARCH) CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS) -s -w" -o $(DIST)/bin/$(BINARY) ./cmd/devicedeck
	cp $(SIDECAR_BIN)/devicedeck-hid $(SIDECAR_BIN)/devicedeck-video $(DIST)/bin/
	# Scrub any absolute local paths (embedded dep contents, Swift build paths)
	# and fail the build if any survive. Runs before signing.
	./scripts/redact-local-paths.sh $(DIST)/bin
	# The archive is a distribution, so it carries the terms with it:
	# Apache-2.0 asks that recipients get the licence, and the upstream
	# notices travel with the sidecars they describe.
	cp LICENSE ATTRIBUTION.md README.md $(DIST)/

# sign signs and notarizes the staged binaries in place (a no-op without
# DEVELOPER_ID, so it is safe to call on a dev machine).
sign:
	./scripts/macos-sign-notarize.sh $(DIST)/bin

# package tars the staged (and possibly signed) dir for distribution, and
# copies it into RELEASE_DIR with its checksum, which the install script
# verifies ("<sha256>  <archive>", as shasum writes it).
package:
	cd dist && tar czf $(notdir $(DIST)).tar.gz $(notdir $(DIST))
	mkdir -p $(RELEASE_DIR)
	cp $(DIST).tar.gz $(RELEASE_DIR)/
	cd $(RELEASE_DIR) && shasum -a 256 $(notdir $(DIST)).tar.gz > $(notdir $(DIST)).tar.gz.sha256
	@echo "packaged $(RELEASE_DIR)/$(notdir $(DIST)).tar.gz (+ .sha256)"

# release and release-signed both sign before tarring: with DEVELOPER_ID set
# the binaries are signed and notarized (Gatekeeper-clean); without it they
# are ad-hoc signed, which redaction makes mandatory — Apple Silicon kills a
# binary whose bytes no longer match its signature.
release: stage sign package
release-signed: release

# release-all builds, signs, notarizes and packages both macOS architectures
# on this Mac, into RELEASE_DIR, ready to upload — the maestro-runner release
# shape. Signing uses scripts/macos-sign-notarize.sh, so it reads the
# Developer ID and notarization credentials from the environment or the
# keychain, never from this file; without them the archives are ad-hoc signed.
#   make release-all VERSION=0.1.1
release-all:
	@if [ "$(VERSION)" = dev ]; then echo "set a version: make release-all VERSION=0.1.1"; exit 1; fi
	rm -rf $(RELEASE_DIR)
	$(MAKE) release VERSION=$(VERSION) ARCH=arm64
	$(MAKE) release VERSION=$(VERSION) ARCH=x86_64
	@echo; echo "ready to upload:"; ls -l $(RELEASE_DIR)

clean:
	rm -f $(BINARY) coverage.out
	rm -rf sidecar/.build dist
