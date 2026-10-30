# API Reference

All requests should be made to the base URL of your deployment (e.g., `https://api.example.com/v1`).

## Authentication

Admin endpoints require the `X-API-Key` header. This key must match one of the keys defined in your `config.yaml`.

---

## Admin Endpoints

### Register/Update Plugin

`POST /v1/plugins`

Registers a new plugin or updates existing metadata. Updating replaces every field, so send the secret and `is_paid` again; the plugin's versions are kept.

**Body:**

```json
{
	"slug": "my-plugin",
	"name": "My Great Plugin",
	"secret": "your-random-secret-here", // Optional: required for license generation
	"is_paid": true
}
```

### Publish New Version

`POST /v1/versions`

Pushes a new release to the database. Usually called from GitHub Actions. Posting a version the plugin already has updates it instead of adding a duplicate, so re-running a release is safe.

**Body:**

```json
{
	"slug": "my-plugin",
	"version": "1.2.0",
	"download_url": "https://storage.example.com/files/my-plugin-1.2.0.zip",
	"changelog": "<h4>Fixed</h4><ul><li>UI bug in settings</li></ul>",
	"requires_wp": "6.0",
	"tested_wp": "6.5",
	"requires_php": "7.4"
}
```

### Generate License Key

`POST /v1/licenses/generate`

Generates a signed license key for a customer.

**Body:**

```json
{
	"slug": "my-plugin",
	"client_id": "customer_123",
	"expires_at": 1767225600
}
```

---

## Public Endpoints

### Update Check

`GET /v1/update-check`

Called by WordPress plugins to check for newer versions.

**Query Parameters:**

- `slug` (required): The plugin slug.
- `version` (required): The current installed version.
- `license_key` (required if plugin is marked `is_paid`).
- `channel` (optional): `patch` | `minor` | `major` | `all`. Defaults to `major`.
  - `patch`: Returns the highest patch version matching current Major and Minor (e.g. `1.2.x` for installed `1.2.0`).
  - `minor`: Returns the highest version in the current Major series (e.g. `1.x.y` for installed `1.2.0`).
  - `major`: Returns the highest version overall.
  - `all`: Returns grouped updates and all newer versions (same as `all=true`).
- `all` (optional): `true` / `1`. Returns all newer versions and grouped patch/minor/major updates.

**Single Channel Success Response (200 OK):**
Returns a JSON object compatible with WordPress's `plugins_api` format if a matching update exists:

```json
{
	"slug": "my-plugin",
	"new_version": "1.2.4",
	"url": "",
	"package": "https://storage.example.com/files/my-plugin-1.2.4.zip",
	"tested": "6.5",
	"requires": "6.0",
	"requires_php": "7.4",
	"sections": {
		"changelog": "<h4>Fixed</h4><ul><li>Bug fixes</li></ul>"
	}
}
```

**Grouped / All Versions Response (200 OK when `all=true` or `channel=all`):**

```json
{
	"patch": {
		"slug": "my-plugin",
		"new_version": "1.2.4",
		"package": "https://storage.example.com/files/my-plugin-1.2.4.zip",
		...
	},
	"minor": {
		"slug": "my-plugin",
		"new_version": "1.4.3",
		"package": "https://storage.example.com/files/my-plugin-1.4.3.zip",
		...
	},
	"major": {
		"slug": "my-plugin",
		"new_version": "2.0.3",
		"package": "https://storage.example.com/files/my-plugin-2.0.3.zip",
		...
	},
	"versions": [
		{ "slug": "my-plugin", "new_version": "2.0.3", ... },
		{ "slug": "my-plugin", "new_version": "1.4.3", ... },
		{ "slug": "my-plugin", "new_version": "1.2.4", ... }
	]
}
```

_Note: `patch`, `minor`, or `major` will be `null` if no newer version exists in that specific category._

**No Update (204 No Content):**
Returned if no newer version matching the requested channel/scope exists.

### Validate License

`POST /v1/licenses/validate`

Public endpoint to check if a license key is valid for a given plugin. This endpoint is rate limited to 6 requests per minute per IP.

**Body:**

```json
{
	"slug": "my-plugin",
	"license_key": "eyJjaW..."
}
```

**Response (200 OK):**

```json
{
	"valid": true
}
```
