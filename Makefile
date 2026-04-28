VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
BINARY   := soulcode

.PHONY: build test lint vet clean install tidy release-dry

## build: compile a binary for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

## install: install to $GOPATH/bin
install:
	go install -ldflags "$(LDFLAGS)" .

## test: run tests with race detector
test:
	go test -race -count=1 ./...

## cover: run tests and open coverage report
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## lint: run golangci-lint (requires golangci-lint installed)
lint:
	golangci-lint run

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy and verify go.mod
tidy:
	go mod tidy
	go mod verify

## clean: remove build artifacts
clean:
	rm -f $(BINARY) coverage.out

## release-dry: dry-run goreleaser
release-dry:
	goreleaser release --snapshot --clean

help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
