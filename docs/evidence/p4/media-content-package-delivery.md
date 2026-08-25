# Media Content Package & Delivery Binding

`00083` is a single-enterprise, local-only schema package. Canonical image,
attachment (PDF), mini-program and group-invite IDs are retained as typed
foreign keys; no tenant model, public object URL, object-storage adapter or
provider credential is introduced.

The package stores preview/validation inputs, private multipart-PDF parts
(10 MiB total), CAS-ready campaign/plan/package/group-invite bindings, and
an immutable outbound-media acceptance snapshot. The outbound record permits
only accepted, queued, attempted, outcome_unknown and reconciled local/EER
states. Its provider eligibility, real-call and delivery-proof flags are
constrained false; it therefore cannot claim sent or delivered and contains
no retry mechanism.

`make p4-media-content-delivery-acceptance` creates an isolated PostgreSQL
16.14 database, migrates through `00083`, directly asserts that `00083` itself
is applied (without asserting that it is the maximum migration), proves empty
`83 -> 82 -> 83`, then
seeds a private canonical PDF/upload, content package, accepted `outbound_media`
effect, `outcome_unknown` transition and manual reconciliation. Its PII-minimal
detail assertion is exactly package ID, opaque `eer_N`, state,
`provider_accepted` and `delivery_proven`; target, payload, evidence and receipt
digests never leave the database query. The populated `00083 -> 82` migration
must fail.

The 12 V2-native operations are registered under
`P4-MEDIA-CONTENT-DELIVERY-00083-2026-08-25` in the OpenAPI candidate registry:
package preview/create/update; PDF initiate/part/complete; delivery-binding
get/create/update; and outbound-media accept/detail/manual-reconcile. These
have no guessed 1:1 legacy route mapping, so the frozen Feature Matrix and its
denominator remain unchanged. `scripts/ci/run_selected_database.sh selected media`
now executes the dedicated PG acceptance; `scripts/ci/run_selected_go.sh selected
media,outbound false` covers the two selected Go domains.
