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

## build: build the server binary for the host OS/arch (stub, no cgo)
## Use `make build-native` for a build that actually calls into krfiles.
.PHONY: build
build:
	mkdir -p server/dist/$(HOST_TARGET)
	CGO_ENABLED=0 go build -trimpath -o server/dist/$(HOST_TARGET)/plugin ./server

## fetch-krfiles: download krfiles native artifacts for the host target
## Populates server/fb/krf/native/ with libkrfiles.{so,dylib}, libkrfiles_api.h
## and krfiles_shim.c so a subsequent cgo build has everything it needs. Run
## this once per checkout or after switching targets.
.PHONY: fetch-krfiles
fetch-krfiles:
	./build/fetch-krfiles.sh $(HOST_TARGET)

## build-native: build the server binary with cgo against libkrfiles
## The LDFLAGS allow-list env var is required because Go's cgo rejects
## @loader_path and $ORIGIN rpath values by default; both are deliberately
## whitelisted here so the resulting binary finds its shared library next
## to itself when Mattermost activates the plugin.
.PHONY: build-native
build-native: fetch-krfiles
	mkdir -p server/dist/$(HOST_TARGET)
	CGO_ENABLED=1 \
	CGO_LDFLAGS_ALLOW='-Wl,-rpath,(@loader_path|\$$ORIGIN).*|-shared-libgcc' \
	go build -trimpath -o server/dist/$(HOST_TARGET)/plugin ./server

## dist: build native host binary and bundle a host-only plugin tarball
## The tarball ships libkrfiles.{so,dylib} alongside the plugin binary in
## server/dist/<target>/, matching the layout the runtime dynamic linker
## expects via the baked-in rpath. Per-target subdirectories keep each
## arch's shared library isolated from siblings in a multi-target bundle.
## This is not a production artifact — CI builds the full four-target
## tarball on tag push.
.PHONY: dist
dist: build-native
	rm -rf bundle dist
	mkdir -p bundle/server/dist/$(HOST_TARGET) dist
	cp server/dist/$(HOST_TARGET)/plugin bundle/server/dist/$(HOST_TARGET)/
	# Ship the shared library next to the plugin binary so @loader_path /
	# $ORIGIN rpath resolves at runtime.
	cp server/fb/krf/native/libkrfiles.* bundle/server/dist/$(HOST_TARGET)/ 2>/dev/null || true
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
