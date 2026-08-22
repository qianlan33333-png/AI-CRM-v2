# Member-grid management route fragment

`NewManagementRouteFragment` owns only these local administration routes:

- `POST /api/admin/service-period-products/{service_product_id}/member-views`
- `PUT /api/admin/service-period-products/{service_product_id}/member-views/{view_id}`
- `DELETE /api/admin/service-period-products/{service_product_id}/member-views/{view_id}`
- `GET /api/admin/service-period-products/{service_product_id}/member-grid/share-settings`
- `POST /api/admin/service-period-products/{service_product_id}/member-grid/collaborators`
- `PUT /api/admin/service-period-products/{service_product_id}/member-grid/collaborators/{collaborator_id}`
- `DELETE /api/admin/service-period-products/{service_product_id}/member-grid/collaborators/{collaborator_id}`

The fragment accepts either the full path above or the relative path produced by
stripping `/api/admin/service-period-products`. It rejects query strings,
encoded paths, trailing slashes, empty segments, non-canonical IDs, unknown JSON
fields, duplicate JSON fields, invalid UTF-8, oversized bodies, missing or
repeated `Content-Type`, and missing or repeated `Idempotency-Key` headers.

## Injectable security boundary

The central adapter must implement `ManagementAuthorizer` and
`ManagementCSRFVerifier`:

- `share-settings` requests `products.read` and never invokes CSRF verification.
- Every mutation requests `products.write`, then requires successful CSRF
  verification, a positive authenticated actor ID, and one valid
  `Idempotency-Key`.
- This package does not alter central auth state or infer permission from a
  collaborator row.

## Closed write semantics

Every body contains `expected_version`. Create uses `expected_version: 0`;
update and delete require the current positive row version. Mutations reserve
and complete the existing local Product operation receipt in the same
transaction as the business write. The same key and payload replay the saved
result; the same key with another payload and stale CAS versions return a
conflict.

A direct saved-view create accepts only `name`, `state`, `sort`, and `columns`.
A clone create accepts `name` and `source_view_id` instead; the filter, sort, and
columns are copied inside the transaction from an existing saved view scoped to
the same product. `default` is the built-in non-database view and is immutable.

Columns are validated against the existing `safeColumns` member-grid schema.
No customer ID, raw mobile, unionid, external identity, opaque field, SQL, or
arbitrary expression is accepted.

Collaborators reference an existing active local `staff_id`. No provider sync or
invitation is sent. `permission: "edit"` is **local metadata only**: it does not
grant `products.write`, does not modify central RBAC, and is never consumed by
this package as authorization.

`share-settings` always returns:

- `external_share_supported: false`
- `external_share_enabled: false`
- `real_external_call_executed: false`
- `collaborator_edit_is_local_metadata_only: true`
- `collaborator_edit_grants_central_permission: false`

No public token, public route, QR code, member mutation, external invitation, or
real external call exists in this fragment.
