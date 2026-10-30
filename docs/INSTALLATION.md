# Installation Guide

## Prerequisites
- Go 1.26 or higher.
- SQLite3 (optional for CLI, the API uses a pure Go driver).

## Building from source

1.  **Clone the repository**:
    ```bash
    git clone <your-repo-url>
    cd update-api
    ```

2.  **Initialize configuration**:
    ```bash
    mkdir -p ~/.config/update-api
    cp config.yaml.dist ~/.config/update-api/config.yaml
    ```
    Edit it to set your desired port and admin API keys.

3.  **Build the binary**:
    ```bash
    go build -o update-api ./cmd/update-api
    ```

## Running the API

You can start the API directly:
```bash
./update-api
```

On the first run, it will automatically create the SQLite database and run the necessary migrations.

Migrations are embedded in the binary, so only the binary and the config need to be deployed.

## File Locations

The server and the CLI follow the XDG Base Directory spec and use the same lookup.

**Config:** `$UPDATE_API_CONFIG` if set, otherwise the first `update-api/config.yaml` found in `$XDG_CONFIG_HOME` (default `~/.config`) then in each directory of `$XDG_CONFIG_DIRS` (default `/etc/xdg`), and finally `/etc/update-api/config.yaml`. If a config shadows another one further down the list, a warning is logged at startup. Without a config file the defaults are used, which have no admin API keys, so the admin endpoints reject every request.

**Data:** `$XDG_DATA_HOME/update-api/` (default `~/.local/share/update-api/`) holds:

- `updates.db`: the database
- `backups/`: database snapshots
- `licenses/`: the license `created` and `validated` logs

Set `server.db_path`, `backup.dir` and `licenses.log_dir` to put them elsewhere. Under systemd they live in `/var/lib/update-api`; see [Deployment](DEPLOYMENT.md).

Older versions kept `config.yaml`, `updates.db` and `licenses/` in the working directory. If any of them is still there and not the one in use, the server and the CLI log a warning naming where it should go. Move it there to keep its data.

## Directory Structure
- `cmd/update-api/`: Entry point.
- `cmd/update-cli/`: Management CLI.
- `internal/`: Core logic (API, Auth, Repository, Backup, GitHub importer).
- `internal/repository/migrations/`: SQL schema files, applied in order and tracked with SQLite's `user_version`.
- `deployment/`: Configuration templates for Nginx and systemd.
