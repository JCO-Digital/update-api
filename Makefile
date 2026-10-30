# Makefile for WordPress Plugin Update API

# Directories
BIN_DIR=bin

# Binary names
SERVER_BINARY=$(BIN_DIR)/update-api
CLI_BINARY=$(BIN_DIR)/update-cli

# Build flags
LDFLAGS=-ldflags="-s -w"

.PHONY: all build build-server build-cli clean run test backup help

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

# Never delete the database here: clean runs wherever the repo is checked
# out, and that includes production.
clean: ## Remove build output
	rm -rf $(BIN_DIR)

test: ## Run go tests
	go test ./...

backup: build-cli ## Write a database backup to the configured backup dir
	$(CLI_BINARY) backup

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
