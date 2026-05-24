# Security Key Hierarchy

## Current Repo-Backed Model

PayGate now supports application-layer envelope encryption for high-risk stored fields.

Environment variables:

- `APP_ENCRYPTION_KEYS`
- `APP_ENCRYPTION_ACTIVE_KEY_VERSION`

Format:

```text
APP_ENCRYPTION_KEYS=v1:<base64-32-byte-key>,v0:<base64-32-byte-key>
APP_ENCRYPTION_ACTIVE_KEY_VERSION=v1
```

## Protected Fields

Current protected-at-rest fields:

- merchant onboarding registration numbers
- merchant onboarding tax identifiers
- onboarding party email and phone
- onboarding document storage keys
- reporting tax-profile GSTIN
- payout beneficiary holder name, IFSC, and VPA
- webhook signing secrets

## Rotation Model

1. Add a new key version to `APP_ENCRYPTION_KEYS`
2. Move `APP_ENCRYPTION_ACTIVE_KEY_VERSION` to the new version
3. New writes use the new version immediately
4. Existing rows remain readable through legacy versions until rewritten

## Production Boundary

This repo uses environment-backed key material so the feature is testable locally.

Production recommendation:

- store root key material in a real KMS or secrets manager
- inject only short-lived decrypted data keys into the app process
- rotate key versions on a defined schedule
- keep decrypt permission scoped to the services that actually need field access
