# LEGACY-API-0757 `/health` central integration receipt

Status: `INTEGRATION_CANDIDATE` (central route wired locally; not yet PR-merged or deployed)

## Frozen legacy product facts

Authority is immutable legacy commit `6cb989c071255437d75953dabb943318a74eb8f4`:

- `aicrm_next/platform/platform_foundation/api.py:17-19` declares public `GET /health`, calls only `GetSystemHealthQuery()`, and has no response-model, status-code, cache, or error override.
- `application.py:6-10` delegates that query to `observability.health_payload`; `observability.py:6-7` delegates to `runtime_health_state`.
- `platform/shared/runtime.py:121-150` uniquely fixes the complete JSON object: `ok`, `status`, `service`, `secret_key_present`, `wechat_shop_callback_token_present`, `wechat_shop_callback_token_required`, `database`, `database_mode`, `fixture_mode`, `production_data_ready`, `production_data_mode`, `repository_policy`, `runtime_owner`, `legacy_runtime_enabled`, and `warning`.
- The endpoint returns `200` for its normal, fixture, and degraded payloads. FastAPI's returned `dict` provides `Content-Type: application/json`; no endpoint or global `/health` cache directive exists. `fixture && production` produces `ok:false`, `status:"degraded"`, and the exact production-fixture warning; fixture outside production remains `ok:true` with warning `fixture data mode`.
- It does not test a DB connection, queue, provider, cache, or external service. Missing secrets are represented only by the two `*_present:false` flags. The legacy generic unhandled-error middleware owns an unrelated `500` response.

This is not `/healthz` (strict process liveness with only `{status:"ok"}`) and does not include the separate `LEGACY-API-0741 GET /api/system/health` readiness payload.

## Reviewed leaf dependency

- `internal/platform/legacyhealth/health.go` contains a pure runtime snapshot query and a stand-alone `http.Handler` for the legacy JSON and method semantics. It accepts presence/mode booleans only, never raw configuration or secret strings.
- `internal/platform/legacyhealth/health_test.go` covers normal PostgreSQL, fixture, production-fixture degradation, missing configuration, no sensitive value serialization, exact JSON content type, absent cache policy, and non-GET `405`.

The reviewed `internal/platform/legacyhealth` leaf remains the only owner of
the 15-field payload and its pure query. This integration adds neither a
database probe, queue/worker probe, provider call, cache nor new storage.

## Central integration candidate

- Fresh base: `e5d6ffbde0d4fcdf1423aad97c218c2b54c2cd13`, the exact-main CLOSED squash for LEGACY-API-0781. Historical WIP was not copied or cherry-picked.
- `cmd/aicrm/api.go` constructs the leaf query from a typed, presence-only startup adapter and mounts only public `GET /health` through the normal recovery, timeout and route-pattern middleware. The concrete route is registered before the final `/{filename}` compatibility catch-all; `/healthz`, login/logout, admin/API and OAuth/callback routes retain their own handlers.
- `internal/config` records only booleans from the frozen legacy environment aliases and setting-presence rules. It does not retain, log, return or serialize a secret value. The existing validated PostgreSQL URL supplies the database-mode fact; fixture and production-fixture boundaries remain exercised through the pure leaf query and black-box router.
- `api/openapi.yaml`, Go candidate bindings, Orval client and `tools/openapi-contract` bind operation `getLegacyHealth` to `LEGACY-API-0757`. The OpenAPI checker rejects authentication, additional response codes and any loss of the closed 15-field JSON schema.
- `docs/api-mapping.jsonl` now maps this route to `getLegacyHealth` / `GET` / `/health` under target `P4-S04-LEGACY-HEALTH`. The 293-row UI feature matrix is intentionally unchanged: this public machine endpoint has no legacy page action, matching the repository's API-only matrix policy. There is no migration/table ownership change or waterline update.
- The CI acceptance manifest gains a no-database target for the real-router normal/boundary/error suite. It is deliberately not an up/down/up migration target.

## Operational meaning today

Once merged, `/health` gives a load balancer or operator a configuration-derived runtime-mode snapshot: whether the process is on PostgreSQL or fixture data, whether it is dangerously running production against fixtures, and whether the named configuration values are present. It is not readiness: it does not prove a live DB connection, River/worker health, provider reachability, or deploy success. This receipt does not claim a deployment, provider call, production database action or real external effect.
