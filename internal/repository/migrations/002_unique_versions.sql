-- Publishing the same version twice (a re-run release workflow, or the GitHub
-- import) used to add a duplicate row. Keep the newest row of each pair and
-- let the unique index turn later inserts into updates.
DELETE FROM versions
WHERE id NOT IN (
    SELECT MAX(id) FROM versions GROUP BY plugin_slug, version
);

DROP INDEX IF EXISTS idx_versions_plugin_slug;
CREATE UNIQUE INDEX IF NOT EXISTS idx_versions_plugin_slug_version ON versions(plugin_slug, version);
