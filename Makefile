.PHONY: build install run test cover lint vet fmt tidy clean ci dist dist-macos-universal help

BINARY  := sighelper
GO      := go
GOFLAGS := -race
GOOS    ?= $(shell $(GO) env GOOS)
GOARCH  ?= $(shell $(GO) env GOARCH)

## build: compile the binary into bin/
build:
	mkdir -p bin/
	$(GO) build -trimpath -ldflags "-s -w" -o bin/$(BINARY) .

## install: go install into GOBIN
install:
	$(GO) install .

## run: run the resolver (pass args via ARGS=...)
run:
	$(GO) run . $(ARGS)

## test: race tests with a coverage profile
test:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

## cover: enforce the constitution's >=95% per-package coverage floor
cover: test
	bash scripts/check-coverage.sh coverage.out

## lint: golangci-lint (gosec enabled)
lint:
	golangci-lint run

## vet: go vet
vet:
	$(GO) vet ./...

## fmt: format sources (import ordering is enforced by lint)
fmt:
	gofmt -s -w .

## tidy: tidy module requirements
tidy:
	$(GO) mod tidy

## dist: build a release binary for GOOS/GOARCH (default: host) into dist/
dist:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-$(GOOS)-$(GOARCH) .

## dist-macos-universal: build a fat amd64+arm64 macOS binary (run on macOS; uses lipo)
dist-macos-universal:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-darwin-arm64 .
	lipo -create -output dist/$(BINARY)-macos-universal dist/$(BINARY)-darwin-amd64 dist/$(BINARY)-darwin-arm64

## clean: remove build artifacts
clean:
	rm -rf bin/ dist/ coverage.out

## ci: the full local gate (mirrors the CI workflow)
ci: vet lint cover
	@echo "all CI gates passed"

## help: list targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
