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

[Install]
WantedBy=multi-user.target
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

## 3. SSL (Certbot)

It is highly recommended to use HTTPS:
```bash
sudo certbot --nginx -d api.plugins.example.com
```
