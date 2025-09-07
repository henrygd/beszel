# Default OS/ARCH values
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
# Skip building the web UI if true
SKIP_WEB ?= false

# Set executable extension based on target OS
EXE_EXT := $(if $(filter windows,$(OS)),.exe,)

.PHONY: tidy build-agent build-hub build-hub-dev build clean lint dev-server dev-agent dev-hub dev generate-locales
.DEFAULT_GOAL := build

clean:
	go clean
	rm -rf ./build

lint:
	golangci-lint run

test: export GOEXPERIMENT=synctest
test:
	go test -tags=testing ./...

tidy:
	go mod tidy

build-web-ui:
	@if command -v bun >/dev/null 2>&1; then \
		bun install --cwd ./src/site && \
		bun run --cwd ./src/site build; \
	else \
		npm install --prefix ./src/site && \
		npm run --prefix ./src/site build; \
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

# Update build-agent to include conditional .NET build
build-agent: tidy build-dotnet-conditional
	GOOS=$(OS) GOARCH=$(ARCH) go build -o ./build/beszel-agent_$(OS)_$(ARCH)$(EXE_EXT) -ldflags "-w -s" ./src/cmd/agent

build-hub: tidy $(if $(filter false,$(SKIP_WEB)),build-web-ui)
	GOOS=$(OS) GOARCH=$(ARCH) go build -o ./build/beszel_$(OS)_$(ARCH)$(EXE_EXT) -ldflags "-w -s" ./src/cmd/hub

build-hub-dev: tidy
	mkdir -p ./src/site/dist && touch ./src/site/dist/index.html
	GOOS=$(OS) GOARCH=$(ARCH) go build -tags development -o ./build/beszel-dev_$(OS)_$(ARCH)$(EXE_EXT) -ldflags "-w -s" ./src/cmd/hub

build: build-agent build-hub

generate-locales:
	@if [ ! -f ./src/site/src/locales/en/en.ts ]; then \
		echo "Generating locales..."; \
		command -v bun >/dev/null 2>&1 && cd ./src/site && bun install && bun run sync || cd ./src/site && npm install && npm run sync; \
	fi

dev-server: generate-locales
	cd ./src/site
	@if command -v bun >/dev/null 2>&1; then \
		cd ./src/site && bun run dev --host 0.0.0.0; \
	else \
		cd ./src/site && npm run dev --host 0.0.0.0; \
	fi

dev-hub: export ENV=dev
dev-hub:
	mkdir -p ./src/site/dist && touch ./src/site/dist/index.html
	@if command -v entr >/dev/null 2>&1; then \
		find ./src/cmd/hub/*.go ./src/{alerts,hub,records,users}/*.go | entr -r -s "cd ./src/cmd/hub && go run -tags development . serve --http 0.0.0.0:8090"; \
	else \
		cd ./src/cmd/hub && go run -tags development . serve --http 0.0.0.0:8090; \
	fi

dev-agent:
	@if command -v entr >/dev/null 2>&1; then \
		find ./src/cmd/agent/*.go ./agent/*.go | entr -r go run github.com/henrygd/beszel/src/cmd/agent; \
	else \
		go run github.com/henrygd/beszel/src/cmd/agent; \
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
