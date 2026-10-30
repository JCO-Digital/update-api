# Deployment Guide

This guide explains how to deploy the API as a background service behind Nginx.

## 1. Systemd Service

Build as your own user, then install the binaries to `/var/www/update-api/`, the unit to `/etc/systemd/system/` and `config.yaml.dist` to `/etc/update-api/config.yaml`:

```bash
make build
sudo make install
```

`make install` doesn't build, so Go never runs as root, and it never overwrites an existing config. Override `INSTALL_DIR`, `SYSTEMD_DIR`, `CONFIG_DIR`, `CONFIG_GROUP` or `DESTDIR` to install elsewhere. If you change `INSTALL_DIR`, change `ExecStart` in the unit to match.

The installed unit, `deployment/update-api.service`, is:

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

The config lives in `/etc/update-api/config.yaml`. The server and the CLI both look there, so neither needs extra environment. It holds the admin API keys, so only root and the service user may read it. `make install` sets that up; by hand it is:

```bash
sudo install -d -m 0750 -o root -g www-data /etc/update-api
sudo install -m 0640 -o root -g www-data config.yaml.dist /etc/update-api/config.yaml
```

### Upgrading from v1.0.0

v1.0.0 used `/etc/xdg/update-api/config.yaml`. That location still works and is searched first, so move the file rather than copying it, or the old copy keeps winning (the server logs a warning when both exist):

```bash
sudo install -d -m 0750 -o root -g www-data /etc/update-api
sudo mv /etc/xdg/update-api/config.yaml /etc/update-api/config.yaml
sudo rmdir /etc/xdg/update-api
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
