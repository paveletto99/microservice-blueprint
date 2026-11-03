.DEFAULT_GOAL := help

SERVICES    := $(notdir $(wildcard cmd/*))
VERSION     ?= dev
LDFLAGS     := -s -w
BIN_DIR     ?= bin

.PHONY: help build test fmt vet check tidy update-deps protoc protoc-check clean

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Build every cmd/* binary into bin/
	@mkdir -p $(BIN_DIR)
	@for s in $(SERVICES); do CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$s ./cmd/$$s || exit 1; done

test: ## Run tests with the race detector
	go test -race ./...

fmt: ## Check formatting
	@test -z "$$(gofmt -l cmd internal pkg test)" || (gofmt -l cmd internal pkg test; exit 1)

vet: ## Run go vet
	go vet ./...

check: fmt vet test ## fmt + vet + race tests

tidy: ## Tidy and re-vendor
	go mod tidy
	go mod vendor

update-deps: ## Upgrade dependencies
	go get -u -t ./...
	$(MAKE) tidy

protoc: ## Regenerate protobuf code
	@go install golang.org/x/tools/cmd/goimports@v0.52.0
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
	@protoc --proto_path=. --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. ./internal/pb/payment/v1/*.proto
	@goimports -w internal/pb

protoc-check: protoc ## Fail if generated protobuf code is stale
	@git diff --exit-code -- internal/pb

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
