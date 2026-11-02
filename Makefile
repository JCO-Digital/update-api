# Makefile for WordPress Plugin Update API

# Directories
BIN_DIR=bin

# Binary names
SERVER_BINARY=$(BIN_DIR)/update-api
CLI_BINARY=$(BIN_DIR)/update-cli
REGISTER_BINARY=$(BIN_DIR)/jcore-register-plugin

# Release version, from the tag (v1.2.0 -> 1.2.0) unless given
VERSION ?= $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
# Platforms the release ships jcore-register-plugin for
REGISTER_PLATFORMS=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
DIST_DIR=dist

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

.PHONY: all build build-server build-cli build-register dist-register clean run test backup install help

all: build ## Build both server and cli

build: build-server build-cli ## Build both binaries

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-server: $(BIN_DIR) ## Build the API server
	go build $(LDFLAGS) -o $(SERVER_BINARY) ./cmd/update-api

build-cli: $(BIN_DIR) ## Build the management CLI
	go build $(LDFLAGS) -o $(CLI_BINARY) ./cmd/update-cli

build-register: $(BIN_DIR) ## Build jcore-register-plugin for this machine
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(REGISTER_BINARY) ./cmd/jcore-register-plugin

# One archive per platform plus SHA256SUMS, as attached to GitHub releases.
# The binary sits at the archive root, named jcore-register-plugin(.exe).
dist-register: ## Cross-compile jcore-register-plugin release archives into dist/
	rm -rf $(DIST_DIR) && mkdir -p $(DIST_DIR)
	@set -e; for platform in $(REGISTER_PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		name=jcore-register-plugin_$(VERSION)_$${os}_$${arch}; \
		exe=jcore-register-plugin; [ $$os = windows ] && exe=$$exe.exe; \
		mkdir -p $(DIST_DIR)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
			-o $(DIST_DIR)/$$name/$$exe ./cmd/jcore-register-plugin; \
		if [ $$os = windows ]; then \
			(cd $(DIST_DIR)/$$name && zip -q ../$$name.zip $$exe); \
		else \
			tar -C $(DIST_DIR)/$$name -czf $(DIST_DIR)/$$name.tar.gz $$exe; \
		fi; \
		rm -rf $(DIST_DIR)/$$name; \
		echo "built $$name"; \
	done
	cd $(DIST_DIR) && sha256sum *.tar.gz *.zip > SHA256SUMS

run: build-server ## Build and run the server
	$(SERVER_BINARY)

# Never delete the database here: clean runs wherever the repo is checked
# out, and that includes production.
clean: ## Remove build output
	rm -rf $(BIN_DIR) $(DIST_DIR)

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
