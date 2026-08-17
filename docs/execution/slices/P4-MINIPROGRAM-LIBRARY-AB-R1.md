# P4 Mini Program Library A+B R1

## Complete denominator

- Page carrier: `LEGACY-API-0054 GET /admin/miniprogram-library`.
- Route IDs `LEGACY-API-0390` through `LEGACY-API-0395` cover list, create, physical delete, get,
  update, and local `test-resolve`.
- `LEGACY-S07-018/054/055/057/058/059/060` are one complete page/API board. H01A1
  `LEGACY-S07-056` is an already merged dependency and is not recounted.
- The existing UI is unchanged; the page carrier redirects into the current admin shell.

## Authorization and effects

- Single-instance Media owner only. Human Session -> Actor -> existing `media.library.read/write`;
  every write and `test-resolve` retains session-bound CSRF.
- `test-resolve` reads only the Media-owned local thumbnail cache. Every success and error envelope
  states local-only/no provider effect; no upload, URL fetch, jump validation, send, or external call.
- Client `thumb_media_id`, including explicit null, is rejected before state. Only the local cache
  resolver may update the cached media ID/expiry snapshot.
- Business fact, Event, and completed receipt share one UoW. `outcome_unknown` is durable and replay
  returns it without retry. Delete is the frozen legacy physical delete and its receipt remains.

## Data and migration

- Migration `00045` owns `media_miniprograms`, local thumbnail cache evidence, operation receipts,
  historical preflight evidence, and immutable source import ledger.
- Stable legacy metadata and source ID are MIGRATE (preflight-gated); provider media ID/expiry and
  embedded base64 are DROP. Image references are REMAP/REBUILD through Media ownership only.
- Real historical counts and coverage remain `EXTERNAL_GATE_REQUIRED`; no live/history database is
  contacted by this slice. See `docs/evidence/slices/P4-MINIPROGRAM-LIBRARY-AB-R1.md`.

## Local verification state

- Clean integration replay base: exact-green main `a5914196e6465c931baa4278cd5164a420222068`.
- Superseded B2-reviewed snapshot: tree `f58111ed503611e73735d0cd15e07bc16fdbbbba`, full-index
  patch SHA-256 `e04e83fdb0b6f94b3c86becbf16875c736b736026535abae6cb9b2eb0f570825`, result
  `BUILD_DEFECT P0=0/P1=3/P2=1`; it is not passing evidence.
- Superseded first repair snapshot: tree `11a2086cabb7eae59b42331c721987d0dc472d74`, full-index
  patch SHA-256 `04f7a6f6f976aef86eccf0625dae85fbd4f370536cd09d3e708f365092afd5bf`, result
  `BUILD_DEFECT P0=0/P1=2/P2=0`; arbitrary REBUILD digest and unbound provider-cache DROP are now
  repaired with DB-enforced insertion/completion checks and permanent negatives.
- PostgreSQL 16.14 clean replay `44/45/44/45` and the expanded DB negative suite pass locally.
- OpenAPI, generated client, API mapping, matrix, six-field acceptance declaration, and canonical
  fingerprints are synchronized. The generic acceptance manifest, lightweight promotion framework,
  P1 reconciliation, migration mapping, repository contract and its complete negative suite pass on
  the staged tree without changing workflow or checker executable logic.
- Go unit/race/vet, the real PG repository suite, HTTP contracts, and Web lint/typecheck/226 tests/
  build/audit pass locally. The first local `make ci-go` invocation omitted workflow-only PostgreSQL
  environment flags and stopped at the expected fail-closed guard; that invocation is not counted as
  acceptance evidence and is rerun with the workflow-equivalent environment before PR.
- Status is `INTEGRATION_READY_LOCAL`; GitHub checks and exact-main merge remain pending.

## Excluded

No new UI, tenant, DTO expansion, workflow/checker/root-dependency/security-policy change, provider
call, production/staging deployment, live migration, payment/refund, or outbound effect.
