# Default OS/ARCH values
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
# Skip building the web UI if true
SKIP_WEB ?= false

.PHONY: tidy build-agent build-hub build clean lint
.DEFAULT_GOAL := build

tidy:
	go mod tidy

build-web-ui:
	@if command -v bun >/dev/null 2>&1; then \
		bun install --cwd ./site && \
		bun run --cwd ./site build; \
	else \
		npm install --prefix ./site && \
		npm run --prefix ./site build; \
	fi

build-agent: tidy
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o ./build/beszel-agent_$(OS)_$(ARCH) -ldflags "-w -s" beszel/cmd/agent

build-hub: tidy $(if $(filter false,$(SKIP_WEB)),build-web-ui)
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o ./build/beszel_$(OS)_$(ARCH) -ldflags "-w -s" beszel/cmd/hub

build: build-agent build-hub

clean:
	go clean
	rm -rf ./build

lint:
	golangci-lint run
