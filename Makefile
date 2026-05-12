SHELL := /bin/bash

.PHONY: help build build-static deps tidy fmt vet lint fix deadcode check

default: help

help:
	@echo "Targets:"
	@echo "  build         Build the binary"
	@echo "  build-static  Build the static Linux AMD64 binary"
	@echo "  deps          Download module dependencies"
	@echo "  tidy          Tidy and verify modules"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  lint          Run golangci-lint"
	@echo "  fix           Run go fix"
	@echo "  deadcode      Check for dead code"
	@echo "  check         Run the full check set"


# Build targets
build:
	@echo "Building"
	CGO_ENABLED=0 go build -o bin/gh-search cmd/gh-search/main.go

build-static:
	@echo "Building static binary"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-w -s" \
		-trimpath \
		-mod=readonly \
		-o bin/gh-search-static \
		cmd/gh-search/main.go


# Development targets
deps:
	@echo "Installing dependencies..."
	go mod download

tidy:
	@echo "Tidying and verifying dependencies..."
	go mod tidy
	go mod verify

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

lint:
	@echo "Running golangci-lint..."
	golangci-lint run

fix:
	@echo "Running go fix..."
	go fix ./...

deadcode:
	@echo "Checking for dead code..."
	deadcode ./...

check: deps tidy fmt vet lint fix deadcode
	@echo "All checks passed!"
