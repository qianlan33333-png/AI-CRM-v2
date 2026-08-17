# P4 Mini Program Library A+B R1 migration and reconciliation evidence

## Superseded implementation snapshot

- Exact-green replay base: `6e21f6914d41e9f436b5d1cbd3c28ce737b86ddb`.
- Locked temporary-index tree: `f58111ed503611e73735d0cd15e07bc16fdbbbba`.
- Full-index patch SHA-256: `e04e83fdb0b6f94b3c86becbf16875c736b736026535abae6cb9b2eb0f570825`.
- Migration blob: `migrations/00045_media_miniprogram_library.sql` =
  `1d0a0a222dcced286e27cbb3f8b23b2dd586d69e`.
- Independent B2 review returned `BUILD_DEFECT` (`P0=0/P1=3/P2=1`). This snapshot is retained only
  as the exact reviewed input and is not passing evidence. The repair snapshot will receive a new
  tree, full-index patch hash, file list, and independent review.
- This receipt is not a live-data import, deployment, provider receipt, or historical reconciliation
  result.

## Superseded first repair snapshot

- Tree `11a2086cabb7eae59b42331c721987d0dc472d74`, full-index patch SHA-256
  `04f7a6f6f976aef86eccf0625dae85fbd4f370536cd09d3e708f365092afd5bf`, exact 28 paths.
- Independent B2 reproduced every object and returned `BUILD_DEFECT` (`P0=0/P1=2/P2=0`). The
  preflight state machine, authoritative T14-208 mapping, supporting index, and prior runtime/UoW
  findings were closed, but the ledger allowed an arbitrary 32-byte REBUILD digest and allowed a
  migrated target to retain provider cache while claiming `dropped`.
- The next repair binds REBUILD to both `media_images.checksum` and `media_image_blobs.checksum` at
  ledger insertion and completion, and requires every migrated target to keep provider cache fields
  empty through reconciliation. Wrong-digest, post-ledger Media checksum drift, cache-at-insert, and
  cache-before-completion are permanent negative cases.

## Frozen source-to-target contract

| Legacy field | R1 target/disposition | Rule |
| --- | --- | --- |
| `id` | `media_miniprograms.legacy_source_id` plus immutable import ledger | Preserve the positive source ID; never mint an unaudited replacement. |
| `name` | `media_miniprograms.name` | Preserve, including an empty update value. Create retains legacy name/title fallback. |
| `appid` | `media_miniprograms.app_id` | Preserve stable metadata. |
| `pagepath` | `media_miniprograms.page_path` | Preserve stable metadata and expose the two frozen wire aliases. |
| `title` | `media_miniprograms.title` | Preserve; an explicitly empty update is rejected. |
| `thumb_image_url` | `media_miniprograms.thumbnail_image_url` conditionally | Metadata only, maximum 2048 characters, and never fetch/follow the URL. |
| `thumb_image_base64` | DROP | Volatile embedded content is not copied into the canonical table. |
| `thumb_media_id` | DROP | Provider cache state is not a stable migration fact. |
| `thumb_media_id_expires_at` | DROP | Provider cache expiry is not a stable migration fact. |
| `enabled` | `media_miniprograms.enabled` | Preserve. |
| `created_at` | `media_miniprograms.created_at` | Preserve business time after authorized preflight validation. |
| `updated_at` | `media_miniprograms.updated_at` | Preserve business time and use with source ID for reconciliation ordering. |
| `thumb_image_id` | `media_miniprograms.thumbnail_image_id` through Media owner | REMAP/REBUILD only through an existing Media image fact; no cross-domain direct write. |

The earlier overlay-only treatment of `LEGACY-T14-208` was rejected by independent B2 review.
The authoritative migration mapping must itself record **MIGRATE (preflight-gated)**, provider-cache
DROP, and Media-owned REMAP/REBUILD without claiming that historical data has been inspected.

## Historical preflight and reconciliation gate

No importer or live-source connector is shipped. A future separately authorized read-only preflight
must provide a SHA-256 source snapshot digest, total row count, URL-only row count, and unresolved
image-reference row count before an import can become `ready`. The database starts with both
preflight and ledger tables empty.

The reconciler must independently prove all of the following against one frozen source snapshot:

1. each source row has exactly one immutable terminal ledger disposition (`migrated` or
   `quarantined`) and ledger cardinality equals the frozen preflight source count;
2. every migrated target exists at ledger-write time and carries the same `legacy_source_id`;
3. row digests match canonical source inputs and duplicate source IDs fail closed;
4. every row durably records legacy thumbnail ID, target Media image ID, rebuild content digest,
   source URL-only/unresolved facts, and provider-cache `dropped` as applicable; no provider cache
   field is treated as canonical;
5. URL-only rows keep the gate closed until a human records exactly one of
   `retain_metadata_without_fetch` or `quarantine_row`, with an auditable decision reference;
6. no URL fetch, WeCom/WeChat upload, jump validation, provider call, send, retry, or cross-domain
   write occurs during preflight/reconciliation.

True historical counts, coverage, and sample-field reconciliation remain
`EXTERNAL_GATE_REQUIRED`. `outcome_unknown` is a completed local cache fact and is never
automatically retried.

## Local receipts

- Repair replay on fresh PostgreSQL 16.14 passed `44 -> 45 -> 44 -> 45`; Event/Auth/Media history
  prefixes remained byte-stable, and the new historical preflight/ledger started empty.
- Real SQL repository/UoW tests cover create/update/physical-delete, receipt/event atomicity,
  same-key replay and conflict, 16-way concurrency, 200k receipt lookup, local cache resolved/miss/
  `outcome_unknown`, immutable ledger, target/source mismatch, and URL-only human-decision negatives.
- HTTP tests cover the page carrier and APIs `LEGACY-API-0390` through `0395`, existing Media
  read/write capability, CSRF on every write and test-resolve, aliases, malformed inputs, and
  local-only responses with `real_external_call_executed=false`.
- OpenAPI contract, feature matrix (`rows=294`), module/version checks, repository fingerprints,
  architecture/ownership/source policy, Go unit/race/vet/build, Web lint/typecheck/unit/build/audit,
  migration validate/negative/Up-Down-Up, secret/sensitive-path scans, and the Mini Program PG/HTTP
  acceptance pass locally. Direct Product, Channel, Coupon, Order, Message Archive, H01A1, H03,
  Survey, Auth, Admin Shell, Execution Runtime, and O7 consumers also pass.
- Historical reconciliation remains a separate external release gate. This package proves the local
  schema, immutable preflight/ledger rules and zero-live-import behavior; it does not self-sign or
  claim real historical reconciliation.

## Exact-main integration evidence

CI Promotion optimization PR #239 is already part of the clean replay base. This package uses its
generic acceptance manifest and declaration-aware promotion boundary without modifying checker
executable logic, workflows, root dependencies, Makefile targets, or security policy.

- `scripts/check_repo_contract.sh`: PASS on the staged tree.
- `scripts/test_repo_contract.sh`: PASS for the complete negative fixture suite.
- `scripts/test_ci_acceptance_manifest.sh`: PASS with the new generic row at sequence `0044`.
- `make p1-reconciliation-contract`: PASS (`routes=781`, `migrate_routes=502`).
- `make migration-mapping-contract`: PASS (`rows=316`, `physical=217`, `pending=0`).
- `make openapi-p1-contract`, repository fingerprints and generated checks: PASS.
- PostgreSQL 16.14 migration compatibility and Mini Program PG/HTTP acceptance: PASS.
- Web fixed-toolchain CI: PASS (`13` files, `226` tests, build and zero high-risk audit findings).

The workflow-equivalent full Go and migration suites, GitHub Required Checks, match-head squash and
exact-main closure are still required before this board can be called `MERGED`.

## External boundaries

`HISTORICAL_DATABASE_NOT_ACCESSED`, `PRODUCTION_DATABASE_NOT_EXECUTED`,
`LIVE_MIGRATION_NOT_EXECUTED`, `REAL_WECOM_OR_WECHAT_NOT_EXECUTED`,
`URL_FETCH_NOT_EXECUTED`, `OUTBOUND_NOT_EXECUTED`, `DEPLOYMENT_NOT_EXECUTED`.
