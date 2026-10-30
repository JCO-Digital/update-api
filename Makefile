# Makefile for WordPress Plugin Update API

# Directories
BIN_DIR=bin

# Binary names
SERVER_BINARY=$(BIN_DIR)/update-api
CLI_BINARY=$(BIN_DIR)/update-cli

# Build flags
LDFLAGS=-ldflags="-s -w"

# Install locations, matching deployment/update-api.service
DESTDIR=
INSTALL_DIR=/var/www/update-api
SYSTEMD_DIR=/etc/systemd/system
SERVICE_FILE=deployment/update-api.service
CONFIG_DIR=/etc/update-api
# The config holds the admin API keys: readable by root and the service group
CONFIG_OWNER=root
CONFIG_GROUP=www-data

.PHONY: all build build-server build-cli clean run test backup install help

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

# Doesn't build, so `sudo make install` uses the binaries from `make build`
# instead of compiling as root.
install: ## Install the binaries, systemd unit and config (run make build first)
	@test -f $(SERVER_BINARY) -a -f $(CLI_BINARY) || { echo "Run 'make build' first"; exit 1; }
	install -d $(DESTDIR)$(INSTALL_DIR)
	install -m 0755 $(SERVER_BINARY) $(CLI_BINARY) $(DESTDIR)$(INSTALL_DIR)/
	install -D -m 0644 $(SERVICE_FILE) $(DESTDIR)$(SYSTEMD_DIR)/update-api.service
	install -d -m 0750 -o $(CONFIG_OWNER) -g $(CONFIG_GROUP) $(DESTDIR)$(CONFIG_DIR)
	@# Never overwrite a config that has real keys in it
	test -e $(DESTDIR)$(CONFIG_DIR)/config.yaml || \
		install -m 0640 -o $(CONFIG_OWNER) -g $(CONFIG_GROUP) config.yaml.dist $(DESTDIR)$(CONFIG_DIR)/config.yaml
	@echo "Run 'sudo systemctl daemon-reload && sudo systemctl restart update-api' to apply"

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
