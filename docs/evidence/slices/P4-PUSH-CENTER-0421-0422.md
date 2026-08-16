# Push Center 0421/0422 integration evidence

- Fresh base: `e5d6ffbde0d4fcdf1423aad97c218c2b54c2cd13`.
- Denominator: 0416–0422 is seven routes. PR #200 is the merged evidence for 0416–0420; this change adds only 0421 and 0422.
- Frozen route contract: human session, global `admin_read` semantics mapped to canonical global `operations.read`, GET/no CSRF, internal PII, no external effects.
- Normal responses preserve 13 sections and nine status definitions. Stats includes sections, effective-status and status aggregation, `real_external_call_executed=false`, capability owner, and a read-only `runtime_queue` object.
- Read failures and invalid timestamps return HTTP 200 with `degraded=true`, `production_read_unavailable`, empty items/sections/counts, the full status definitions, and `real_external_call_executed=false`; they do not claim successful empty production data.
- Migration receipt: isolated PG16.14 up/down/up passed `43/44/43/44`; initial readiness is false, no tenant columns or cross-domain foreign keys were created.
- Performance receipt: a 200,000-row `ILIKE` probe used the Push Center trigram index and did not contain `Seq Scan`.
- Safety receipt: no provider, worker, queue acceptance, delivery receipt, automatic retry, or production database was used. `unknown_after_dispatch` remains a manual reconciliation state.
