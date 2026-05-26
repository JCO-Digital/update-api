CREATE TABLE IF NOT EXISTS plugins (
    slug TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    secret TEXT NOT NULL,
    is_paid BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_slug TEXT NOT NULL,
    version TEXT NOT NULL,
    download_url TEXT NOT NULL,
    requires_wp TEXT,
    tested_wp TEXT,
    requires_php TEXT,
    changelog TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (plugin_slug) REFERENCES plugins(slug) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_versions_plugin_slug ON versions(plugin_slug);
