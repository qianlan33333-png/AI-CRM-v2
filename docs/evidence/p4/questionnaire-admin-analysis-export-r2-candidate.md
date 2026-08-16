# Questionnaire Admin Analysis & Export R2 — B1 Non-central Candidate Receipt

## Immutable facts and legal slice boundary

- Evidence source: immutable legacy snapshot `6cb989c071255437d75953dabb943318a74eb8f4`, plus the canonical mapping and route triage on base `270d628e19a21da4114faa8443a6af18a68af686`.
- The complete administrator business goal is three routes: `LEGACY-API-0442` results summary, `LEGACY-API-0444` submission detail pagination, and `LEGACY-API-0433` browser CSV download. All are `questionnaire` owned and consume submission/answer snapshots; none participates in H5 submission, OAuth, WeCom, webhook, storage, or external-push execution.
- `LEGACY-API-0434` is not a prerequisite read: it is `POST .../export/preview`, `manage_questionnaire`, CSRF-protected command behavior that returns a masked three-row sample plus a guarded `questionnaire.export.preview` plan. It has no browser UI reference. It remains a separate future command slice.
- `LEGACY-API-0435` is not an administrator analysis view: it is `GET .../latest-submit-debug`, `admin_read`, `safe_debug=true`, `source_status=fixture`, and returns only the latest submission. It has no browser UI reference. It remains a separate diagnostics slice.
- Therefore 0434/0435 are legally split by user goal and authorization/method semantics, rather than misrepresented as one-route slices. `LEGACY-API-0636`, `0439`, H5 OAuth/submit, and the nine questionnaire-definition routes are not part of this candidate.

## Frozen contracts

| ID | Route | Request/auth | Success and errors |
| --- | --- | --- | --- |
| 0442 | `GET /api/admin/questionnaires/{questionnaire_id}/results` | path integer; human session, global `admin_read`, no CSRF, sensitive PII | `200` read envelope with `ok`, `route_owner`, `fallback_used=false`, `source_status`, `read_model_status`, `degraded=false`, `page_error`, `questionnaire_id`, and `results{submission_count,latest_submitted_at,average_score,rules}`. Missing owner is `404 questionnaire not found`. A production repository failure is `503`, `source_status=production_unavailable`, `read_model_status=unavailable`, `degraded=true`, with `results={}`. |
| 0444 | `GET /api/admin/questionnaires/{questionnaire_id}/submissions?limit=20&offset=0` | integer query defaults 20/0; human session, global `admin_read`, no CSRF, sensitive PII | `200` same read metadata plus identical `items` and `submissions`, `total`, `limit`, `offset`. Snapshot rows are `submitted_at DESC,id DESC` and include identity/source fields, `total_score` and `score`, tags, answers, and question/option snapshots. Missing owner is `404`; production unavailable is `503` with empty arrays and zero total. The old source has no explicit upper page cap; R2's bounded page request is an engineering decision, not a claim about an already live route. |
| 0433 | `GET /api/admin/questionnaires/{questionnaire_id}/export` | path integer; accepts `Idempotency-Key` header or query only for command correlation; human session, global `read_customer`, no CSRF, sensitive PII | default 10,000 rows; no server-side file/storage effect. `200 text/csv; charset=utf-8`, UTF-8 BOM, `Content-Disposition: attachment; filename="questionnaire-{slug}-submissions.csv"`, `X-AICRM-Route-Owner: ai_crm_next`, source-status header, and fallback header false. Base columns are `submission_id,submitted_at,external_userid,用户昵称,unionid,mobile,score,final_tags`, then definition-order question titles with snapshot-only additions and duplicate title suffixes. Time is Beijing `YYYY-MM-DD HH:MM:SS`; tags/options join with `、`; multiline values are CSV-quoted. Error envelope: input `400`, missing `404`, unavailable `503` with `degraded=true`; all include `ok=false`, route/source/write metadata, no fallback and no real external call. |
| 0434 | `POST .../export/preview` | JSON object; `fields` defaults to `submission_id,external_userid,answers,created_at`; correlation fields are stripped from the command payload; human session, global `manage_questionnaire`, CSRF required | `200` command envelope with `export_preview{fields,estimated_count,masked_sample,file_created:false}` and a guarded `side_effect_plan`; identity PII fields are replaced by `masked`. The plan is not a real provider/storage effect. A malformed JSON body is treated as `{}` by the old handler; a non-object body is `400`, missing owner is `404`, and unavailable is `503`. Separate command slice. |
| 0435 | `GET .../latest-submit-debug` | path integer; human session, global `admin_read`, no CSRF | `200 {ok:true,submission,source_status:"fixture",safe_debug:true}`; missing owner `404`, other old query errors `400`. Separate diagnostics slice. |

The historical role names support the listed capabilities (`questionnaire_admin` includes `admin_read`, `read_customer`, and `manage_questionnaire`); `manage_operations` is not the old permission for any of these five routes. The future M replay must map these exact contracts to the target Session → Actor authorization model without widening PII access.

## R2 leaf contents and safety boundary

- `internal/survey/app/submission_analysis.go` adds a Survey-owned read-store seam, summary/page validation, deterministic submission order checks, OneID conflict fail-closed classification, and a CSV encoder. It has no database adapter because no target submission/answer snapshot table exists.
- CSV formula cells are prefixed with an apostrophe when their first non-space character is `=`, `+`, `-`, or `@`. This is the explicitly requested R2 security hardening; it is documented because the legacy encoder emitted those values raw.
- Focused tests cover empty/ordered boundary validation, invalid paging, missing questionnaire, OneID conflict, unavailable store, PII-bearing output without logging, Beijing time conversion, UTF-8 BOM, CSV newline quoting, and formula injection. No production data or provider is called.

## Future M clean replay checklist (not performed here)

1. Choose and review the Survey-owned physical `submission` and `answer_snapshot` schema, indexes, and migration number/current/latest-waterline assertions.
2. Implement the Survey store adapter and legal Customer/Identity read-port resolution; fail closed for a OneID conflict and never query another domain table directly.
3. Bind exact browser session/Actor capability checks, PII audit count, and CSRF rule (`GET` none; 0434 `POST` required).
4. Register 0433/0442/0444 in `cmd/aicrm/api.go`, OpenAPI/generated artifacts, canonical mapping, feature matrix, acceptance manifest, and repo-contract only in the serialized central replay.
5. Preserve 0434 and 0435 as distinct subsequent slices; do not use this candidate as proof they are live.

## Status

`CANDIDATE_READY` only after the local checks and Draft PR recorded below. This receipt does not claim `MERGED`, staging, deployment, a live route, migration, or production data access. Exact main cannot currently let an administrator use these legacy interfaces for submission statistics, detail, or export; even this candidate waits for the future M clean replay.
