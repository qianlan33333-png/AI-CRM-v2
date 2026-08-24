# EE01 Internal Event Safe Export Local Core

`00073_internal_event_safe_exports.sql` is Events-owned local snapshot storage.
The three native operations are `createInternalEventSafeExport`,
`getInternalEventSafeExport`, and `downloadInternalEventSafeExport`.

The export is admin-only and actor-bound. Create requires a human session,
CSRF, and an idempotency key; reads re-authorize the actor. Its fixed CSV
projection contains only `event_id`, `event_type`, `occurred_at`,
`dispatched`, `consumer`, `status`, `attempt_count`, and `completed_at`.
It never exposes event payloads, customer IDs, job/lease/error fields, or
provider data.

The creation UoW reserves the receipt, materializes the bounded local
`event_log` plus `event_deliveries` rows, appends `events.safe_export_created`,
and completes the receipt last. Header, rows, and completed receipts are
immutable. The database transition checks the header count and audit fact;
row insertion and completion share the export parent lock.

Verification is local only: focused/race service and HTTP tests, OpenAPI
native-operation contract, and PostgreSQL 16.14 `TestInternalEventSafeExportPG16`.
This is neither River, Outbound, Provider, nor external-delivery evidence.
Deployment and external effects remain `NOT_EXECUTED`.
