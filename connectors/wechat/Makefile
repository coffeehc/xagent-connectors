VERSION ?= wechat-v0.0.1
BINARY_NAME := xagent-wechat-connector
OUTPUT_DIR ?= dist
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GIT_TAG := $(shell git describe --tags 2>/dev/null || true)
GIT_REV := $(shell git rev-parse --short HEAD 2>/dev/null || true)
BUILD_TIME := $(shell date +%y%m%d%H%M)

LDFLAGS := -w -s \
	-X main.RunMode=prod \
	-X github.com/coffeehc/boot/configuration.Version=$(VERSION) \
	-X github.com/coffeehc/boot/configuration.GitTag=$(GIT_TAG) \
	-X github.com/coffeehc/boot/configuration.GitRev=$(GIT_REV) \
	-X github.com/coffeehc/boot/configuration.BuildTime=$(BUILD_TIME)

.PHONY: build test clean

build:
	mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags '$(LDFLAGS)' -o $(OUTPUT_DIR)/$(BINARY_NAME) .

test:
	go test ./...

clean:
	rm -rf $(OUTPUT_DIR)
