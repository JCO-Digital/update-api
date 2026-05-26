# License System

The API uses a stateless license verification system. This means the API does not need to store every issued license key in the database to verify it. Instead, it uses cryptographic signatures.

## How it works

1.  **Secret Key**: Each plugin has a unique `secret` stored in the database.
2.  **Payload**: When a license is generated, a JSON payload is created:
    ```json
    {
      "cid": "customer_123",
      "exp": 1767225600,
      "slug": "my-plugin"
    }
    ```
3.  **Signature**: The payload is Base64 encoded, and an HMAC-SHA256 signature is created using the plugin's `secret`.
4.  **Token**: The final license key is `base64_payload.signature`.

## Validation Logic

When the API receives an update check for a paid plugin:
1.  It splits the license key into payload and signature.
2.  It re-calculates the signature of the payload using the plugin's secret from the DB.
3.  If the signatures match, it decodes the payload and checks:
    - Does the `slug` match the requested plugin?
    - Is the `exp` (expiration date) in the future?
4.  If all checks pass, the update is allowed.

## Security Considerations
- **Keep Secrets Safe**: If a plugin's secret is leaked, anyone can generate valid license keys for that plugin.
- **Rotation**: If you change a plugin's secret, all previously issued license keys for that plugin will immediately become invalid.
