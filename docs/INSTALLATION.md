# Installation Guide

## Prerequisites
- Go 1.21 or higher.
- SQLite3 (optional for CLI, the API uses a pure Go driver).

## Building from source

1.  **Clone the repository**:
    ```bash
    git clone <your-repo-url>
    cd update-api
    ```

2.  **Initialize configuration**:
    ```bash
    cp config.yaml.dist config.yaml
    ```
    Edit `config.yaml` to set your desired port, database path, and admin API keys.

3.  **Build the binary**:
    ```bash
    go build -o update-api ./cmd/update-api
    ```

## Running the API

You can start the API directly:
```bash
./update-api
```

On the first run, it will automatically create the `updates.db` SQLite file and run the necessary migrations.

## Directory Structure
- `cmd/update-api/`: Entry point.
- `internal/`: Core logic (API, Auth, Repository).
- `migrations/`: SQL schema files.
- `deployment/`: Configuration templates for Nginx and systemd.
