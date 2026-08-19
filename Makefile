.PHONY: build test lint clean sidecar sidecar-test release

BINARY := devicedeck
PKG := github.com/devicelab-dev/DeviceDeck
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT)
DIST := dist/$(BINARY)-$(VERSION)-darwin-$(shell uname -m)

build:
	go build -o $(BINARY) ./cmd/devicedeck

test:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

sidecar:
	swift build --package-path sidecar -c release

sidecar-test:
	swift test --package-path sidecar

lint:
	gofmt -l .
	go vet ./...

# release stages the three binaries side by side, which is the layout the
# server discovers by default: it looks for each sidecar next to its own
# executable before falling back to the Swift build directory. macOS only
# — the sidecars talk to CoreSimulator, so there is nothing to ship
# elsewhere.
release: sidecar
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) ./cmd/devicedeck
	cp sidecar/.build/release/devicedeck-hid sidecar/.build/release/devicedeck-video $(DIST)/
	# The archive is a distribution, so it carries the terms with it:
	# Apache-2.0 asks that recipients get the licence, and the upstream
	# notices travel with the sidecars they describe.
	cp LICENSE ATTRIBUTION.md README.md $(DIST)/
	cd dist && tar czf $(notdir $(DIST)).tar.gz $(notdir $(DIST))
	@echo "packaged $(DIST).tar.gz"

clean:
	rm -f $(BINARY) coverage.out
	rm -rf sidecar/.build dist
