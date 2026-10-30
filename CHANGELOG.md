# Changelog

## v1.0.0 (2026-10-30)

#### Features

- config: back up the database and move files to XDG locations (BREAKING CHANGE) (3f4bd98)

#### Maintenance

- config: add foonver configuration file (c77a4ff)

## v0.1.0 (2026-09-15)

#### Features

- api: add support for update channels in version checks (b01e93c)
- cli: add list command for plugins and versions (90a81e8)
- api: add license validation endpoint and make secret optional (80cfede)
- repository: add json tags to plugin and version models (82646c4)
- api: implement wordpress plugin update server and management cli (a8a00ca)

#### Bug Fixes

- repository: sort versions using semver to determine the latest release (af47c8b)
- auth: secure signature verification with hmac.equal (326b5f2)
- api: use constant time comparison for admin key validation (bab384a)
- api: validate request fields in HandleGenerateLicense (1353660)
- server: add timeouts to http server configuration (1f70355)
- sqlite: enable foreign key constraints on connection initialization (83c9bc0)

#### Documentation

- add project documentation (a5ba9a0)

#### Build System

- makefile: organize build artifacts into a bin directory (17b804a)

#### Maintenance

- gitignore: ignore updates.db file (1c941ac)
- add config.yaml to gitignore (d20fdb3)

