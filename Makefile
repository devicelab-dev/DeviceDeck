.PHONY: build test lint clean

BINARY := devicedeck
PKG := github.com/devicelab-dev/DeviceDeck

build:
	go build -o $(BINARY) ./cmd/devicedeck

test:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

lint:
	gofmt -l .
	go vet ./...

clean:
	rm -f $(BINARY) coverage.out
