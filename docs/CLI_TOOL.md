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

Running this for an existing slug updates the plugin and keeps its versions.

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

Adding a version that already exists for the plugin updates it.

## License Management

### Generate a License Key

```bash
./update-cli license gen --slug my-plugin --cid "customer_123" --days 365
```

- `--slug`: Plugin slug.
- `--cid`: Client/Customer ID.
- `--days`: (Optional) Number of days until expiration (default: 365).

## Backups

### Write a Backup Now

```bash
./update-cli backup [--dir <dir>]
```

Writes `updates-<UTC timestamp>.db` to the configured backup directory. The server also does this on its own schedule; see [Deployment](DEPLOYMENT.md#3-backups).

## Importing from GitHub

### Rebuild Plugins and Versions from Releases

```bash
./update-cli import [--org JCO-Digital] [--topic jcore-plugin] [--dry-run]
```

For every repository in the organization with the topic, this adds each release that has `<slug>.zip` attached, the same way the release workflow publishes it: the zip URL, the release notes as changelog, and the requirements from the plugin header at that tag. Running it again updates the versions in place rather than duplicating them.

Existing plugins are left as they are. New plugins are created free and without a license secret.

Set `GITHUB_TOKEN` to raise GitHub's API rate limit.

## Registering a Plugin Remotely

`update-cli` writes to the database directly, so it only works on the server. `jcore-register-plugin` does the one step a new plugin needs before its first release, over the API from any machine: it registers the slug with `POST /v1/plugins`. Without it, the release workflow's `POST /v1/versions` fails with a foreign key error (jcore-update's publish workflow fails on that since it uses `curl --fail-with-body`) and `update-check` answers "plugin not found".

Download it from the [releases](https://github.com/JCO-Digital/update-api/releases), or run it from a checkout:

```bash
gh release download -R JCO-Digital/update-api -p 'jcore-register-plugin_*_linux_amd64.tar.gz' -O - | tar xz jcore-register-plugin
UPDATE_API_KEY_FILE=~/.config/update-api/admin.key ./jcore-register-plugin --dry-run ~/projects/jcore-example

# or, from this repo
go run ./cmd/jcore-register-plugin --dry-run ~/projects/jcore-example
```

- The slug comes from the directory name and the name from the main file's `Plugin Name` header. `--slug` and `--name` override them, and work without a directory.
- The admin key comes from `$UPDATE_API_KEY`, or from a file given by `--key-file` / `$UPDATE_API_KEY_FILE`. There is no flag for the key itself, so it stays out of shell history. It is never printed.
- `--api` (or `$UPDATE_API_URL`) defaults to `https://update.jcore.fi/v1`. Plain http is refused except to localhost.
- First it asks the public `update-check` endpoint whether the slug exists. A registered slug is reported and left alone, because re-posting would replace every field, including a paid plugin's secret.
- `--dry-run` does that check and prints the request it would send, with the key redacted.
- Writing to a non-local API asks for confirmation; `--yes` skips it.
- `--paid` registers a paid plugin with the secret from `$UPDATE_PLUGIN_SECRET` or `--secret-file`, or a random one that stays on the server.

It ends with the setup it doesn't automate: the GitHub repo and its topic, secrets, the dist repo (only when `dist_deploy: true`) and branch rules.

## Listing Data

### List all Plugins

```bash
./update-cli list plugins
```

### List Versions for a Plugin

```bash
./update-cli list versions --slug my-plugin
```
