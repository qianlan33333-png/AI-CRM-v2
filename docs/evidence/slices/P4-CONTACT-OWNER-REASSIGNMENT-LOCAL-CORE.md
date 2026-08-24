# P4 Contact Owner Reassignment Local Core

This package is Contact-local only. It accepts a bounded UTF-8 CSV with exactly `customer_id`, `expected_owner_staff_id`, `expected_updated_at`, and `target_owner_staff_id`; it creates a durable, actor-bound, short-lived preview and applies owner changes atomically only after the exact confirmation phrase, preview hash, CSRF, admin-only capability, and idempotency key are present.

The execution transaction locks active canonical `staff.id` targets and Contact `customers` in ascending customer-id order, validates expected owner/timestamp after locking, changes only `customers.owner_staff_id`, appends local `customer.updated` events, and completes its receipt/result before commit. No provider, WeCom transfer, external user ID, mobile, welcome message, or frontend surface is included.

Matrix status remains unchanged: LEGACY-S07-110 through LEGACY-S07-115 stay pending and LEGACY-S07-023 is intentionally excluded. The V2 difference is deliberate: safe CSV plus durable preview/confirmation/replay facts replaces legacy Excel/session/provider-transfer behavior; it proves local CRM ownership mutation only, not an external WeCom transfer.
