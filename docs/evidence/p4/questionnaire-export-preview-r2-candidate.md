# Questionnaire Export Preview R2 — B1 Non-central Candidate Receipt

## Scope and safety boundary

- This candidate implements only `LEGACY-API-0434`: `POST /api/admin/questionnaires/{questionnaire_id}/export/preview`.
- It expressly excludes `LEGACY-API-0435`. That endpoint exposes raw PII, token, and answers and is in `SAFETY HARD STOP`; no diagnostics code, DTO, test fixture, route, or dependency is carried here.
- It also excludes 0433, 0442, and 0444, which remain immutable evidence in Draft PR #231 and its separate worktree.
- No route, OpenAPI/generated artifact, migration number/waterline, manifest, feature matrix, mapping, checker, worker, file, storage adapter, or provider call is changed.

## Frozen 0434 contract

- Human session, global `manage_questionnaire`, and CSRF are required. The future route must reject non-object JSON as `400`; omitted or empty `fields` use `submission_id,external_userid,answers,created_at` in that order.
- Successful output is only an `export_preview_planned` projection: estimated total, latest three submissions in `submitted_at DESC,id DESC` order, a masked sample, `file_created=false`, and a plan with `requires_approval=true`, `adapter_mode=real_blocked`, and `real_external_call_executed=false`.
- Top-level identity fields are `masked` when present. Free answers remain sensitive manager-visible output and are neither logged nor persisted in an idempotency receipt. The preview does not create a file or mean queued, accepted, executed, or provider success.
- Missing questionnaire maps to `404`; malformed/non-object request input maps to `400`; unavailable auth/store maps to `503` in the future HTTP adapter.

## R2 leaf design

- `internal/survey/app/export_preview.go` defines the app-local authorization/store seams, digest-only receipt protocol, three-row read contract, identity masking, and blocked-plan DTO.
- The receipt contains actor scope, key digest, payload digest, and state only. A replay re-reads the current sensitive sample; it never stores free answers or identifiers as an idempotency result snapshot.
- No target submission/answer snapshot schema is present on exact main. A future M replay must supply the Survey-owned migration, legal Identity read port, concrete store adapter, session/Actor/CSRF handler binding, and central route/OpenAPI/generated/manifest wiring.

## Status

`CANDIDATE_READY` only after local checks and a separate Draft PR. No merge, deployment, staging, file generation, storage write, worker enqueue, provider effect, or production-data access is claimed.
