
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
# ~                                        ~
# ~   ██████╗  ██████╗ ██████╗  ██████╗    ~
# ~   ██╔══██╗██╔═══██╗██╔══██╗██╔═══██╗   ~
# ~   ██████╔╝██║   ██║██████╔╝██║   ██║   ~
# ~   ██╔═══╝ ██║   ██║██╔══██╗██║   ██║   ~
# ~   ██║     ╚██████╔╝██████╔╝╚██████╔╝   ~
# ~   ╚═╝      ╚═════╝ ╚═════╝  ╚═════╝    ~
# ~                                        ~
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

all: compile

version     ?=  0.0.1
target      ?=  pobo
org         ?=  pobo-giova
authorname  ?=  Paolo Giovannini
authoremail ?=  paolo.giovannini@skiff.com
license     ?=  MIT
year        ?=  2023
copyright   ?=  Copyright (c) $(year)

local_docker_repo ?= 127.0.0.1:5000

compile: ## Compile for the local architecture ⚙
	@echo "Compiling..."
	go build -ldflags "\
	-X 'github.com/$(org)/$(target).Version=$(version)' \
	-X 'github.com/$(org)/$(target).AuthorName=$(authorname)' \
	-X 'github.com/$(org)/$(target).AuthorEmail=$(authoremail)' \
	-X 'github.com/$(org)/$(target).Copyright=$(copyright)' \
	-X 'github.com/$(org)/$(target).License=$(license)' \
	-X 'github.com/$(org)/$(target).Name=$(target)'" \
	-o $(target) cmd/*.go

install: ## Install the program to /usr/bin 🎉
	@echo "Installing..."
	sudo cp $(target) /usr/bin/$(target)

test: clean compile install ## 🤓 Run go tests
	@echo "Testing..."
	go test -v ./...

clean: ## Clean your artifacts 🧼
	@echo "Cleaning..."
	rm -rvf release/*

.PHONY: release
release: ## Make the binaries for a GitHub release 📦
	mkdir -p release
	GOOS="linux" GOARCH="amd64" go build -ldflags "-X 'github.com/$(org)/$(target).Version=$(version)'" -o release/$(target)-linux-amd64 cmd/*.go
	GOOS="linux" GOARCH="arm" go build -ldflags "-X 'github.com/$(org)/$(target).Version=$(version)'" -o release/$(target)-linux-arm cmd/*.go
	GOOS="linux" GOARCH="arm64" go build -ldflags "-X 'github.com/$(org)/$(target).Version=$(version)'" -o release/$(target)-linux-arm64 cmd/*.go
	GOOS="linux" GOARCH="386" go build -ldflags "-X 'github.com/$(org)/$(target).Version=$(version)'" -o release/$(target)-linux-386 cmd/*.go
	GOOS="darwin" GOARCH="amd64" go build -ldflags "-X 'github.com/$(org)/$(target).Version=$(version)'" -o release/$(target)-darwin-amd64 cmd/*.go

.PHONY: help
help:  ## 🤔 Show help messages for make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: ko-init
ko-init: ## 📦📦📦 ko init image and manifest to work with local kind cluster and registry
	@echo "Initilizing ko..."
	export KO_DOCKER_REPO=$(local_docker_repo)
	ko build cmd/main.go
# ko resolve -f config/deploy.yaml > config/release.yaml

.PHONY: ko-run
ko-run:
	ko apply -f config/

.PHONY: ko-del
ko-del:
	ko delete -f config/

.PHONY: local-repo
local-repo:
	docker run -d --net=kind --restart=always -p "$(local_docker_repo):5000" --name "kind-registry" registry:2

.PHONY: install-hooks
install-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"
	@echo "Pre-push hook will run: make fmt-check lint test"

.PHONY: vet
vet:
	$Q go vet -mod=vendor github.com/cloudflare/cloudflared/...

.PHONY: fmt
fmt:
	@goimports -l -w -local github.com/cloudflare/cloudflared $$(go list -mod=vendor -f '{{.Dir}}' -a ./... | fgrep -v tunnelrpc/proto)
	@go fmt $$(go list -mod=vendor -f '{{.Dir}}' -a ./... | fgrep -v tunnelrpc/proto)

.PHONY: fmt-check
fmt-check:
	@./.ci/scripts/fmt-check.sh

.PHONY: lint
lint:
	@golangci-lint run

.PHONY: test
test: vet
	$Q go test -json -v -mod=vendor -race $(LDFLAGS) ./... 2>&1 | tee $(GO_TEST_LOG_OUTPUT)
ifneq ($(FIPS), true)
	@go run -mod=readonly github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest -input $(GO_TEST_LOG_OUTPUT)
endif

.PHONY: cover
cover:
	@echo ""
	@echo "=====> Total test coverage: <====="
	@echo ""
	# Print the overall coverage here for quick access.
	$Q go tool cover -func ".cover/c.out" | grep "total:" | awk '{print $$3}'
	# Generate the HTML report that can be viewed from the browser in CI.
	$Q go tool cover -html ".cover/c.out" -o .cover/all.html