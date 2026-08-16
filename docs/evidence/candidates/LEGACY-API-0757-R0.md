# LEGACY-API-0757 `/health` R0 candidate receipt

Status: `CANDIDATE_READY` (not wired, not merged, not deployed)

## Frozen legacy product facts

Authority is immutable legacy commit `6cb989c071255437d75953dabb943318a74eb8f4`:

- `aicrm_next/platform/platform_foundation/api.py:17-19` declares public `GET /health`, calls only `GetSystemHealthQuery()`, and has no response-model, status-code, cache, or error override.
- `application.py:6-10` delegates that query to `observability.health_payload`; `observability.py:6-7` delegates to `runtime_health_state`.
- `platform/shared/runtime.py:121-150` uniquely fixes the complete JSON object: `ok`, `status`, `service`, `secret_key_present`, `wechat_shop_callback_token_present`, `wechat_shop_callback_token_required`, `database`, `database_mode`, `fixture_mode`, `production_data_ready`, `production_data_mode`, `repository_policy`, `runtime_owner`, `legacy_runtime_enabled`, and `warning`.
- The endpoint returns `200` for its normal, fixture, and degraded payloads. FastAPI's returned `dict` provides `Content-Type: application/json`; no endpoint or global `/health` cache directive exists. `fixture && production` produces `ok:false`, `status:"degraded"`, and the exact production-fixture warning; fixture outside production remains `ok:true` with warning `fixture data mode`.
- It does not test a DB connection, queue, provider, cache, or external service. Missing secrets are represented only by the two `*_present:false` flags. The legacy generic unhandled-error middleware owns an unrelated `500` response.

This is not `/healthz` (strict process liveness with only `{status:"ok"}`) and does not include the separate `LEGACY-API-0741 GET /api/system/health` readiness payload.

## R0 candidate contents

- `internal/platform/legacyhealth/health.go` contains a pure runtime snapshot query and a stand-alone `http.Handler` for the legacy JSON and method semantics. It accepts presence/mode booleans only, never raw configuration or secret strings.
- `internal/platform/legacyhealth/health_test.go` covers normal PostgreSQL, fixture, production-fixture degradation, missing configuration, no sensitive value serialization, exact JSON content type, absent cache policy, and non-GET `405`.

## Deliberately remaining central replay

The future M clean replay must, from a fresh exact-green main, decide and make the coordinated changes to: `cmd/aicrm/api.go` (public route construction and mount), `api/openapi.yaml`, generated bindings, `scripts/check_repo_contract.sh`, feature matrix, API/migration mapping, migration waterline, and acceptance manifest. It must also resolve route precedence with existing `/healthz`, preserve public unauthenticated access, and prove the complete HTTP surface under the real router. None of those files is touched by this candidate.

## Operational meaning today

Once centrally mounted, `/health` gives a load balancer or operator a configuration-derived runtime-mode snapshot: whether the process is on PostgreSQL or fixture data, whether it is dangerously running production against fixtures, and whether the named configuration values are present. It is not readiness: it does not prove a live DB connection, River/worker health, provider reachability, or deploy success. This R0 branch does not mount it, so current load balancers and operators cannot obtain that snapshot from v2 yet.
