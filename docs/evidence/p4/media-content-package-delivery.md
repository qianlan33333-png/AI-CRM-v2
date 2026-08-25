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

The migration compatibility target applies only `00083`, rolls it back to
`00082`, then reapplies it. It asserts `version_id=83` directly rather than
using the maximum migration version.
