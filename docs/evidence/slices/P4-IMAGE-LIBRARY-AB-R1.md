# P4 Image Library A+B R1 declaration evidence

## Scope and exact source

- Reconciliation base: `59c7fa14742d13d62cdf7f7c54834b5b35dfec38`.
- The frozen fact audit is `handoffs/codex-p4/image-library-ab-declaration-audit.md`, which inspected
  exact main `44dc377bf20e172a6858a2a5d954ed6d91833ee7`; this base changes CI selection only and retains
  the audited Image Library implementation.
- This is a declaration-only correction. It changes only mapping `LEGACY-API-0052` and
  `LEGACY-API-0361`, plus matrix rows `LEGACY-S07-017` and `LEGACY-S07-049`.
- Canonical inventory correction: the implemented `/admin/image-library` workspace is an edge-served
  SPA deep link, not a Go/OpenAPI page carrier. Therefore `LEGACY-API-0052` remains visibly implemented
  in `LEGACY-S07-017`, while its `candidate_v2_operation_id/method/path` stay
  `PENDING_HUMAN_DESIGN` until a real canonical operation exists. The API upload mapping `0361` is
  unaffected.

## Declared local capabilities

| ID | Local capability and exact-tree evidence |
| --- | --- |
| `LEGACY-API-0052` / `LEGACY-S07-017` | `deploy/Caddyfile` serves non-API deep links through `index.html`; `web/src/main.tsx` mounts `ImageLibraryPage` at `/admin/image-library` for admin/ops. Commit `435fa46c5daf49da694085dd3f2132c66ed2435b` (PR #254) and `web/src/main.test.tsx` cover the route, navigation and sales fail-closed behavior. |
| `LEGACY-API-0356`, `0358` / `LEGACY-S07-049` | The page consumes the local list and facet routes through `web/src/image-library.ts` and `web/src/image-library-ui.tsx`, with search, tag/category and unlabeled filters, pagination, response-envelope validation and retry UI. Backend commits `d81ba86bde4752fe6d3e402f7bf5bebf007db600` (#247) and `b73aa59276cdb4dc6075ca87fdf3c97f40cd9cf3` (#242) provide the projections; Web tests and `acceptance/media/image_facets_integration_test.go` are committed evidence. |
| `LEGACY-API-0361` / `LEGACY-S07-050` | Multipart upload is `uploadLegacyImage`, committed in `72e9c60024f62e746d7792102a2a76a9741d32ae` (PR #209). #254 supplies the real form, CSRF and single-use idempotency key, followed only by a list/facet reload after a confirmed response. `acceptance/media/h01a1_integration_test.go`, `web/src/image-library.test.ts` and `web/src/image-library-ui.test.tsx` are committed evidence. S07-050 was already `IMPLEMENTED/SYNTHETIC_PASS`; its status is intentionally unchanged. |

## Deliberate non-claims

- `LEGACY-S07-051`, `LEGACY-S07-052`, and `LEGACY-S07-053` stay `NOT_STARTED/NOT_RUN`: current UI
  has no PUT transport or metadata form, no enabled toggle, and no DELETE confirmation/force/reference
  conflict flow. Existing backend 0364/0362 routes do not close those user flows.
- The current card deliberately has no image preview. Detail (`0363`) and binary variants (`0366`) are
  not consumed by this page; deferred `0359`, `0360`, and `0365` remain outside this declaration.
- `0361` writes only local Media PostgreSQL metadata/blob, receipt and same-UoW event facts. The
  response contract reports `real_external_call_executed=false` and PostgreSQL storage; no provider,
  WeCom, queue, object store, historical reconciliation, or external effect is claimed.
- `DEPLOYMENT_NOT_EXECUTED`, `HISTORICAL_DATABASE_NOT_ACCESSED`,
  `PRODUCTION_DATABASE_NOT_EXECUTED`, `LIVE_MIGRATION_NOT_EXECUTED`,
  `REAL_WECOM_OR_WECHAT_NOT_EXECUTED`, and `OUTBOUND_NOT_EXECUTED` remain in force.

## Required local document gates

- `jq -e .` for every JSONL mapping row.
- `make feature-matrix-contract` and `make migration-mapping-contract`.
- `make generate-check`, `scripts/test_repo_contract.sh`, `scripts/check_repo_contract.sh`, and the
  changed-range Gitleaks scan. These gates validate this declaration patch only; they do not turn the
  non-claims above into deployment or external-effect evidence.
