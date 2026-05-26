# Makefile for WordPress Plugin Update API

# Directories
BIN_DIR=bin

# Binary names
SERVER_BINARY=$(BIN_DIR)/update-api
CLI_BINARY=$(BIN_DIR)/update-cli

# Build flags
LDFLAGS=-ldflags="-s -w"

.PHONY: all build build-server build-cli clean run test help

all: build ## Build both server and cli

build: build-server build-cli ## Build both binaries

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-server: $(BIN_DIR) ## Build the API server
	go build $(LDFLAGS) -o $(SERVER_BINARY) ./cmd/update-api

build-cli: $(BIN_DIR) ## Build the management CLI
	go build $(LDFLAGS) -o $(CLI_BINARY) ./cmd/update-cli

run: build-server ## Build and run the server
	$(SERVER_BINARY)

clean: ## Remove binaries and sqlite database
	rm -rf $(BIN_DIR) updates.db

test: ## Run go tests
	go test ./...

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
