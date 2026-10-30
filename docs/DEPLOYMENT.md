# Deployment Guide

This guide explains how to deploy the API as a background service behind Nginx.

## 1. Systemd Service

Create a service file at `/etc/systemd/system/update-api.service`:

```ini
[Unit]
Description=WordPress Plugin Update API
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=/var/www/update-api
ExecStart=/var/www/update-api/update-api
Restart=always
# Creates /var/lib/update-api owned by www-data. With XDG_DATA_HOME=/var/lib
# the database, backups and license logs default to that directory.
StateDirectory=update-api
StateDirectoryMode=0750
Environment=XDG_DATA_HOME=/var/lib

[Install]
WantedBy=multi-user.target
```

The database, backups and license logs live in `/var/lib/update-api/`.

Put the config in `/etc/xdg/update-api/config.yaml`. That is in the default XDG search path for every user, so the server and the CLI both find it without extra environment. It holds the admin API keys, so only root and the service user may read it:

```bash
sudo mkdir -p /etc/xdg/update-api
sudo cp config.yaml.dist /etc/xdg/update-api/config.yaml
sudo chown root:www-data /etc/xdg/update-api/config.yaml
sudo chmod 0640 /etc/xdg/update-api/config.yaml
```

The CLI doesn't get the unit's environment, so set the data paths in the config as well:

```yaml
server:
  db_path: "/var/lib/update-api/updates.db"
backup:
  dir: "/var/lib/update-api/backups"
licenses:
  log_dir: "/var/lib/update-api/licenses"
```

Run the CLI as the service user, so the files it writes stay readable by the server:

```bash
sudo -u www-data /var/www/update-api/update-cli list plugins
```

**Commands:**
```bash
sudo systemctl daemon-reload
sudo systemctl enable update-api
sudo systemctl start update-api
```

## 2. Nginx Reverse Proxy

Create an Nginx configuration at `/etc/nginx/sites-available/api.plugins.example.com`:

```nginx
server {
    listen 80;
    server_name api.plugins.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Commands:**
```bash
sudo ln -s /etc/nginx/sites-available/api.plugins.example.com /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## 3. Backups

The server writes a snapshot of the database to `backup.dir` when it starts and then every `backup.interval`, keeping the newest `backup.keep` (see `config.yaml.dist`). Snapshots are named `updates-<UTC timestamp>.db` and are complete SQLite databases.

Take one by hand, for example before an upgrade:

```bash
sudo -u www-data /var/www/update-api/update-cli backup
```

These snapshots sit on the same disk as the database. Copy the backup directory somewhere else as well, for example with a nightly `rsync` or `restic` job.

The database runs in WAL mode, so `updates.db` alone is not a complete copy while the server is running. Always copy a snapshot, never the live file.

### Restoring

```bash
sudo systemctl stop update-api
cd /var/lib/update-api
sudo cp updates.db updates.db.broken   # keep what was there
sudo cp backups/updates-<timestamp>.db updates.db
sudo rm -f updates.db-wal updates.db-shm
sudo chown www-data:www-data updates.db
sudo systemctl start update-api
```

### Rebuilding from GitHub

If no backup is usable, the plugins and versions can be rebuilt from the GitHub releases of every `JCO-Digital` repo with the `jcore-plugin` topic:

```bash
sudo -u www-data /var/www/update-api/update-cli import --dry-run
sudo -u www-data /var/www/update-api/update-cli import
```

Plugin secrets and the paid flag are not on GitHub. Plugins the import creates are free and have no secret; set those again with `update-cli plugin add`.

## 4. SSL (Certbot)

It is highly recommended to use HTTPS:
```bash
sudo certbot --nginx -d api.plugins.example.com
```
