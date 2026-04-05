# mattermost-plugin-filebrowser — local development Makefile.
#
# The CI workflows do the full cross-platform build matrix; this Makefile is
# for running tests, linting, and producing a plugin bundle against the host
# OS/arch so you can iterate quickly.

.DEFAULT_GOAL := help

VERSION := $(shell cat VERSION)
PLUGIN_ID := $(shell jq -r .id plugin.json)
HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)
HOST_TARGET := $(HOST_GOOS)-$(HOST_GOARCH)

## help: show this help
.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## test: run all Go unit tests
.PHONY: test
test:
	go test -race ./...

## vet: run go vet
.PHONY: vet
vet:
	go vet ./...

## cover: produce coverage report and open it
.PHONY: cover
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## build: build the server binary for the host OS/arch
.PHONY: build
build:
	mkdir -p server/dist
	go build -trimpath -o server/dist/plugin-$(HOST_TARGET) ./server

## dist: build host binary and bundle a host-only plugin tarball for local testing
.PHONY: dist
dist: build
	rm -rf bundle
	mkdir -p bundle/server/dist
	cp server/dist/plugin-$(HOST_TARGET) bundle/server/dist/
	cp plugin.json bundle/
	[ -f assets/icon.svg ] && mkdir -p bundle/assets && cp assets/icon.svg bundle/assets/ || true
	tar czf dist/mattermost-plugin-filebrowser-$(VERSION).tar.gz -C bundle .
	@echo "Plugin bundle: dist/mattermost-plugin-filebrowser-$(VERSION).tar.gz"

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf server/dist bundle dist coverage.out

## version: print the version read from VERSION
.PHONY: version
version:
	@echo $(VERSION)
