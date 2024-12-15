# Default OS/ARCH values
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
# Skip building the web UI if true
SKIP_WEB ?= false

.PHONY: tidy build-agent build-hub build clean lint dev-server dev-agent dev-hub dev
.DEFAULT_GOAL := build

clean:
	go clean
	rm -rf ./build

lint:
	golangci-lint run

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

dev-server:
	cd ./site
	@if command -v bun >/dev/null 2>&1; then \
		cd ./site && bun install && bun run dev; \
	else \
		cd ./site && npm install && npm run dev; \
	fi

dev-hub:
	@if command -v entr >/dev/null 2>&1; then \
		find ./cmd/hub/*.go ./internal/{alerts,hub,records,users}/*.go | entr -r -s "cd ./cmd/hub && go run . serve"; \
	else \
		cd ./cmd/hub && go run . serve; \
	fi

dev-agent:
	@if command -v entr >/dev/null 2>&1; then \
		find ./cmd/agent/*.go ./internal/agent/*.go | entr -r go run beszel/cmd/agent; \
	else \
		go run beszel/cmd/agent; \
	fi

# KEY="..." make -j dev
dev: dev-server dev-hub dev-agent
