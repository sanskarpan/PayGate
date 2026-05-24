# Retention Policies

PayGate applies policy-driven retention to operational artifacts that contain
payloads, generated export content, or external storage locators.

Current artifact classes:

- `report_export`
  - default action: `redact_content`
  - default retention: `30` days
- `webhook_delivery_attempt`
  - default action: `redact_payload`
  - default retention: `14` days
- `onboarding_document`
  - default action: `redact_locator`
  - default retention: `90` days

What enforcement does:

- `redact_content`
  - clears generated export file content and download token material
- `redact_payload`
  - clears stored webhook request and response payloads and redacts stored error detail
- `redact_locator`
  - clears onboarding document storage keys while preserving review metadata

Legal holds:

- legal holds can be scoped by artifact class
- optionally narrowed to one merchant and one artifact id
- active holds block retention enforcement until released

Runtime model:

- retention policies are stored in `paygate_ops.retention_policies`
- legal holds are stored in `paygate_ops.legal_holds`
- each execution is recorded in `paygate_ops.retention_runs`
- the API gateway starts a periodic retention worker and also supports manual execution

Operator APIs:

- `GET /v1/retention/policies`
- `PUT /v1/retention/policies/{artifactClass}`
- `GET /v1/retention/holds`
- `POST /v1/retention/holds`
- `POST /v1/retention/holds/{holdID}/release`
- `GET /v1/retention/runs`
- `POST /v1/retention/run`
