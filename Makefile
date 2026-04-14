.PHONY: all clean build test lint run fmt help

.DEFAULT_GOAL:=help

BINARY_NAME:=crony
BIN_OUT_DIR?=bin

GOARCH?=$(shell go env GOARCH)
GOARM?=$(shell go env GOARM)

GO_BUILD_FLAGS?=-v
GO_BUILD_LD_FLAGS:=-w -s

GO_BUILD_OUTPUT:=$(BIN_OUT_DIR)/$(BINARY_NAME)

GOLANG_LINT_VERSION=v2.11.3

export PATH:=$(shell go env GOPATH)/bin:$(PATH)

all: build test lint ## Build binary (with tests)

clean: ## cleans output directory
	rm -rf $(BIN_OUT_DIR)/*

build: ## Build binary
	go build $(GO_BUILD_FLAGS) -ldflags="$(GO_BUILD_LD_FLAGS)" -o $(GO_BUILD_OUTPUT) .

test: ## run unit tests
	go test -race -count=1 ./...

lint: fmt ## run golangcli-lint checks
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANG_LINT_VERSION) run --timeout 5m

run: build ## Build and run binary
	./$(GO_BUILD_OUTPUT)

fmt: ## gofmt all go files
	go fmt ./...

help:  ## Shows help
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
