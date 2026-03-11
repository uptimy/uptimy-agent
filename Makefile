BINARY_NAME := uptimy-agent
PKG := github.com/uptimy/uptimy-agent
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) \
           -X $(PKG)/internal/version.Commit=$(COMMIT) \
           -X $(PKG)/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: all build test lint run clean docker-build fmt vet dist

all: lint test build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/agent

run: build
	./bin/$(BINARY_NAME) run --config configs/default.yaml

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/ coverage.out

docker-build:
	docker build -t uptimy/agent:$(VERSION) .

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

tidy:
	go mod tidy

# Build release tarballs for supported platforms.
dist: clean
	@mkdir -p dist
	@for os in linux darwin; do \
		for arch in amd64 arm64; do \
			echo "Building $$os/$$arch..."; \
			GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
				-o dist/$(BINARY_NAME) ./cmd/agent; \
			cp configs/default.yaml dist/config.yaml; \
			cp deploy/systemd/uptimy-agent.service dist/uptimy-agent.service; \
			tar -czf dist/$(BINARY_NAME)_$(VERSION)_$${os}_$${arch}.tar.gz \
				-C dist $(BINARY_NAME) config.yaml uptimy-agent.service; \
			rm -f dist/$(BINARY_NAME) dist/config.yaml dist/uptimy-agent.service; \
		done; \
	done
	@echo "Release tarballs in dist/"

.DEFAULT_GOAL := build
