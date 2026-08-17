# P4 System Health 0741

## Frozen denominator

- One complete legacy route: `LEGACY-API-0741 GET /api/system/health`.
- Public read-only readiness. It has no Session, Actor, Capability, CSRF, redirect, idempotency key,
  receipt, mutation, or external effect.
- The response always contains exactly six ordered components: WeCom configuration, release,
  runtime units, database, migration, and queues.

## Contract and safety

- Database, migration, queue probe, and unknown runtime-unit state fail closed. A bounded observation
  timeout is warning-only, and warning-only responses remain HTTP 200.
- Production requires a complete release SHA and PostgreSQL. Non-production may report an explicit
  fixture or missing release SHA as a warning.
- The public response contains no raw SHA, URL, error, event, job, receipt, payload, identity, PII,
  secret, or provider result. `outcome_unknown` is never retried and appears only as an aggregate
  integer capped at 99.
- `Cache-Control: no-store` is mandatory. The final router mounts the route before authentication;
  unsupported methods fail without invoking authentication.

## Delivery

- Clean integration replay base: exact-green main `f10fda7f7069048b9b456a71a2f9545be28c6dc1`.
- Uses the generic six-field acceptance manifest. No Makefile target, workflow, root dependency,
  security policy, migration, UI, tenant boundary, or external integration changes.
- Migration closure: `no_schema_or_external_effect`; this read-only route adds no table, column,
  index, migration, migration-mapping decision, queue write, provider call, or external effect.
- Candidate Merge Guard now accepts the same explicit no-schema declaration from an added API mapping
  when a route has no Feature Matrix row. It still requires matching slice evidence and retains all
  formal mapping, acceptance, HTTP composition, internal closure, and candidate rejection gates.
- Local verification and GitHub/exact-main receipts are recorded in
  `docs/evidence/slices/P4-SYSTEM-HEALTH-0741.md`.

## Excluded

No staging or production deployment, live migration, production DB access, real WeCom call, payment,
refund, outbound send, or automatic retry.
