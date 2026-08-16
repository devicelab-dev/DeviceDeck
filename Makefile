.PHONY: build test lint clean sidecar sidecar-test

BINARY := devicedeck
PKG := github.com/devicelab-dev/DeviceDeck

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

clean:
	rm -f $(BINARY) coverage.out
	rm -rf sidecar/.build
