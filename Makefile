.PHONY: build clean test lint package deploy help trivy-scan

GOATFLOW_DIR := $(shell realpath ../goatflow 2>/dev/null || echo ../goatflow)
-include $(GOATFLOW_DIR)/.env
export

BINARY := kanban
PLUGIN_BINARY := kanban
PACKAGE_NAME := goat-kanban
BUILD_DIR := bin
CMD_DIR := ./cmd/kanban-plugin
RAW_VERSION := $(shell sed -n "s/^version:[ ]*//p" plugin.yaml | head -n 1 | tr -d '"')
VERSION := $(if $(RAW_VERSION),$(RAW_VERSION),dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)
GOATFLOW_URL ?= http://localhost:8080
GO_IMAGE ?= golang:1.25.12-alpine

build:
	@echo "Building $(PLUGIN_BINARY) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	docker run --rm \
		-u "$$(id -u):$$(id -g)" \
		-v "$(CURDIR)":/src \
		-v "$(GOATFLOW_DIR)":/goatflow \
		-w /src \
		-e GOCACHE=/src/.gocache \
		-e GOTOOLCHAIN=auto \
		-e GOMODCACHE=/src/.gomod \
		-e GOTMPDIR=/src/.tmp \
		$(GO_IMAGE) sh -c "mkdir -p /src/.gocache /src/.gomod /src/.tmp && go mod download && go build -buildvcs=false -ldflags='$(LDFLAGS)' -o $(BUILD_DIR)/$(PLUGIN_BINARY) $(CMD_DIR)"

clean:
	@rm -rf $(BUILD_DIR)

test:
	@echo "Running tests..."
	@docker run --rm \
		-u "$$(id -u):$$(id -g)" \
		-v "$(CURDIR)":/src \
		-v "$(GOATFLOW_DIR)":/goatflow \
		-w /src \
		-e GOCACHE=/src/.gocache \
		-e GOTOOLCHAIN=auto \
		-e GOMODCACHE=/src/.gomod \
		-e GOTMPDIR=/src/.tmp \
		$(GO_IMAGE) sh -c "mkdir -p /src/.gocache /src/.gomod /src/.tmp && go test -buildvcs=false -v -count=1 ./..."

## lint: Enforce database-agnostic SQL (MySQL + PostgreSQL) via gk-sql-lint
lint:
	@echo "🔍  Linting SQL portability (gk-sql-lint)..."
	@docker run --rm \
		-u "$$(id -u):$$(id -g)" \
		-v "$(CURDIR)":/src \
		-w /src \
		-e GOCACHE=/src/.gocache \
		-e GOTOOLCHAIN=auto \
		-e GOMODCACHE=/src/.gomod \
		-e GOTMPDIR=/src/.tmp \
		-e HOME=/tmp \
		$(GO_IMAGE) sh -c "mkdir -p /src/.gocache /src/.gomod /src/.tmp && go run github.com/goatkit/sql-lint@v0.2.0 /src"

package: build
	@echo "Packaging $(BINARY) $(VERSION)..."
	@rm -f $(BUILD_DIR)/$(PACKAGE_NAME).zip
	@cd $(BUILD_DIR) && cp ../plugin.yaml . && zip -q -r $(PACKAGE_NAME).zip $(PLUGIN_BINARY) plugin.yaml && rm -f plugin.yaml
	@echo "Package: $(BUILD_DIR)/$(PACKAGE_NAME).zip"

## sign: Sign the plugin package with an ed25519 key (KEY=<hex-private-key> or GOATFLOW_SIGNING_KEY env)
sign: package
	@KEY="$(KEY)"; \
	if [ -z "$$KEY" ]; then KEY=$${GOATFLOW_SIGNING_KEY:-}; fi; \
	if [ -z "$$KEY" ]; then \
		echo "ERROR: No signing key. Generate one with: make keygen"; \
		echo "Or set KEY=<hex-private-key> or GOATFLOW_SIGNING_KEY env var."; \
		exit 1; \
	fi; \
	docker run --rm \
		-u "$$(id -u):$$(id -g)" \
		-v "$(CURDIR)":/src \
		-v "$(GOATFLOW_DIR)":/goatflow \
		-w /goatflow \
		-e GOCACHE=/src/.gocache \
		-e GOTOOLCHAIN=auto \
		-e GOMODCACHE=/src/.gomod \
		-e GOTMPDIR=/src/.tmp \
		$(GO_IMAGE) sh -c "mkdir -p /src/.gocache /src/.gomod /src/.tmp && go run ./cmd/gk sign /src/$(BUILD_DIR)/$(PACKAGE_NAME).zip --key $$KEY"

## keygen: Generate a new ed25519 signing key pair
keygen:
	@docker run --rm \
		-v "$(GOATFLOW_DIR)":/goatflow \
		-w /goatflow \
		-e GOCACHE=/src/.gocache \
		-e GOTOOLCHAIN=auto \
		-e GOMODCACHE=/src/.gomod \
		-e GOTMPDIR=/src/.tmp \
		$(GO_IMAGE) sh -c "mkdir -p /src/.gocache /src/.gomod /src/.tmp && go run ./cmd/gk keys generate"

deploy: package
	@test -n "$(ADMIN_API_KEY)$(ADMIN_PASSWORD)" || { echo "ERROR: ADMIN_API_KEY or ADMIN_PASSWORD missing from $(GOATFLOW_DIR)/.env"; exit 1; }
	@echo "Deploying $(BINARY) to $(GOATFLOW_URL)..."
	@TOKEN="$(ADMIN_API_KEY)"; \
	RESP=""; \
	if [ -n "$$TOKEN" ]; then \
		RESP=$$(curl -sk "$(GOATFLOW_URL)/api/v1/plugins/upload" -H "Authorization: Bearer $$TOKEN" -H "Accept: application/json" -F "plugin=@$(BUILD_DIR)/$(PACKAGE_NAME).zip"); \
	fi; \
	case "$$RESP" in \
		*\"Invalid\ or\ expired\ token\"*|*\"success\":false*|\"\") \
			PAYLOAD=$$(python3 -c 'import json, os; print(json.dumps({"login": os.environ.get("ADMIN_USER", ""), "password": os.environ.get("ADMIN_PASSWORD", "")}))'); \
			TOKEN=$$(curl -sk "$(GOATFLOW_URL)/api/auth/login" -H "Content-Type: application/json" -H "Accept: application/json" -d "$$PAYLOAD" | python3 -c 'import json, sys; data=json.load(sys.stdin); print(data.get("access_token") or data.get("token") or "")'); \
			if [ -z "$$TOKEN" ]; then echo "ERROR: Auth failed"; exit 1; fi; \
			RESP=$$(curl -sk "$(GOATFLOW_URL)/api/v1/plugins/upload" -H "Authorization: Bearer $$TOKEN" -H "Accept: application/json" -F "plugin=@$(BUILD_DIR)/$(PACKAGE_NAME).zip"); \
			;; \
	esac; \
	echo "$$RESP" | python3 -m json.tool 2>/dev/null || echo "$$RESP"

help:
	@echo "GoatFlow Kanban plugin"
	@echo ""
	@echo "Usage:"
	@echo "  make build                   Build the gRPC plugin binary"
	@echo "  make test                    Run plugin tests"
	@echo "  make lint                    Enforce DB-agnostic SQL (gk-sql-lint)"
	@echo "  make package                 Build and ZIP plugin.yaml + binary"
	@echo "  make sign KEY=<hex>          Package and sign with ed25519"
	@echo "  make keygen                  Generate a new ed25519 signing key pair"
	@echo "  make deploy                  Package and upload via GoatFlow API"
	@echo "  make deploy GOATFLOW_URL=..  Deploy to a specific GoatFlow instance"
