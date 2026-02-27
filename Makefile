APP_NAME := appordown
VERSION ?= dev
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
REVISION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")

# Link-time metadata injected into cmd/appordown/main.go.
LD_FLAGS := -X main.version=$(VERSION)
LD_FLAGS += -X main.date=$(BUILD_DATE)
LD_FLAGS += -X main.revision=$(REVISION)

.PHONY: help
help:
	@echo "Targets:"
	@echo "  build    Build $(APP_NAME) with embedded version metadata"
	@echo "  run      Build and run $(APP_NAME)"

.PHONY: build
build:
	CGO_ENABLED=0 go build \
		-ldflags "$(LD_FLAGS)" \
		-o $(APP_NAME) \
		./cmd/appordown

.PHONY: run
run: build
	./$(APP_NAME)
