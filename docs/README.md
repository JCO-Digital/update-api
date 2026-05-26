# WordPress Plugin Update API Documentation

Welcome to the documentation for the WordPress Plugin Update API. This system allows you to host and manage updates for in-house WordPress plugins with support for license key validation.

## Table of Contents

1.  [Installation Guide](INSTALLATION.md) - How to build and deploy the API.
2.  [API Reference](API_REFERENCE.md) - Detailed documentation of all REST endpoints.
3.  [WordPress Integration](WORDPRESS_INTEGRATION.md) - How to connect your plugins to this API.
4.  [License System](LICENSE_SYSTEM.md) - Deep dive into the stateless HMAC license implementation.
5.  [Deployment](DEPLOYMENT.md) - Nginx and Systemd configuration.
6.  [CLI Tool](CLI_TOOL.md) - Manage plugins and licenses from the terminal.

## System Overview

The API is built in Go and uses SQLite for storage. It is designed to be lightweight, fast, and easy to run behind an Nginx proxy.

### Key Features:

- **WordPress Compatible**: Responses match the format expected by WordPress core.
- **Stateless Licenses**: Validates license keys using HMAC signatures (no DB lookup needed for validation).
- **CI/CD Ready**: Easy to integrate with GitHub Actions for automated version publishing.
- **Admin Security**: Management endpoints protected by API keys.
