# API Reference

All requests should be made to the base URL of your deployment (e.g., `https://api.example.com/v1`).

## Authentication

Admin endpoints require the `X-API-Key` header. This key must match one of the keys defined in your `config.yaml`.

---

## Admin Endpoints

### Register/Update Plugin

`POST /v1/plugins`

Registers a new plugin or updates existing metadata.

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

Pushes a new release to the database. Usually called from GitHub Actions.

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

**Success Response (200 OK):**
Returns a JSON object compatible with WordPress's `plugins_api` format if a newer version exists.

**No Update (204 No Content):**
Returned if the installed version is equal to or newer than the latest version.

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
