.PHONY: all clean build test lint run fmt help build-e2e-image e2e-test

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

# coverage_report <profile> — prints per-file coverage and a total line.
# Parses the raw coverage profile so each file appears as one aggregated row.
define coverage_report
	@echo ""
	@echo "Coverage report ($(1)):"
	@awk 'NR>1 { sub(/:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$$/, "", $$1); s[$$1]+=$$2; if ($$3>0) c[$$1]+=$$2 } END { for (f in s) printf "  %-70s %6.1f%%  (%d/%d)\n", f, 100*c[f]/s[f], c[f], s[f] }' $(1) | sort
	@awk 'NR>1 { T+=$$2; if ($$3>0) C+=$$2 } END { printf "  %-70s %6.1f%%  (%d/%d)\n", "TOTAL", (T>0 ? 100*C/T : 0), C, T }' $(1)
endef

all: build test lint ## Build binary (with tests)

clean: ## cleans output directory
	rm -rf $(BIN_OUT_DIR)/*
	rm -f coverage.out coverage-e2e.out

build: ## Build binary
	go build $(GO_BUILD_FLAGS) -ldflags="$(GO_BUILD_LD_FLAGS)" -o $(GO_BUILD_OUTPUT) .

test: ## run unit tests
	go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	$(call coverage_report,coverage.out)

lint: fmt ## run golangcli-lint checks
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANG_LINT_VERSION) run --timeout 5m

run: build ## Build and run binary
	./$(GO_BUILD_OUTPUT)

fmt: ## gofmt all go files
	go fmt ./...

help:  ## Shows help
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build-e2e-image: ## Build crony image tagged crony-e2e:latest for end-to-end tests
	docker build -t crony-e2e:latest .

e2e-test: build-e2e-image ## Run end-to-end tests against the crony-e2e image
	CRONY_IMAGE=crony-e2e:latest go test -tags=e2e -count=1 -timeout=15m -covermode=atomic -coverpkg=./... -coverprofile=coverage-e2e.out ./e2e/...
	$(call coverage_report,coverage-e2e.out)
