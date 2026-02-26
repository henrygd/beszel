# Default OS/ARCH values
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
# Skip building the web UI if true
SKIP_WEB ?= false
# Controls NVML/glibc agent build tag behavior:
# - auto (default): enable on linux/amd64 glibc hosts
# - true: always enable
# - false: always disable
NVML ?= auto

# Detect glibc host for local linux/amd64 builds.
HOST_GLIBC := $(shell \
	if [ "$(OS)" = "linux" ] && [ "$(ARCH)" = "amd64" ]; then \
		for p in /lib64/ld-linux-x86-64.so.2 /lib/x86_64-linux-gnu/ld-linux-x86-64.so.2 /lib/ld-linux-x86-64.so.2; do \
			[ -e "$$p" ] && { echo true; exit 0; }; \
		done; \
		if command -v ldd >/dev/null 2>&1; then \
			if ldd --version 2>&1 | tr '[:upper:]' '[:lower:]' | awk '/gnu libc|glibc/{found=1} END{exit !found}'; then \
				echo true; \
			else \
				echo false; \
			fi; \
		else \
			echo false; \
		fi; \
	else \
		echo false; \
	fi)

# Enable glibc build tag for NVML on supported Linux builds.
AGENT_GO_TAGS :=
ifeq ($(NVML),true)
AGENT_GO_TAGS := -tags glibc
else ifeq ($(NVML),auto)
ifeq ($(HOST_GLIBC),true)
AGENT_GO_TAGS := -tags glibc
endif
endif

# Set executable extension based on target OS
EXE_EXT := $(if $(filter windows,$(OS)),.exe,)

.PHONY: tidy build-agent build-hub build-hub-dev build clean lint dev-server dev-agent dev-hub dev generate-locales fetch-smartctl-conditional
.DEFAULT_GOAL := build

clean:
	go clean
	rm -rf ./build

lint:
	golangci-lint run

test:
	go test -tags=testing ./...

tidy:
	go mod tidy

build-web-ui:
	@if command -v bun >/dev/null 2>&1; then \
		bun install --cwd ./internal/site && \
		bun run --cwd ./internal/site build; \
	else \
		npm install --prefix ./internal/site && \
		npm run --prefix ./internal/site build; \
	fi

# Conditional .NET build - only for Windows
build-dotnet-conditional:
	@if [ "$(OS)" = "windows" ]; then \
		echo "Building .NET executable for Windows..."; \
		if command -v dotnet >/dev/null 2>&1; then \
			rm -rf ./agent/lhm/bin; \
			dotnet build -c Release ./agent/lhm/beszel_lhm.csproj; \
		else \
			echo "Error: dotnet not found. Install .NET SDK to build Windows agent."; \
			exit 1; \
		fi; \
	fi

# Download smartctl.exe at build time for Windows (skips if already present)
fetch-smartctl-conditional:
	@if [ "$(OS)" = "windows" ]; then \
		go generate -run fetchsmartctl ./agent; \
	fi

# Update build-agent to include conditional .NET build
build-agent: tidy build-dotnet-conditional fetch-smartctl-conditional
	GOOS=$(OS) GOARCH=$(ARCH) go build $(AGENT_GO_TAGS) -o ./build/beszel-agent_$(OS)_$(ARCH)$(EXE_EXT) -ldflags "-w -s" ./internal/cmd/agent

build-hub: tidy $(if $(filter false,$(SKIP_WEB)),build-web-ui)
	GOOS=$(OS) GOARCH=$(ARCH) go build -o ./build/beszel_$(OS)_$(ARCH)$(EXE_EXT) -ldflags "-w -s" ./internal/cmd/hub

build-hub-dev: tidy
	mkdir -p ./internal/site/dist && touch ./internal/site/dist/index.html
	GOOS=$(OS) GOARCH=$(ARCH) go build -tags development -o ./build/beszel-dev_$(OS)_$(ARCH)$(EXE_EXT) -ldflags "-w -s" ./internal/cmd/hub

build: build-agent build-hub

generate-locales:
	@if [ ! -f ./internal/site/src/locales/en/en.ts ]; then \
		echo "Generating locales..."; \
		command -v bun >/dev/null 2>&1 && cd ./internal/site && bun install && bun run sync || cd ./internal/site && npm install && npm run sync; \
	fi

dev-server: generate-locales
	cd ./internal/site
	@if command -v bun >/dev/null 2>&1; then \
		cd ./internal/site && bun run dev --host 0.0.0.0; \
	else \
		cd ./internal/site && npm run dev --host 0.0.0.0; \
	fi

dev-hub: export ENV=dev
dev-hub:
	mkdir -p ./internal/site/dist && touch ./internal/site/dist/index.html
	@if command -v entr >/dev/null 2>&1; then \
		find ./internal -type f -name '*.go' | entr -r -s "cd ./internal/cmd/hub && go run -tags development . serve --http 0.0.0.0:8090"; \
	else \
		cd ./internal/cmd/hub && go run -tags development . serve --http 0.0.0.0:8090; \
	fi

dev-agent:
	@if command -v entr >/dev/null 2>&1; then \
		find ./internal/cmd/agent/*.go ./agent/*.go | entr -r go run $(AGENT_GO_TAGS) github.com/henrygd/beszel/internal/cmd/agent; \
	else \
		go run $(AGENT_GO_TAGS) github.com/henrygd/beszel/internal/cmd/agent; \
	fi
	
build-dotnet:
	@if command -v dotnet >/dev/null 2>&1; then \
		rm -rf ./agent/lhm/bin; \
		dotnet build -c Release ./agent/lhm/beszel_lhm.csproj; \
	else \
		echo "dotnet not found"; \
	fi


# KEY="..." make -j dev
dev: dev-server dev-hub dev-agent
