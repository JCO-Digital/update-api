# CLI Tool Documentation

The `update-cli` tool allows you to manage plugins, versions, and licenses directly from the command line without needing to use the REST API.

## Building the CLI

```bash
go build -o update-cli ./cmd/update-cli
```

## Plugin Management

### Add or Update a Plugin

```bash
./update-cli plugin add --slug my-plugin --name "My Plugin" --secret "random-secret" --paid
```

- `--slug`: The WordPress slug.
- `--name`: Display name.
- `--secret`: A random string used to sign licenses.
- `--paid`: (Optional) Flag to indicate if license checks are required.

## Version Management

### Add a New Version

```bash
./update-cli version add \
  --slug my-plugin \
  --version 1.2.0 \
  --url "https://example.com/plugin-1.2.0.zip" \
  --changelog "Fixed some bugs"
```

- `--slug`: Plugin slug.
- `--version`: SemVer string.
- `--url`: Public download link.
- `--req-wp`: (Optional) Minimum WP version (default: 6.0).
- `--test-wp`: (Optional) Tested WP version (default: 6.5).

## License Management

### Generate a License Key

```bash
./update-cli license gen --slug my-plugin --cid "customer_123" --days 365
```

- `--slug`: Plugin slug.
- `--cid`: Client/Customer ID.
- `--days`: (Optional) Number of days until expiration (default: 365).

## Listing Data

### List all Plugins

```bash
./update-cli list plugins
```

### List Versions for a Plugin

```bash
./update-cli list versions --slug my-plugin
```
