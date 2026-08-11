#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "repo-contract-tests: $*" >&2
  exit 1
}

for forbidden_git_env in \
  GIT_INDEX_FILE GIT_DIR GIT_WORK_TREE GIT_OBJECT_DIRECTORY \
  GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_QUARANTINE_PATH; do
  [[ -z "${!forbidden_git_env:-}" ]] ||
    fail "repository redirection environment is forbidden: $forbidden_git_env"
done

repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"
test_root="$(mktemp -d -t aicrm-v2-repo-contract-test.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

make_fixture() {
  local name="$1"
  local fixture="$test_root/$name"
  mkdir -p "$fixture"
  git -C "$repo_root" archive --format=tar "$(git -C "$repo_root" write-tree)" |
    tar -xf - -C "$fixture"
  git -C "$fixture" init -q
  git -C "$fixture" add -A
  printf '%s\n' "$fixture"
}
restage_make_receipt() { local fixture="$1" digest
  git -C "$fixture" add Makefile; digest="$(git -C "$fixture" show :Makefile | sha256sum | awk '{print $1}')"
  sed -i.bak -E "/^verify_index_sha256 Makefile/{n;s/[0-9a-f]{64}/$digest/;}" "$fixture/scripts/check_repo_contract.sh"; rm -f "$fixture/scripts/check_repo_contract.sh.bak"; git -C "$fixture" add scripts/check_repo_contract.sh; }

baseline_fixture="$(make_fixture baseline)"
if ! (cd "$baseline_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "valid staged baseline was rejected"
fi

shell_shadow_fixture="$(make_fixture shell-environment-shadow)"
printf '%s\n' '#!/usr/bin/env bash' 'unsafe() {' '  local PATH=/tmp' '}' >"$shell_shadow_fixture/scripts/unsafe_env_shadow.sh"
git -C "$shell_shadow_fixture" add scripts/unsafe_env_shadow.sh
if (cd "$shell_shadow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "shell local variable shadowing PATH was accepted"
fi

for file_path in acceptance/fixtures/postgres.go acceptance/fixtures/postgres_test.go docs/execution/slices/P2-00.md; do
  acceptance_fixture_receipt="$(make_fixture "p2-00-receipt-${file_path##*/}")"
  case "$file_path" in *.go) printf '%s\n' '// P2-00 receipt drift' >>"$acceptance_fixture_receipt/$file_path" ;; *) printf '%s\n' '# P2-00 receipt drift' >>"$acceptance_fixture_receipt/$file_path" ;; esac
  git -C "$acceptance_fixture_receipt" add "$file_path"
  if (cd "$acceptance_fixture_receipt" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-00 acceptance fixture receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/platform/store/uow.go internal/platform/store/uow_test.go acceptance/p2s01r/doc.go acceptance/p2s01r/uow_integration_test.go docs/execution/slices/P2-01R.md docs/execution/implementation-plan.md; do
  uow_receipt_fixture="$(make_fixture "p2-01r-receipt-${file_path//\//-}")"
  case "$file_path" in *.go) printf '%s\n' '// P2-01R receipt drift' >>"$uow_receipt_fixture/$file_path" ;; *) printf '%s\n' '# P2-01R receipt drift' >>"$uow_receipt_fixture/$file_path" ;; esac
  git -C "$uow_receipt_fixture" add "$file_path"
  if (cd "$uow_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-01R UoW or housekeeping receipt drift was accepted: $file_path"
  fi
done

missing_uow_implementation="$(make_fixture missing-p2-01r-uow)"
rm -f "$missing_uow_implementation/internal/platform/store/uow.go"
git -C "$missing_uow_implementation" add -u internal/platform/store/uow.go
if (cd "$missing_uow_implementation" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-01R UnitOfWork implementation was accepted"
fi

for file_path in cmd/aicrm/main.go cmd/aicrm/components.go internal/config/load.go internal/config/schema.go internal/config/schema_test.go acceptance/p0s01/process_blackbox.sh docs/execution/slices/P2-02.md; do
  config_receipt_fixture="$(make_fixture "p2-02-receipt-${file_path//\//-}")"
  case "$file_path" in *.go) printf '%s\n' '// P2-02 receipt drift' >>"$config_receipt_fixture/$file_path" ;; *) printf '%s\n' '# P2-02 receipt drift' >>"$config_receipt_fixture/$file_path" ;; esac
  git -C "$config_receipt_fixture" add "$file_path"
  if (cd "$config_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-02 config receipt drift was accepted: $file_path"
  fi
done

missing_config_loader="$(make_fixture missing-p2-02-config-loader)"
rm -f "$missing_config_loader/internal/config/load.go"
git -C "$missing_config_loader" add -u internal/config/load.go
if (cd "$missing_config_loader" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-02 config loader was accepted"
fi

for file_path in \
  migrations/00002_event_log.sql \
  sqlc.yaml \
  internal/events/port/port.go \
  internal/events/store/queries/event_log.sql \
  internal/events/store/appender.go \
  internal/events/store/appender_test.go \
  internal/events/store/generated/db.go \
  internal/events/store/generated/event_log.sql.go \
  internal/events/store/generated/models.go \
  internal/events/store/generated/querier.go \
  acceptance/p2s06/doc.go \
  acceptance/p2s06/event_log_integration_test.go \
  acceptance/p1s11/contracts_test.go \
  docs/execution/slices/P2-06.md; do
  event_receipt_fixture="$(make_fixture "p2-06-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-06 receipt drift' >>"$event_receipt_fixture/$file_path" ;;
    *.sql) printf '%s\n' '-- P2-06 receipt drift' >>"$event_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-06 receipt drift' >>"$event_receipt_fixture/$file_path" ;;
  esac
  git -C "$event_receipt_fixture" add "$file_path"
  if (cd "$event_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-06 event append receipt drift was accepted: $file_path"
  fi
done

missing_event_appender="$(make_fixture missing-p2-06-event-appender)"
rm -f "$missing_event_appender/internal/events/store/appender.go"
git -C "$missing_event_appender" add -u internal/events/store/appender.go
if (cd "$missing_event_appender" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-06 event appender was accepted"
fi

for file_path in \
  migrations/00003_settings.sql \
  sqlc.yaml \
  internal/config/port/port.go \
  internal/config/registry.go \
  internal/config/registry_test.go \
  internal/config/app/manager.go \
  internal/config/app/manager_test.go \
  internal/config/store/repository.go \
  internal/config/store/queries/settings.sql \
  internal/config/store/generated/db.go \
  internal/config/store/generated/models.go \
  internal/config/store/generated/querier.go \
  internal/config/store/generated/settings.sql.go \
  acceptance/p2s03/doc.go \
  acceptance/p2s03/settings_integration_test.go \
  acceptance/p1s11/contracts_test.go \
  docs/execution/slices/P2-03.md \
  docs/evidence/slices/P2-03-registry-tests.md; do
  settings_receipt_fixture="$(make_fixture "p2-03-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-03 receipt drift' >>"$settings_receipt_fixture/$file_path" ;;
    *.sql) printf '%s\n' '-- P2-03 receipt drift' >>"$settings_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-03 receipt drift' >>"$settings_receipt_fixture/$file_path" ;;
  esac
  git -C "$settings_receipt_fixture" add "$file_path"
  if (cd "$settings_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-03 settings boundary receipt drift was accepted: $file_path"
  fi
done

missing_settings_registry="$(make_fixture missing-p2-03-settings-registry)"
rm -f "$missing_settings_registry/internal/config/registry.go"
git -C "$missing_settings_registry" add -u internal/config/registry.go
if (cd "$missing_settings_registry" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-03 settings registry was accepted"
fi

for file_path in \
  acceptance/p0s01/process_blackbox.sh \
  acceptance/p0s01/static_contract.sh \
  acceptance/p2s04/doc.go \
  acceptance/p2s04/queue_isolation_integration_test.go \
  cmd/aicrm/main.go \
  cmd/aicrm/components.go \
  cmd/aicrm/components_test.go \
  internal/config/schema.go \
  internal/config/schema_test.go \
  internal/platform/jobqueue/client.go \
  internal/platform/jobqueue/client_test.go \
  internal/platform/jobqueue/queue.go \
  internal/platform/jobqueue/queue_policy_test.go \
  scripts/check_arch_imports.go \
  scripts/test_arch_imports.sh \
  docs/execution/slices/P2-04.md \
  docs/evidence/slices/P2-04-queue-policy-tests.md; do
  queue_receipt_fixture="$(make_fixture "p2-04-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-04 receipt drift' >>"$queue_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-04 receipt drift' >>"$queue_receipt_fixture/$file_path" ;;
  esac
  git -C "$queue_receipt_fixture" add "$file_path"
  if (cd "$queue_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-04 River queue isolation receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/platform/jobqueue/client.go internal/platform/jobqueue/queue.go; do
  missing_jobqueue_file="$(make_fixture "missing-p2-04-${file_path##*/}")"
  rm -f "$missing_jobqueue_file/$file_path"
  git -C "$missing_jobqueue_file" add -u "$file_path"
  if (cd "$missing_jobqueue_file" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "missing P2-04 jobqueue boundary was accepted: $file_path"
  fi
done

non_fail_fast_p0s01="$(make_fixture non-fail-fast-p0s01)"
sed -i.bak 's#acceptance/p0s01/static_contract[.]sh && \\#acceptance/p0s01/static_contract.sh; \\#' "$non_fail_fast_p0s01/Makefile"
rm -f "$non_fail_fast_p0s01/Makefile.bak"
restage_make_receipt "$non_fail_fast_p0s01"
if (cd "$non_fail_fast_p0s01" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "non-fail-fast P0-S01 acceptance recipe was accepted"
fi

disconnected_p2s04="$(make_fixture disconnected-p2-s04-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s04-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s04/Makefile"
rm -f "$disconnected_p2s04/Makefile.bak"
restage_make_receipt "$disconnected_p2s04"
if (cd "$disconnected_p2s04" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S04 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s04="$(make_fixture hollow-p2-s04-target)"
sed -i.bak '/^p2-s04-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s04/Makefile"
rm -f "$hollow_p2s04/Makefile.bak"
restage_make_receipt "$hollow_p2s04"
if (cd "$hollow_p2s04" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S04 acceptance target was accepted"
fi

for file_path in \
  acceptance/p2s05/doc.go \
  acceptance/p2s05/scheduler_integration_test.go \
  cmd/aicrm/components.go \
  cmd/aicrm/scheduler.go \
  internal/platform/jobqueue/client.go \
  internal/platform/jobqueue/client_test.go \
  internal/platform/jobqueue/queue.go \
  internal/platform/scheduler/scheduler.go \
  internal/platform/scheduler/scheduler_test.go \
  scripts/check_arch_imports.go \
  scripts/test_arch_imports.sh \
  docs/execution/slices/P2-05.md; do
  scheduler_receipt_fixture="$(make_fixture "p2-05-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-05 receipt drift' >>"$scheduler_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-05 receipt drift' >>"$scheduler_receipt_fixture/$file_path" ;;
  esac
  git -C "$scheduler_receipt_fixture" add "$file_path"
  if (cd "$scheduler_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-05 scheduler receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/platform/scheduler/scheduler.go cmd/aicrm/scheduler.go; do
  missing_scheduler_file="$(make_fixture "missing-p2-05-${file_path##*/}")"
  rm -f "$missing_scheduler_file/$file_path"
  git -C "$missing_scheduler_file" add -u "$file_path"
  if (cd "$missing_scheduler_file" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "missing P2-05 unique scheduler boundary was accepted: $file_path"
  fi
done

disconnected_p2s05="$(make_fixture disconnected-p2-s05-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s05-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s05/Makefile"
rm -f "$disconnected_p2s05/Makefile.bak"
restage_make_receipt "$disconnected_p2s05"
if (cd "$disconnected_p2s05" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S05 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s05="$(make_fixture hollow-p2-s05-target)"
sed -i.bak '/^p2-s05-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s05/Makefile"
rm -f "$hollow_p2s05/Makefile.bak"
restage_make_receipt "$hollow_p2s05"
if (cd "$hollow_p2s05" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S05 acceptance target was accepted"
fi

for file_path in \
  acceptance/p2s07/doc.go \
  acceptance/p2s07/dispatcher_integration_test.go \
  cmd/aicrm/components.go \
  cmd/aicrm/scheduler.go \
  cmd/aicrm/scheduler_test.go \
  internal/events/port/port.go \
  internal/events/store/queries/event_log.sql \
  internal/events/store/generated/event_log.sql.go \
  internal/events/store/generated/models.go \
  internal/events/store/generated/querier.go \
  internal/events/dispatcher/dispatcher.go \
  internal/events/dispatcher/dispatcher_test.go \
  internal/events/dispatcher/jobs.go \
  docs/execution/slices/P2-07.md \
  docs/evidence/slices/P2-07-dispatcher-tests.md; do
  dispatcher_receipt_fixture="$(make_fixture "p2-07-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-07 receipt drift' >>"$dispatcher_receipt_fixture/$file_path" ;;
    *.sql) printf '%s\n' '-- P2-07 receipt drift' >>"$dispatcher_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-07 receipt drift' >>"$dispatcher_receipt_fixture/$file_path" ;;
  esac
  git -C "$dispatcher_receipt_fixture" add "$file_path"
  if (cd "$dispatcher_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-07 dispatcher receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/events/dispatcher/dispatcher.go internal/events/dispatcher/jobs.go; do
  missing_dispatcher_file="$(make_fixture "missing-p2-07-${file_path##*/}")"
  rm -f "$missing_dispatcher_file/$file_path"
  git -C "$missing_dispatcher_file" add -u "$file_path"
  if (cd "$missing_dispatcher_file" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "missing P2-07 dispatcher core was accepted: $file_path"
  fi
done

disconnected_p2s07="$(make_fixture disconnected-p2-s07-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s07-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s07/Makefile"
rm -f "$disconnected_p2s07/Makefile.bak"
restage_make_receipt "$disconnected_p2s07"
if (cd "$disconnected_p2s07" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S07 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s07="$(make_fixture hollow-p2-s07-target)"
sed -i.bak '/^p2-s07-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s07/Makefile"
rm -f "$hollow_p2s07/Makefile.bak"
restage_make_receipt "$hollow_p2s07"
if (cd "$hollow_p2s07" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S07 acceptance target was accepted"
fi

for file_path in \
  acceptance/p2s08/doc.go \
  acceptance/p2s08/gateway_blackbox_test.go \
  internal/platform/http/errors.go \
  internal/platform/http/gateway.go \
  internal/platform/http/gateway_test.go \
  docs/execution/slices/P2-08.md \
  docs/evidence/slices/P2-08-http-tests.md; do
  http_receipt_fixture="$(make_fixture "p2-08-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-08 receipt drift' >>"$http_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-08 receipt drift' >>"$http_receipt_fixture/$file_path" ;;
  esac
  git -C "$http_receipt_fixture" add "$file_path"
  if (cd "$http_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-08 HTTP gateway receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/platform/http/errors.go internal/platform/http/gateway.go; do
  missing_http_file="$(make_fixture "missing-p2-08-${file_path##*/}")"
  rm -f "$missing_http_file/$file_path"
  git -C "$missing_http_file" add -u "$file_path"
  if (cd "$missing_http_file" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "missing P2-08 HTTP gateway core was accepted: $file_path"
  fi
done

disconnected_p2s08="$(make_fixture disconnected-p2-s08-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s08-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s08/Makefile"
rm -f "$disconnected_p2s08/Makefile.bak"
restage_make_receipt "$disconnected_p2s08"
if (cd "$disconnected_p2s08" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S08 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s08="$(make_fixture hollow-p2-s08-target)"
sed -i.bak '/^p2-s08-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s08/Makefile"
rm -f "$hollow_p2s08/Makefile.bak"
restage_make_receipt "$hollow_p2s08"
if (cd "$hollow_p2s08" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S08 acceptance target was accepted"
fi

for file_path in \
  migrations/00004_auth.sql \
  internal/auth/port/port.go \
  internal/auth/app/service.go \
  internal/auth/app/service_test.go \
  internal/auth/http/handler.go \
  internal/auth/store/repository.go \
  internal/auth/store/queries/auth.sql \
  acceptance/p2s09/doc.go \
  acceptance/p2s09/session_integration_test.go \
  docs/execution/slices/P2-09.md \
  docs/evidence/slices/P2-09-auth-service-tests.md; do
  auth_receipt_fixture="$(make_fixture "p2-09-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-09 receipt drift' >>"$auth_receipt_fixture/$file_path" ;;
    *.sql) printf '%s\n' '-- P2-09 receipt drift' >>"$auth_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-09 receipt drift' >>"$auth_receipt_fixture/$file_path" ;;
  esac
  git -C "$auth_receipt_fixture" add "$file_path"
  if (cd "$auth_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-09 auth-session receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/auth/app/service.go internal/auth/http/handler.go migrations/00004_auth.sql; do
  missing_auth_file="$(make_fixture "missing-p2-09-${file_path//\//-}")"
  rm -f "$missing_auth_file/$file_path"
  git -C "$missing_auth_file" add -u "$file_path"
  if (cd "$missing_auth_file" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "missing P2-09 auth-session core was accepted: $file_path"
  fi
done

disconnected_p2s09="$(make_fixture disconnected-p2-s09-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s09-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s09/Makefile"
rm -f "$disconnected_p2s09/Makefile.bak"
restage_make_receipt "$disconnected_p2s09"
if (cd "$disconnected_p2s09" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S09 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s09="$(make_fixture hollow-p2-s09-target)"
sed -i.bak '/^p2-s09-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s09/Makefile"
rm -f "$hollow_p2s09/Makefile.bak"
restage_make_receipt "$hollow_p2s09"
if (cd "$hollow_p2s09" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S09 acceptance target was accepted"
fi

for file_path in \
  api/openapi.yaml \
  internal/auth/port/port.go \
  internal/auth/port/port_test.go \
  internal/auth/app/policy.go \
  internal/auth/app/policy_test.go \
  internal/auth/http/authorization.go \
  acceptance/p2s10/doc.go \
  acceptance/p2s10/rbac_contract_test.go \
  docs/execution/slices/P2-10.md \
  docs/evidence/slices/P2-10-rbac-tests.md; do
  rbac_receipt_fixture="$(make_fixture "p2-10-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-10 receipt drift' >>"$rbac_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-10 receipt drift' >>"$rbac_receipt_fixture/$file_path" ;;
  esac
  git -C "$rbac_receipt_fixture" add "$file_path"
  if (cd "$rbac_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-10 RBAC receipt drift was accepted: $file_path"
  fi
done

for file_path in internal/auth/app/policy.go internal/auth/http/authorization.go; do
  missing_rbac_file="$(make_fixture "missing-p2-10-${file_path//\//-}")"
  rm -f "$missing_rbac_file/$file_path"
  git -C "$missing_rbac_file" add -u "$file_path"
  if (cd "$missing_rbac_file" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "missing P2-10 RBAC core was accepted: $file_path"
  fi
done

disconnected_p2s10="$(make_fixture disconnected-p2-s10-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s10-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s10/Makefile"
rm -f "$disconnected_p2s10/Makefile.bak"
restage_make_receipt "$disconnected_p2s10"
if (cd "$disconnected_p2s10" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S10 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s10="$(make_fixture hollow-p2-s10-target)"
sed -i.bak '/^p2-s10-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s10/Makefile"
rm -f "$hollow_p2s10/Makefile.bak"
restage_make_receipt "$hollow_p2s10"
if (cd "$hollow_p2s10" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S10 acceptance target was accepted"
fi

for file_path in \
  cmd/aicrm/api.go \
  cmd/aicrm/api_test.go \
  cmd/aicrm/components.go \
  internal/platform/http/gateway.go \
  internal/platform/http/gateway_test.go \
  acceptance/p2s11/doc.go \
  acceptance/p2s11/gateway_router_test.go \
  docs/execution/slices/P2-11.md \
  docs/evidence/slices/P2-11-gateway-tests.md; do
  gateway_router_receipt_fixture="$(make_fixture "p2-11-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P2-11 receipt drift' >>"$gateway_router_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-11 receipt drift' >>"$gateway_router_receipt_fixture/$file_path" ;;
  esac
  git -C "$gateway_router_receipt_fixture" add "$file_path"
  if (cd "$gateway_router_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-11 gateway/router receipt drift was accepted: $file_path"
  fi
done

missing_p2s11_router="$(make_fixture missing-p2-11-router)"
rm -f "$missing_p2s11_router/cmd/aicrm/api.go"
git -C "$missing_p2s11_router" add -u cmd/aicrm/api.go
if (cd "$missing_p2s11_router" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-11 API router was accepted"
fi

disconnected_p2s11="$(make_fixture disconnected-p2-s11-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p2-s11-acceptance([[:space:]]|$)/\1/' "$disconnected_p2s11/Makefile"
rm -f "$disconnected_p2s11/Makefile.bak"
restage_make_receipt "$disconnected_p2s11"
if (cd "$disconnected_p2s11" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-S11 acceptance target disconnected from ci-go was accepted"
fi

hollow_p2s11="$(make_fixture hollow-p2-s11-target)"
sed -i.bak '/^p2-s11-acceptance:$/ { n; s/.*/\t@true/; }' "$hollow_p2s11/Makefile"
rm -f "$hollow_p2s11/Makefile.bak"
restage_make_receipt "$hollow_p2s11"
if (cd "$hollow_p2s11" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P2-S11 acceptance target was accepted"
fi

for file_path in \
  web/src/main.tsx \
  web/src/main.test.tsx \
  web/src/shell.css \
  docs/execution/slices/P2-12.md \
  docs/evidence/slices/P2-12-web-shell.md; do
  web_shell_receipt_fixture="$(make_fixture "p2-12-receipt-${file_path//\//-}")"
  case "$file_path" in
    *.tsx) printf '%s\n' '// P2-12 receipt drift' >>"$web_shell_receipt_fixture/$file_path" ;;
    *.css) printf '%s\n' '/* P2-12 receipt drift */' >>"$web_shell_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-12 receipt drift' >>"$web_shell_receipt_fixture/$file_path" ;;
  esac
  git -C "$web_shell_receipt_fixture" add "$file_path"
  if (cd "$web_shell_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-12 Web shell receipt drift was accepted: $file_path"
  fi
done

p2s13_card_fixture="$(make_fixture p2-13-card-receipt)"
printf '%s\n' '# P2-13 receipt drift' >>"$p2s13_card_fixture/docs/execution/slices/P2-13.md"
git -C "$p2s13_card_fixture" add docs/execution/slices/P2-13.md
if (cd "$p2s13_card_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-13 slice card receipt drift was accepted"
fi

for file_path in \
  package.json \
  web/src/auth.ts \
  web/src/auth.test.ts \
  web/src/auth-ui.tsx \
  web/src/auth-ui.test.tsx \
  web/src/main.tsx \
  web/src/main.test.tsx \
  web/src/shell.css \
  docs/execution/slices/P2-13.md \
  docs/evidence/slices/P2-13-auth-ui.md; do
  p2s13_receipt_fixture="$(make_fixture "p2-13-receipt-${file_path//\//-}")"
  case "$file_path" in
    package.json) sed -i.bak 's/"test":/"test_drift":/' "$p2s13_receipt_fixture/$file_path"; rm -f "$p2s13_receipt_fixture/$file_path.bak" ;;
    *.ts|*.tsx) printf '%s\n' '// P2-13 receipt drift' >>"$p2s13_receipt_fixture/$file_path" ;;
    *.css) printf '%s\n' '/* P2-13 receipt drift */' >>"$p2s13_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# P2-13 receipt drift' >>"$p2s13_receipt_fixture/$file_path" ;;
  esac
  git -C "$p2s13_receipt_fixture" add "$file_path"
  if (cd "$p2s13_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P2-13 auth receipt drift was accepted: $file_path"
  fi
done

p2s14_card_fixture="$(make_fixture p2-14-card-receipt)"
printf '%s\n' '# P2-14 receipt drift' >>"$p2s14_card_fixture/docs/execution/slices/P2-14.md"
git -C "$p2s14_card_fixture" add docs/execution/slices/P2-14.md
if (cd "$p2s14_card_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P2-14 slice card receipt drift was accepted"
fi

missing_p2s12_router="$(make_fixture missing-p2-12-router)"
rm -f "$missing_p2s12_router/web/src/main.tsx"
git -C "$missing_p2s12_router" add -u web/src/main.tsx
if (cd "$missing_p2s12_router" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-12 Web router was accepted"
fi

missing_acceptance_helper="$(make_fixture missing-p2-00-helper)"
rm -f "$missing_acceptance_helper/acceptance/fixtures/postgres.go"
git -C "$missing_acceptance_helper" add -u acceptance/fixtures/postgres.go
if (cd "$missing_acceptance_helper" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P2-00 acceptance helper was accepted"
fi

disconnected_acceptance_target="$(make_fixture disconnected-p2-00-target)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]acceptance-fixtures([[:space:]]|$)/\1/' "$disconnected_acceptance_target/Makefile"
rm -f "$disconnected_acceptance_target/Makefile.bak"
restage_make_receipt "$disconnected_acceptance_target"
if (cd "$disconnected_acceptance_target" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without P2-00 acceptance fixtures was accepted"
fi

missing_acceptance_dsn="$(make_fixture missing-p2-00-workflow-dsn)"
sed -i.bak '/^          ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:/d' "$missing_acceptance_dsn/.github/workflows/application-go.yml"
rm -f "$missing_acceptance_dsn/.github/workflows/application-go.yml.bak"
git -C "$missing_acceptance_dsn" add .github/workflows/application-go.yml
workflow_digest="$(git -C "$missing_acceptance_dsn" show :.github/workflows/application-go.yml | sha256sum | awk '{print $1}')"
sed -i.bak -E "/^verify_index_sha256 \.github\/workflows\/application-go\.yml/{n;s/[0-9a-f]{64}/$workflow_digest/;}" "$missing_acceptance_dsn/scripts/check_repo_contract.sh"
rm -f "$missing_acceptance_dsn/scripts/check_repo_contract.sh.bak"
git -C "$missing_acceptance_dsn" add scripts/check_repo_contract.sh
if (cd "$missing_acceptance_dsn" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "application workflow without the P2-00 acceptance DSN was accepted"
fi

for file_path in .gitleaks.toml scripts/test_gitleaks_config.sh docs/execution/slices/M0-7.md; do
  gitleaks_receipt_fixture="$(make_fixture "gitleaks-receipt-${file_path##*/}")"
  printf '%s\n' '# gitleaks receipt drift' >>"$gitleaks_receipt_fixture/$file_path"
  git -C "$gitleaks_receipt_fixture" add "$file_path"
  if (cd "$gitleaks_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "gitleaks contract receipt drift was accepted: $file_path"
  fi
done

gitleaks_config_mode_fixture="$(make_fixture gitleaks-config-mode)"
chmod 755 "$gitleaks_config_mode_fixture/.gitleaks.toml"
git -C "$gitleaks_config_mode_fixture" add .gitleaks.toml
if (cd "$gitleaks_config_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "executable gitleaks config was accepted"
fi

gitleaks_runner_mode_fixture="$(make_fixture gitleaks-runner-mode)"
chmod 644 "$gitleaks_runner_mode_fixture/scripts/test_gitleaks_config.sh"
git -C "$gitleaks_runner_mode_fixture" add scripts/test_gitleaks_config.sh
if (cd "$gitleaks_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "non-executable gitleaks regression runner was accepted"
fi

weak_gitleaks_fixture="$(make_fixture weak-gitleaks-allowlist)"
sed -i.bak 's/condition = "AND"/condition = "OR"/' "$weak_gitleaks_fixture/.gitleaks.toml"
rm -f "$weak_gitleaks_fixture/.gitleaks.toml.bak"
git -C "$weak_gitleaks_fixture" add .gitleaks.toml
weak_digest="$(git -C "$weak_gitleaks_fixture" show :.gitleaks.toml | sha256sum | awk '{print $1}')"
sed -i.bak -E "/^verify_index_sha256 \.gitleaks\.toml/{n;s/[0-9a-f]{64}/$weak_digest/;}" \
  "$weak_gitleaks_fixture/scripts/check_repo_contract.sh"
rm -f "$weak_gitleaks_fixture/scripts/check_repo_contract.sh.bak"
git -C "$weak_gitleaks_fixture" add scripts/check_repo_contract.sh
if (cd "$weak_gitleaks_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "OR-composed gitleaks allowlist was accepted"
fi

for kind in missing-config missing-runner; do
  disconnected_gitleaks_fixture="$(make_fixture "gitleaks-$kind")"
  case "$kind" in
    missing-config) sed -i.bak 's/ --config \.gitleaks\.toml//' "$disconnected_gitleaks_fixture/.github/workflows/secret-scan.yml" ;;
    missing-runner) sed -i.bak '/scripts\/test_gitleaks_config\.sh/d' "$disconnected_gitleaks_fixture/.github/workflows/secret-scan.yml" ;;
  esac
  rm -f "$disconnected_gitleaks_fixture/.github/workflows/secret-scan.yml.bak"
  git -C "$disconnected_gitleaks_fixture" add .github/workflows/secret-scan.yml
  workflow_digest="$(git -C "$disconnected_gitleaks_fixture" show :.github/workflows/secret-scan.yml | sha256sum | awk '{print $1}')"
  sed -i.bak -E "/^verify_index_sha256 \.github\/workflows\/secret-scan\.yml/{n;s/[0-9a-f]{64}/$workflow_digest/;}" \
    "$disconnected_gitleaks_fixture/scripts/check_repo_contract.sh"
  rm -f "$disconnected_gitleaks_fixture/scripts/check_repo_contract.sh.bak"
  git -C "$disconnected_gitleaks_fixture" add scripts/check_repo_contract.sh
  if (cd "$disconnected_gitleaks_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "secret scan workflow without $kind protection was accepted"
  fi
done

for file_path in \
  docs/spec/AI-CRM-v2-执行方案.md \
  docs/spec/AI-CRM-v2-执行方案-v2-至P3.md \
  docs/spec/AI-CRM-v2-重构详细设计.md \
  docs/spec/SHA256SUMS \
  docs/execution/slices/M0-2.md; do
  spec_receipt_fixture="$(make_fixture "m0-2-receipt-${file_path##*/}")"
  printf '%s\n' '# M0-2 receipt drift' >>"$spec_receipt_fixture/$file_path"
  git -C "$spec_receipt_fixture" add "$file_path"
  if (cd "$spec_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "M0-2 spec receipt drift was accepted: $file_path"
  fi
done

m0_v2_mode_fixture="$(make_fixture m0-2-v2-mode)"
chmod 755 "$m0_v2_mode_fixture/docs/spec/AI-CRM-v2-执行方案-v2-至P3.md"
git -C "$m0_v2_mode_fixture" add docs/spec/AI-CRM-v2-执行方案-v2-至P3.md
if (cd "$m0_v2_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "M0-2 v2 spec mode drift was accepted"
fi

for file_path in tools/query-plan-gate/main.go tools/query-plan-gate/main_test.go scripts/test_query_plan_gate.sh docs/execution/slices/M0-3.md tools/go.mod; do
  query_plan_receipt_fixture="$(make_fixture "query-plan-receipt-${file_path##*/}")"
  case "$file_path" in *.go) printf '%s\n' '// query plan receipt drift' >>"$query_plan_receipt_fixture/$file_path" ;; *) printf '%s\n' '# query plan receipt drift' >>"$query_plan_receipt_fixture/$file_path" ;; esac
  git -C "$query_plan_receipt_fixture" add "$file_path"
  if (cd "$query_plan_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "query plan gate receipt drift was accepted: $file_path"
  fi
done

query_plan_runner_mode_fixture="$(make_fixture query-plan-runner-mode)"
chmod 644 "$query_plan_runner_mode_fixture/scripts/test_query_plan_gate.sh"
git -C "$query_plan_runner_mode_fixture" add scripts/test_query_plan_gate.sh
if (cd "$query_plan_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "query plan gate runner mode drift was accepted"
fi

for file_path in \
  docs/migration-mapping.jsonl \
  docs/evidence/p1/migration-lifecycle-index-6cb989c.json \
  docs/migration-mapping.md \
  tools/migration-mapping/main.go \
  tools/migration-mapping/main_test.go \
  docs/execution/slices/P1-C02.md; do
  migration_mapping_receipt_fixture="$(make_fixture "migration-mapping-receipt-${file_path##*/}")"
  case "$file_path" in *.go) printf '%s\n' '// P1-C02 receipt drift' >>"$migration_mapping_receipt_fixture/$file_path" ;; *) printf '%s\n' '# P1-C02 receipt drift' >>"$migration_mapping_receipt_fixture/$file_path" ;; esac
  git -C "$migration_mapping_receipt_fixture" add "$file_path"
  if (cd "$migration_mapping_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P1-C02 receipt drift was accepted: $file_path"
  fi
done

migration_index_mode_fixture="$(make_fixture migration-index-mode)"
chmod 755 "$migration_index_mode_fixture/docs/evidence/p1/migration-lifecycle-index-6cb989c.json"
git -C "$migration_index_mode_fixture" add docs/evidence/p1/migration-lifecycle-index-6cb989c.json
if (cd "$migration_index_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P1-C02 lifecycle index mode drift was accepted"
fi

for file_path in \
  docs/api-mapping.jsonl \
  docs/evidence/p1/route-triage.csv \
  docs/evidence/p1/g1-decisions.md \
  docs/execution/slices/G1-D01.md \
  tools/p1-reconciliation/main.go \
  tools/p1-reconciliation/main_test.go \
  docs/execution/slices/P1-C03.md; do
  reconciliation_receipt_fixture="$(make_fixture "p1-reconciliation-receipt-${file_path##*/}")"
  case "$file_path" in *.go) printf '%s\n' '// P1-C03 receipt drift' >>"$reconciliation_receipt_fixture/$file_path" ;; *) printf '%s\n' '# P1-C03 receipt drift' >>"$reconciliation_receipt_fixture/$file_path" ;; esac
  git -C "$reconciliation_receipt_fixture" add "$file_path"
  if (cd "$reconciliation_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P1-C03 receipt drift was accepted: $file_path"
  fi
done

for file_path in \
  docs/evidence/p1/g1-signoff-pack.md \
  docs/evidence/p1/feature-matrix-top20.md \
  docs/evidence/p1/migration-exceptions.md \
  docs/execution/slices/G1-D02.md \
  docs/spec/AI-CRM-v2-P2P3执行计划.md; do
  g1d02_receipt_fixture="$(make_fixture "g1d02-receipt-${file_path##*/}")"
  printf '%s\n' '# G1-D02 receipt drift' >>"$g1d02_receipt_fixture/$file_path"
  git -C "$g1d02_receipt_fixture" add "$file_path"
  if (cd "$g1d02_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "G1-D02 receipt drift was accepted: $file_path"
  fi
done

broken_reconciliation_ci_fixture="$(make_fixture broken-p1-reconciliation-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]p1-reconciliation-contract([[:space:]]|$)/\1/' "$broken_reconciliation_ci_fixture/Makefile"
rm -f "$broken_reconciliation_ci_fixture/Makefile.bak"
restage_make_receipt "$broken_reconciliation_ci_fixture"
if (cd "$broken_reconciliation_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without P1 reconciliation was accepted"
fi

hollow_reconciliation_fixture="$(make_fixture hollow-p1-reconciliation)"
sed -i.bak '/^p1-reconciliation-contract:$/ { n; s/.*/\t@true/; }' "$hollow_reconciliation_fixture/Makefile"
rm -f "$hollow_reconciliation_fixture/Makefile.bak"
restage_make_receipt "$hollow_reconciliation_fixture"
if (cd "$hollow_reconciliation_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P1 reconciliation target was accepted"
fi

for file_path in \
  api/openapi.yaml \
  api/oapi-codegen.yaml \
  api/oapi-codegen-p1-candidate.yaml \
  internal/api/generated/server.gen.go \
  internal/api/candidate/generated/server.gen.go \
  tools/openapi-contract/main.go \
  tools/openapi-contract/main_test.go \
  acceptance/p1s11/contracts_test.go \
  acceptance/p1s11/doc.go \
  docs/execution/slices/P1-S11.md \
  internal/auth/port/port.go \
  internal/contact/port/port.go \
  internal/identity/port/port.go \
  internal/platform/port/uow.go; do
  openapi_receipt_fixture="$(make_fixture "p1-s11-receipt-${file_path//\//-}")"
  case "$file_path" in *.go) printf '%s\n' '// P1-S11 receipt drift' >>"$openapi_receipt_fixture/$file_path" ;; *) printf '%s\n' '# P1-S11 receipt drift' >>"$openapi_receipt_fixture/$file_path" ;; esac
  git -C "$openapi_receipt_fixture" add "$file_path"
  if (cd "$openapi_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P1-S11 receipt drift was accepted: $file_path"
  fi
done

candidate_generated_mode_fixture="$(make_fixture p1-s11-candidate-mode)"
chmod 755 "$candidate_generated_mode_fixture/internal/api/candidate/generated/server.gen.go"
git -C "$candidate_generated_mode_fixture" add internal/api/candidate/generated/server.gen.go
if (cd "$candidate_generated_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P1-S11 candidate generated mode drift was accepted"
fi

broken_openapi_ci_fixture="$(make_fixture broken-p1-s11-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]openapi-p1-contract([[:space:]]|$)/\1/' "$broken_openapi_ci_fixture/Makefile"
rm -f "$broken_openapi_ci_fixture/Makefile.bak"
restage_make_receipt "$broken_openapi_ci_fixture"
if (cd "$broken_openapi_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without P1-S11 OpenAPI contract was accepted"
fi

hollow_openapi_fixture="$(make_fixture hollow-p1-s11-openapi)"
sed -i.bak '/^openapi-p1-contract:$/ { n; s/.*/\t@true/; }' "$hollow_openapi_fixture/Makefile"
rm -f "$hollow_openapi_fixture/Makefile.bak"
restage_make_receipt "$hollow_openapi_fixture"
if (cd "$hollow_openapi_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P1-S11 OpenAPI target was accepted"
fi

missing_candidate_generator_fixture="$(make_fixture missing-p1-s11-candidate-generator)"
sed -i.bak '/oapi-codegen-p1-candidate.yaml/d' "$missing_candidate_generator_fixture/Makefile"
rm -f "$missing_candidate_generator_fixture/Makefile.bak"
restage_make_receipt "$missing_candidate_generator_fixture"
if (cd "$missing_candidate_generator_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "OpenAPI generation without the isolated P1 candidate server was accepted"
fi

broken_query_plan_ci_fixture="$(make_fixture broken-query-plan-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]query-plan-gate-test([[:space:]]|$)/\1/' "$broken_query_plan_ci_fixture/Makefile"
rm -f "$broken_query_plan_ci_fixture/Makefile.bak"
restage_make_receipt "$broken_query_plan_ci_fixture"
if (cd "$broken_query_plan_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without query plan gate tests was accepted"
fi

hollow_query_plan_target_fixture="$(make_fixture hollow-query-plan-target)"
sed -i.bak '/^query-plan-gate:$/ { n; s/.*/\t@true/; }' "$hollow_query_plan_target_fixture/Makefile"
rm -f "$hollow_query_plan_target_fixture/Makefile.bak"
restage_make_receipt "$hollow_query_plan_target_fixture"
if (cd "$hollow_query_plan_target_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow query plan gate target was accepted"
fi

make_gitless_matrix_fixture() {
  local fixture
  fixture="$(make_fixture "$1")"
  rm -rf "$fixture/.git"
  [[ ! -e "$fixture/.git" ]] || fail "feature matrix fixture retained Git metadata"
  printf '%s\n' "$fixture"
}
assert_matrix_rejected() {
  local name="$1" fixture="$2" expected="$3" log
  log="$test_root/$name.log"
  if (cd / && /bin/bash "$fixture/scripts/check_feature_matrix_contract.sh" >"$log" 2>&1); then
    fail "feature matrix negative was accepted: $name"
  fi
  grep -Fq "$expected" "$log" || fail "feature matrix negative missed its diagnostic: $name"
}

matrix_valid_fixture="$(make_gitless_matrix_fixture feature-matrix-valid)"
printf '%s\n' '#!/bin/sh' 'exit 97' >"$matrix_valid_fixture/hostile-bash-env"; chmod 755 "$matrix_valid_fixture/hostile-bash-env"
matrix_output="$(cd / && BASH_ENV="$matrix_valid_fixture/hostile-bash-env" ENV="$matrix_valid_fixture/hostile-bash-env" PATH=/nonexistent MAKEFLAGS="SHELL=$matrix_valid_fixture/hostile-bash-env .SHELLFLAGS=-c\\ true" /usr/bin/make -C "$matrix_valid_fixture" --no-print-directory feature-matrix-contract)" ||
  fail "valid hostile-environment feature matrix was rejected without Git"
[[ "$matrix_output" == 'feature-matrix-contract: PASS (rows=293)' ]] || fail "feature matrix contract did not report the unique PASS"
set +e
completion_output="$(cd / && /bin/bash "$matrix_valid_fixture/scripts/check_feature_matrix_contract.sh" --completion p1 2>&1)"
completion_status=$?
set -e
[[ "$completion_status" -eq 0 && "$completion_output" == 'feature-matrix-completion: PASS phase=p1 rows=293 synthetic=0 staging=0 production=0' ]] ||
  fail "P1 completion did not report the signed G1-D02 state"

assert_completion() {
  local fixture="$1" phase="$2" expected="$3" output result
  set +e
  output="$(cd / && /bin/bash "$fixture/scripts/check_feature_matrix_contract.sh" --completion "$phase" 2>&1)"; result=$?
  set -e
  [[ "$result" -eq 2 && "$output" == "$expected" ]] || fail "completion level drifted: $phase/$expected"
}
synthetic_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-synthetic)"
sed -i.bak -e '2s/,"MIGRATE","NOT_STARTED","NOT_RUN","APPROVED"/,"MIGRATE","IMPLEMENTED","SYNTHETIC_PASS","APPROVED"/' \
  -e '2s#,"","",""$#,"pr=https://github.com/qianlan33333-png/AI-CRM-v2/pull/1;merge_sha=84b893aef66f8be0074b25894debb95bbbdd975c;tests=unit;paths=internal/example","method=fixture;command=make-test",""#' \
  "$synthetic_matrix_fixture/docs/feature-matrix.csv"; rm -f "$synthetic_matrix_fixture/docs/feature-matrix.csv.bak"
/bin/bash "$synthetic_matrix_fixture/scripts/check_feature_matrix_contract.sh" >/dev/null || fail "valid synthetic evidence was rejected"
assert_completion "$synthetic_matrix_fixture" p4 'feature-matrix-completion: PENDING phase=p4 rows=293 pending=292 synthetic=1 staging=0 production=0'
assert_completion "$synthetic_matrix_fixture" p5 'feature-matrix-completion: PENDING phase=p5 rows=293 pending=293 synthetic=1 staging=0 production=0'
staging_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-staging)"
sed -i.bak -e '2s/,"MIGRATE","NOT_STARTED","NOT_RUN","APPROVED"/,"MIGRATE","IMPLEMENTED","STAGING_PASS","APPROVED"/' \
  -e '2s#,"","",""$#,"pr=https://github.com/qianlan33333-png/AI-CRM-v2/pull/1;merge_sha=84b893aef66f8be0074b25894debb95bbbdd975c;tests=unit;paths=internal/example","environment=staging;build_sha=84b893aef66f8be0074b25894debb95bbbdd975c;time=2026-08-09T00:00:00Z;evidence=log",""#' \
  "$staging_matrix_fixture/docs/feature-matrix.csv"; rm -f "$staging_matrix_fixture/docs/feature-matrix.csv.bak"
/bin/bash "$staging_matrix_fixture/scripts/check_feature_matrix_contract.sh" >/dev/null || fail "valid staging evidence was rejected"
assert_completion "$staging_matrix_fixture" p5 'feature-matrix-completion: PENDING phase=p5 rows=293 pending=292 synthetic=0 staging=1 production=0'
production_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-production)"
sed -i.bak -e '2s/,"MIGRATE","NOT_STARTED","NOT_RUN","APPROVED"/,"MIGRATE","IMPLEMENTED","PRODUCTION_PASS","APPROVED"/' \
  -e '2s#,"","",""$#,"pr=https://github.com/qianlan33333-png/AI-CRM-v2/pull/1;merge_sha=84b893aef66f8be0074b25894debb95bbbdd975c;tests=unit;paths=internal/example","environment=production;build_sha=84b893aef66f8be0074b25894debb95bbbdd975c;time=2026-08-09T00:00:00Z;evidence=receipt;authorization=fixture",""#' \
  "$production_matrix_fixture/docs/feature-matrix.csv"; rm -f "$production_matrix_fixture/docs/feature-matrix.csv.bak"
/bin/bash "$production_matrix_fixture/scripts/check_feature_matrix_contract.sh" >/dev/null || fail "valid production evidence was rejected"
assert_completion "$production_matrix_fixture" p5 'feature-matrix-completion: PENDING phase=p5 rows=293 pending=292 synthetic=0 staging=0 production=1'

duplicate_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-duplicate)"
sed -i.bak '3s/LEGACY-S05-002/LEGACY-S05-001/' "$duplicate_matrix_fixture/docs/feature-matrix.csv"; rm -f "$duplicate_matrix_fixture/docs/feature-matrix.csv.bak"
assert_matrix_rejected duplicate "$duplicate_matrix_fixture" 'duplicate feature_id'

enum_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-enum)"
sed -i.bak '2s/,"MIGRATE",/,"BROKEN",/' "$enum_matrix_fixture/docs/feature-matrix.csv"; rm -f "$enum_matrix_fixture/docs/feature-matrix.csv.bak"
assert_matrix_rejected enum "$enum_matrix_fixture" 'invalid disposition'

forged_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-forged-decision)"
sed -i.bak '2s/G1-D02-2026-08-10/G1-D02-FORGED/' "$forged_matrix_fixture/docs/feature-matrix.csv"; rm -f "$forged_matrix_fixture/docs/feature-matrix.csv.bak"
assert_matrix_rejected forged-decision "$forged_matrix_fixture" 'MIGRATE row lacks exact G1-D02 decision evidence'

target_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-target)"
sed -i.bak '2s/,""$/,"LEGACY-MISSING-999"/' "$target_matrix_fixture/docs/feature-matrix.csv"; rm -f "$target_matrix_fixture/docs/feature-matrix.csv.bak"
assert_matrix_rejected target "$target_matrix_fixture" 'dangling target: LEGACY-MISSING-999'

evidence_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-evidence)"
sed -i.bak '2s/,"NOT_STARTED","NOT_RUN"/,"IMPLEMENTED","NOT_RUN"/' "$evidence_matrix_fixture/docs/feature-matrix.csv"; rm -f "$evidence_matrix_fixture/docs/feature-matrix.csv.bak"
assert_matrix_rejected evidence "$evidence_matrix_fixture" 'IMPLEMENTED evidence lacks PR, merge SHA, tests, or paths'

malformed_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-malformed)"
printf '%s\n' '"' >>"$malformed_matrix_fixture/docs/feature-matrix.csv"
assert_matrix_rejected malformed "$malformed_matrix_fixture" 'unterminated quoted field'

nul_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-nul)"
nul_size="$(wc -c <"$nul_matrix_fixture/docs/feature-matrix.csv" | tr -d ' ')"
dd if="$nul_matrix_fixture/docs/feature-matrix.csv" of="$nul_matrix_fixture/docs/feature-matrix.csv.tmp" bs=1 count=$((nul_size - 1)) 2>/dev/null
printf '\0' >>"$nul_matrix_fixture/docs/feature-matrix.csv.tmp"; mv "$nul_matrix_fixture/docs/feature-matrix.csv.tmp" "$nul_matrix_fixture/docs/feature-matrix.csv"
assert_matrix_rejected nul "$nul_matrix_fixture" 'matrix contains NUL bytes'

noeol_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-no-final-lf)"
noeol_size="$(wc -c <"$noeol_matrix_fixture/docs/feature-matrix.csv" | tr -d ' ')"
dd if="$noeol_matrix_fixture/docs/feature-matrix.csv" of="$noeol_matrix_fixture/docs/feature-matrix.csv.tmp" bs=1 count=$((noeol_size - 1)) 2>/dev/null
mv "$noeol_matrix_fixture/docs/feature-matrix.csv.tmp" "$noeol_matrix_fixture/docs/feature-matrix.csv"
assert_matrix_rejected no-final-lf "$noeol_matrix_fixture" 'matrix must end with one LF'
long_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-long-row)"; awk 'BEGIN { for (i=0;i<4097;i++) printf "x"; print "" }' >>"$long_matrix_fixture/docs/feature-matrix.csv"; assert_matrix_rejected long-row "$long_matrix_fixture" 'physical row exceeds 4096 bytes'

deleted_matrix_fixture="$(make_gitless_matrix_fixture feature-matrix-deleted-row)"
awk 'NR != 2' "$deleted_matrix_fixture/docs/feature-matrix.csv" >"$deleted_matrix_fixture/docs/feature-matrix.csv.tmp"
mv "$deleted_matrix_fixture/docs/feature-matrix.csv.tmp" "$deleted_matrix_fixture/docs/feature-matrix.csv"
assert_matrix_rejected deleted-row "$deleted_matrix_fixture" 'anchor version or row count mismatch'

rewritten_anchor_fixture="$(make_gitless_matrix_fixture feature-matrix-rewritten-anchor)"
sed -i.bak '2s/293/292/' "$rewritten_anchor_fixture/docs/evidence/p1/feature-matrix-id-anchor.v1"; rm -f "$rewritten_anchor_fixture/docs/evidence/p1/feature-matrix-id-anchor.v1.bak"
assert_matrix_rejected rewritten-anchor "$rewritten_anchor_fixture" 'anchor is not the frozen revision'

for kind in mode symlink fifo; do
  shape_fixture="$(make_gitless_matrix_fixture "feature-matrix-$kind")"
  expected_shape_error='regular file required: docs/feature-matrix.csv'
  case "$kind" in
    mode) chmod 755 "$shape_fixture/docs/feature-matrix.csv"; expected_shape_error='mode must be exactly 0644: docs/feature-matrix.csv' ;;
    symlink) mv "$shape_fixture/docs/feature-matrix.csv" "$shape_fixture/docs/feature-matrix.csv.real"; ln -s feature-matrix.csv.real "$shape_fixture/docs/feature-matrix.csv" ;;
    fifo) mv "$shape_fixture/docs/feature-matrix.csv" "$shape_fixture/docs/feature-matrix.csv.real"; mkfifo "$shape_fixture/docs/feature-matrix.csv" ;;
  esac
  assert_matrix_rejected "$kind" "$shape_fixture" "$expected_shape_error"
done
scripts_link_fixture="$(make_gitless_matrix_fixture feature-matrix-scripts-symlink)"; mv "$scripts_link_fixture/scripts" "$scripts_link_fixture/scripts.real"; ln -s scripts.real "$scripts_link_fixture/scripts"
assert_matrix_rejected scripts-symlink "$scripts_link_fixture" 'real script directory required: scripts'

for file_path in scripts/check_feature_matrix_contract.sh docs/evidence/p1/feature-matrix-id-anchor.v1 docs/execution/slices/P1-S08.md; do
  receipt_fixture="$(make_fixture "feature-matrix-receipt-${file_path##*/}")"
  printf '%s\n' '# P1-S08 receipt drift' >>"$receipt_fixture/$file_path"
  git -C "$receipt_fixture" add "$file_path"
  if (cd "$receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P1-S08 receipt drift was accepted: $file_path"
  fi
done

matrix_mode_fixture="$(make_fixture feature-matrix-mode)"
chmod 755 "$matrix_mode_fixture/docs/feature-matrix.csv"
git -C "$matrix_mode_fixture" add docs/feature-matrix.csv
if (cd "$matrix_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then fail "feature matrix mode drift was accepted"; fi

broken_matrix_ci_fixture="$(make_fixture broken-feature-matrix-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]feature-matrix-contract([[:space:]]|$)/\1/' "$broken_matrix_ci_fixture/Makefile"; rm -f "$broken_matrix_ci_fixture/Makefile.bak"
restage_make_receipt "$broken_matrix_ci_fixture"
if (cd "$broken_matrix_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then fail "ci-go without feature matrix contract was accepted"; fi

hollow_matrix_target_fixture="$(make_fixture hollow-feature-matrix-target)"
sed -i.bak '/^feature-matrix-contract:$/ { n; s/.*/\t@true/; }' "$hollow_matrix_target_fixture/Makefile"; rm -f "$hollow_matrix_target_fixture/Makefile.bak"
restage_make_receipt "$hollow_matrix_target_fixture"
if (cd "$hollow_matrix_target_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then fail "hollow feature matrix contract target was accepted"; fi

for kind in eval include variable call alternate; do dynamic_matrix_target_fixture="$(make_fixture "dynamic-feature-matrix-$kind")"
  case "$kind" in eval) printf '%s\n' 'matrix_gate := feature-matrix-contract' '$(eval $(matrix_gate): ; @true)' >>"$dynamic_matrix_target_fixture/Makefile" ;; include) printf '%s\n' 'feature-matrix-contract:' $'\t@true' >"$dynamic_matrix_target_fixture/injected.mk"; printf '%s\n' 'include injected.mk' >>"$dynamic_matrix_target_fixture/Makefile" ;; variable) printf '%s\n' 'matrix_gate := feature-matrix-contract' '$(matrix_gate):' $'\t@true' >>"$dynamic_matrix_target_fixture/Makefile" ;; call) printf '%s\n' 'matrix_rule := feature-matrix-contract: ; @true' '$(call eval,$(matrix_rule))' >>"$dynamic_matrix_target_fixture/Makefile" ;; alternate) printf '%s\n' 'feature-matrix-contract:' $'\t@true' >"$dynamic_matrix_target_fixture/GNUmakefile" ;; esac
  restage_make_receipt "$dynamic_matrix_target_fixture"
  if (cd "$dynamic_matrix_target_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then fail "$kind-generated feature matrix override was accepted"; fi
done

for file_path in \
  tools/legacy-route-export/main.go \
  tools/legacy-route-export/main_test.go \
  docs/execution/slices/P1-S01.md \
  docs/evidence/p1/legacy-routes-6cb989c.json; do
  p1s01_receipt_fixture="$(make_fixture "p1s01-receipt-${file_path##*/}")"
  printf '%s\n' '# P1-S01 receipt drift' >>"$p1s01_receipt_fixture/$file_path"
  git -C "$p1s01_receipt_fixture" add "$file_path"
  if (cd "$p1s01_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P1-S01 receipt drift was accepted: $file_path"
  fi
done

broken_p1s01_ci_fixture="$(make_fixture broken-p1s01-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]legacy-route-export-test([[:space:]]|$)/\1/' \
  "$broken_p1s01_ci_fixture/Makefile"
rm -f "$broken_p1s01_ci_fixture/Makefile.bak"
git -C "$broken_p1s01_ci_fixture" add Makefile
if (cd "$broken_p1s01_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the P1-S01 exporter tests was accepted"
fi

for file_path in \
  tools/snapshot-gate/main.go \
  tools/snapshot-gate/main_test.go \
  acceptance/snapshots/catalog.v1.json \
  acceptance/p0s10/test_snapshot_gate.sh \
  docs/adr/ADR-001.md \
  docs/adr/ADR-010.md \
  docs/adr/ADR-011.md \
  docs/execution/slices/M0-5.md \
  docs/architecture/canonical.md \
  .github/CODEOWNERS; do
  snapshot_receipt_fixture="$(make_fixture "snapshot-gate-receipt-${file_path##*/}")"
  case "$file_path" in
    *.go) printf '%s\n' '// snapshot gate receipt drift' >>"$snapshot_receipt_fixture/$file_path" ;;
    *) printf '%s\n' '# snapshot gate receipt drift' >>"$snapshot_receipt_fixture/$file_path" ;;
  esac
  git -C "$snapshot_receipt_fixture" add "$file_path"
  if (cd "$snapshot_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "snapshot gate receipt drift was accepted: $file_path"
  fi
done

snapshot_runner_mode_fixture="$(make_fixture snapshot-gate-runner-mode)"
chmod 644 "$snapshot_runner_mode_fixture/acceptance/p0s10/test_snapshot_gate.sh"
git -C "$snapshot_runner_mode_fixture" add acceptance/p0s10/test_snapshot_gate.sh
if (cd "$snapshot_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "snapshot gate runner mode drift was accepted"
fi

broken_snapshot_ci_fixture="$(make_fixture broken-snapshot-gate-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]snapshot-gate-test([[:space:]]|$)/\1/' \
  "$broken_snapshot_ci_fixture/Makefile"
rm -f "$broken_snapshot_ci_fixture/Makefile.bak"
restage_make_receipt "$broken_snapshot_ci_fixture"
if (cd "$broken_snapshot_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without snapshot gate tests was accepted"
fi

hollow_snapshot_fixture="$(make_fixture hollow-snapshot-gate)"
sed -i.bak '/^snapshot-gate:$/ { n; s/.*/\t@true/; }' "$hollow_snapshot_fixture/Makefile"
rm -f "$hollow_snapshot_fixture/Makefile.bak"
restage_make_receipt "$hollow_snapshot_fixture"
if (cd "$hollow_snapshot_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow snapshot gate target was accepted"
fi

retired_replay_fixture="$(make_fixture retired-contract-replay-coexistence)"
mkdir -p "$retired_replay_fixture/tools/contract-replay"
printf '%s\n' 'retired' >"$retired_replay_fixture/tools/contract-replay/README"
git -C "$retired_replay_fixture" add tools/contract-replay/README
if (cd "$retired_replay_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "retired contract replay file_path was accepted"
fi

tracked_snapshot_actual_fixture="$(make_fixture tracked-snapshot-actual)"
printf '%s\n' '{"version":1,"cases":[]}' >"$tracked_snapshot_actual_fixture/acceptance/snapshots/actual.json"
git -C "$tracked_snapshot_actual_fixture" add acceptance/snapshots/actual.json
if (cd "$tracked_snapshot_actual_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "tracked handwritten snapshot actual was accepted"
fi

for file_path in \
  scripts/sourcepolicy/main.go \
  scripts/test_source_policy.sh \
  scripts/check_generated_sources.sh \
  scripts/test_gitless_generated_check.sh \
  AGENTS.md \
  docs/execution/slices/M0-6.md; do
  source_policy_receipt_fixture="$(make_fixture "source-policy-receipt-${file_path##*/}")"
  printf '%s\n' '# source policy receipt drift' >>"$source_policy_receipt_fixture/$file_path"
  git -C "$source_policy_receipt_fixture" add "$file_path"
  if (cd "$source_policy_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "source policy lint receipt drift was accepted: $file_path"
  fi
done

source_policy_runner_mode_fixture="$(make_fixture source-policy-runner-mode)"
chmod 644 "$source_policy_runner_mode_fixture/scripts/test_source_policy.sh"
git -C "$source_policy_runner_mode_fixture" add scripts/test_source_policy.sh
if (cd "$source_policy_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "source policy lint runner mode drift was accepted"
fi

generated_runner_mode_fixture="$(make_fixture generated-runner-mode)"
chmod 644 "$generated_runner_mode_fixture/scripts/test_gitless_generated_check.sh"
git -C "$generated_runner_mode_fixture" add scripts/test_gitless_generated_check.sh
if (cd "$generated_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "gitless generated runner mode drift was accepted"
fi

broken_source_policy_ci_fixture="$(make_fixture broken-source-policy-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]source-policy-lint-test([[:space:]]|$)/\1/' "$broken_source_policy_ci_fixture/Makefile"
rm -f "$broken_source_policy_ci_fixture/Makefile.bak"
git -C "$broken_source_policy_ci_fixture" add Makefile
if (cd "$broken_source_policy_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without source policy lint tests was accepted"
fi

for file_path in scripts/check_slice_inputs.sh scripts/test_slice_inputs.sh docs/execution/slices/M0-1.md; do
  slice_input_receipt_fixture="$(make_fixture "slice-input-receipt-${file_path##*/}")"
  printf '%s\n' '# slice input receipt drift' >>"$slice_input_receipt_fixture/$file_path"
  git -C "$slice_input_receipt_fixture" add "$file_path"
  if (cd "$slice_input_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "slice input contract receipt drift was accepted: $file_path"
  fi
done

slice_input_runner_mode_fixture="$(make_fixture slice-input-runner-mode)"
chmod 644 "$slice_input_runner_mode_fixture/scripts/test_slice_inputs.sh"
git -C "$slice_input_runner_mode_fixture" add scripts/test_slice_inputs.sh
if (cd "$slice_input_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "slice input contract runner mode drift was accepted"
fi

broken_slice_input_ci_fixture="$(make_fixture broken-slice-input-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]slice-input-contract-test([[:space:]]|$)/\1/' "$broken_slice_input_ci_fixture/Makefile"
rm -f "$broken_slice_input_ci_fixture/Makefile.bak"
restage_make_receipt "$broken_slice_input_ci_fixture"
if (cd "$broken_slice_input_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without slice input contract tests was accepted"
fi

hollow_slice_input_target_fixture="$(make_fixture hollow-slice-input-target)"
sed -i.bak '/^slice-input-contract:$/ { n; s/.*/\t@true/; }' "$hollow_slice_input_target_fixture/Makefile"
rm -f "$hollow_slice_input_target_fixture/Makefile.bak"
restage_make_receipt "$hollow_slice_input_target_fixture"
if (cd "$hollow_slice_input_target_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow slice input contract target was accepted"
fi

for file_path in scripts/ownership/main.go scripts/test_ownership.sh docs/architecture/table-ownership.yml; do
  ownership_receipt_fixture="$(make_fixture "ownership-receipt-${file_path##*/}")"
  printf '%s\n' '# ownership receipt drift' >>"$ownership_receipt_fixture/$file_path"
  git -C "$ownership_receipt_fixture" add "$file_path"
  if (cd "$ownership_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "ownership lint receipt drift was accepted: $file_path"
  fi
done

ownership_runner_mode_fixture="$(make_fixture ownership-runner-mode)"
chmod 644 "$ownership_runner_mode_fixture/scripts/test_ownership.sh"
git -C "$ownership_runner_mode_fixture" add scripts/test_ownership.sh
if (cd "$ownership_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ownership lint runner mode drift was accepted"
fi

broken_ownership_ci_fixture="$(make_fixture broken-ownership-ci)"
sed -i.bak -E '/^ci-go:/ s/[[:space:]]ownership-lint-test([[:space:]]|$)/\1/' "$broken_ownership_ci_fixture/Makefile"
rm -f "$broken_ownership_ci_fixture/Makefile.bak"
git -C "$broken_ownership_ci_fixture" add Makefile
if (cd "$broken_ownership_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without ownership lint tests was accepted"
fi

for file_path in scripts/check_arch_imports.go scripts/test_arch_imports.sh; do
  arch_receipt_fixture="$(make_fixture "arch-receipt-${file_path##*/}")"
  printf '%s\n' '// architecture receipt drift' >>"$arch_receipt_fixture/$file_path"
  git -C "$arch_receipt_fixture" add "$file_path"
  if (cd "$arch_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "architecture lint receipt drift was accepted: $file_path"
  fi
done

arch_runner_mode_fixture="$(make_fixture arch-runner-mode)"
chmod 644 "$arch_runner_mode_fixture/scripts/test_arch_imports.sh"
git -C "$arch_runner_mode_fixture" add scripts/test_arch_imports.sh
if (cd "$arch_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "architecture lint runner mode drift was accepted"
fi

broken_arch_ci_fixture="$(make_fixture broken-arch-ci)"
sed -i.bak -E \
  '/^ci-go:/ s/[[:space:]]arch-import-lint-test([[:space:]]|$)/\1/' \
  "$broken_arch_ci_fixture/Makefile"
rm -f "$broken_arch_ci_fixture/Makefile.bak"
git -C "$broken_arch_ci_fixture" add Makefile
if (cd "$broken_arch_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the architecture lint tests was accepted"
fi

missing_p0s02_runner_fixture="$(make_fixture missing-p0s02-runner)"
rm -f "$missing_p0s02_runner_fixture/acceptance/p0s02/test_static_contract.sh"
git -C "$missing_p0s02_runner_fixture" add -u acceptance/p0s02/test_static_contract.sh
if (cd "$missing_p0s02_runner_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P0-S02 static-contract runner was accepted"
fi

p0s02_runner_mode_fixture="$(make_fixture p0s02-runner-mode)"
chmod 644 "$p0s02_runner_mode_fixture/acceptance/p0s02/test_static_contract.sh"
git -C "$p0s02_runner_mode_fixture" add acceptance/p0s02/test_static_contract.sh
if (cd "$p0s02_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S02 static-contract runner mode drift was accepted"
fi

broken_p0s02_acceptance_fixture="$(make_fixture broken-p0s02-acceptance)"
sed -i.bak 's/^p0-s02-acceptance: p0-s02-contract$/p0-s02-acceptance:/' \
  "$broken_p0s02_acceptance_fixture/Makefile"
rm -f "$broken_p0s02_acceptance_fixture/Makefile.bak"
git -C "$broken_p0s02_acceptance_fixture" add Makefile
if (cd "$broken_p0s02_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S02 acceptance target without contract dependency was accepted"
fi

missing_p0s03_contract_fixture="$(make_fixture missing-p0s03-contract)"
rm -f "$missing_p0s03_contract_fixture/acceptance/p0s03/static_contract.sh"
git -C "$missing_p0s03_contract_fixture" add -u acceptance/p0s03/static_contract.sh
if (cd "$missing_p0s03_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing required P0-S03 contract file was accepted"
fi

for file_path in \
  acceptance/p0s03/query_contract_test.go \
  acceptance/p0s03/source_contract.go \
  acceptance/p0s03/test_contract.sh; do
  p0s03_receipt_fixture="$(make_fixture "p0s03-receipt-${file_path##*/}")"
  case "$file_path" in
    *.go) printf '%s\n' '// P0-S03 receipt drift' >>"$p0s03_receipt_fixture/$file_path" ;;
    *.sh) printf '%s\n' '# P0-S03 receipt drift' >>"$p0s03_receipt_fixture/$file_path" ;;
  esac
  git -C "$p0s03_receipt_fixture" add "$file_path"
  if (cd "$p0s03_receipt_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P0-S03 receipt drift was accepted: $file_path"
  fi
done

broken_p0s03_acceptance_fixture="$(make_fixture broken-p0s03-acceptance)"
sed -i.bak -E \
  's/^p0-s03-acceptance: p0-s03-contract$/p0-s03-acceptance:/' \
  "$broken_p0s03_acceptance_fixture/Makefile"
rm -f "$broken_p0s03_acceptance_fixture/Makefile.bak"
git -C "$broken_p0s03_acceptance_fixture" add Makefile
if (cd "$broken_p0s03_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S03 acceptance target without contract dependency was accepted"
fi

broken_p0s03_ci_fixture="$(make_fixture broken-p0s03-ci-dependency)"
sed -i.bak -E \
  '/^ci-go:/ s/[[:space:]]p0-s03-acceptance([[:space:]])/\1/' \
  "$broken_p0s03_ci_fixture/Makefile"
rm -f "$broken_p0s03_ci_fixture/Makefile.bak"
if grep -Eq '^ci-go:.*p0-s03-acceptance([[:space:]]|$)' "$broken_p0s03_ci_fixture/Makefile"; then
  fail "failed to remove the P0-S03 acceptance dependency"
fi
grep -Eq '^ci-go:.*p0-s04-acceptance([[:space:]]|$)' "$broken_p0s03_ci_fixture/Makefile" ||
  fail "P0-S03 fixture unexpectedly removed the P0-S04 acceptance dependency"
git -C "$broken_p0s03_ci_fixture" add Makefile
if (cd "$broken_p0s03_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the P0-S03 acceptance dependency was accepted"
fi

hollow_p0s03_acceptance_fixture="$(make_fixture hollow-p0s03-acceptance)"
sed -i.bak \
  's#acceptance/p0s03/static_contract.sh#true#' \
  "$hollow_p0s03_acceptance_fixture/Makefile"
rm -f "$hollow_p0s03_acceptance_fixture/Makefile.bak"
git -C "$hollow_p0s03_acceptance_fixture" add Makefile
if (cd "$hollow_p0s03_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "hollow P0-S03 acceptance recipe was accepted"
fi

duplicate_p0s03_acceptance_fixture="$(make_fixture duplicate-p0s03-acceptance)"
printf '%s\n' '' 'p0-s03-acceptance: p0-s03-contract' $'\t@true' \
  >>"$duplicate_p0s03_acceptance_fixture/Makefile"
git -C "$duplicate_p0s03_acceptance_fixture" add Makefile
if (cd "$duplicate_p0s03_acceptance_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "duplicate P0-S03 acceptance target was accepted"
fi

missing_p0s04_contract_fixture="$(make_fixture missing-p0s04-contract)"
rm -f "$missing_p0s04_contract_fixture/acceptance/p0s04/static_contract.sh"
git -C "$missing_p0s04_contract_fixture" add -u acceptance/p0s04/static_contract.sh
if (cd "$missing_p0s04_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing required P0-S04 contract file was accepted"
fi

replaced_p0s04_runner_fixture="$(make_fixture replaced-p0s04-runner)"
sed -i.bak \
  's#acceptance/p0s04/test_source_contract.sh#true#' \
  "$replaced_p0s04_runner_fixture/Makefile"
rm -f "$replaced_p0s04_runner_fixture/Makefile.bak"
git -C "$replaced_p0s04_runner_fixture" add Makefile
if (cd "$replaced_p0s04_runner_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "replaced P0-S04 contract runner was accepted"
fi

missing_p0s04_caller_hygiene_fixture="$(make_fixture missing-p0s04-caller-hygiene)"
sed -i.bak '/^unexport BASH_ENV ENV$/d' \
  "$missing_p0s04_caller_hygiene_fixture/Makefile"
rm -f "$missing_p0s04_caller_hygiene_fixture/Makefile.bak"
git -C "$missing_p0s04_caller_hygiene_fixture" add Makefile
if (cd "$missing_p0s04_caller_hygiene_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P0-S04 BASH_ENV and ENV caller hygiene was accepted"
fi

missing_p0s04_recipe_hygiene_fixture="$(make_fixture missing-p0s04-recipe-hygiene)"
sed -i.bak \
  's#@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh#@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh#' \
  "$missing_p0s04_recipe_hygiene_fixture/Makefile"
rm -f "$missing_p0s04_recipe_hygiene_fixture/Makefile.bak"
git -C "$missing_p0s04_recipe_hygiene_fixture" add Makefile
if (cd "$missing_p0s04_recipe_hygiene_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "missing P0-S04 contract recipe BASH_ENV and ENV hygiene was accepted"
fi

weakened_p0s04_coverage_fixture="$(make_fixture weakened-p0s04-coverage-proof)"
sed -i.bak 's/matches == 1 && !invalid/1/' \
  "$weakened_p0s04_coverage_fixture/Makefile"
rm -f "$weakened_p0s04_coverage_fixture/Makefile.bak"
git -C "$weakened_p0s04_coverage_fixture" add Makefile
if (cd "$weakened_p0s04_coverage_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "weakened P0-S04 positive coverage proof was accepted"
fi

broken_p0s04_ci_fixture="$(make_fixture broken-p0s04-ci-dependency)"
sed -i.bak -E \
  '/^ci-go:/ s/[[:space:]]p0-s04-acceptance([[:space:]])/\1/' \
  "$broken_p0s04_ci_fixture/Makefile"
rm -f "$broken_p0s04_ci_fixture/Makefile.bak"
git -C "$broken_p0s04_ci_fixture" add Makefile
if (cd "$broken_p0s04_ci_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "ci-go without the P0-S04 acceptance dependency was accepted"
fi

missing_p0s04_workflow_fixture="$(make_fixture missing-p0s04-workflow-integration)"
sed -i.bak '/^          make p0-s04-integration$/d' \
  "$missing_p0s04_workflow_fixture/.github/workflows/application-go.yml"
rm -f "$missing_p0s04_workflow_fixture/.github/workflows/application-go.yml.bak"
git -C "$missing_p0s04_workflow_fixture" add .github/workflows/application-go.yml
if (cd "$missing_p0s04_workflow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "application workflow without P0-S04 integration was accepted"
fi

missing_orval_gate_fixture="$(make_fixture missing-orval-gate)"
sed -i.bak 's/npm run orval:check && //' \
  "$missing_orval_gate_fixture/package.json"
rm -f "$missing_orval_gate_fixture/package.json.bak"
git -C "$missing_orval_gate_fixture" add package.json
if (cd "$missing_orval_gate_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "package scripts without the Orval consistency gate were accepted"
fi

orval_runner_mode_fixture="$(make_fixture orval-runner-mode)"
chmod 644 "$orval_runner_mode_fixture/scripts/test_orval_generated_check.sh"
git -C "$orval_runner_mode_fixture" add scripts/test_orval_generated_check.sh
if (cd "$orval_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "Orval generated-client runner mode drift was accepted"
fi

duplicate_p0s04_contract_fixture="$(make_fixture duplicate-p0s04-contract)"
printf '%s\n' '' 'p0-s04-contract:' $'\t@true' \
  >>"$duplicate_p0s04_contract_fixture/Makefile"
git -C "$duplicate_p0s04_contract_fixture" add Makefile
if (cd "$duplicate_p0s04_contract_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "duplicate or overriding P0-S04 contract target was accepted"
fi

multi_target_p0s04_override_fixture="$(make_fixture multi-target-p0s04-override)"
printf '%s\n' '' 'p0-s04-contract p0-s04-sidecar:' $'\t@true' \
  >>"$multi_target_p0s04_override_fixture/Makefile"
git -C "$multi_target_p0s04_override_fixture" add Makefile
if (cd "$multi_target_p0s04_override_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "multi-target P0-S04 contract override was accepted"
fi

p0s04_runner_mode_fixture="$(make_fixture p0s04-runner-mode)"
chmod 644 "$p0s04_runner_mode_fixture/acceptance/p0s04/test_static_contract.sh"
git -C "$p0s04_runner_mode_fixture" add acceptance/p0s04/test_static_contract.sh
if (cd "$p0s04_runner_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S04 static runner mode drift was accepted"
fi

p0s04_go_mode_fixture="$(make_fixture p0s04-go-mode)"
chmod 755 "$p0s04_go_mode_fixture/internal/platform/river/contract.go"
git -C "$p0s04_go_mode_fixture" add internal/platform/river/contract.go
if (cd "$p0s04_go_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "P0-S04 Go contract mode drift was accepted"
fi

for file_path in go.mod go.sum; do
  p0s04_module_pin_fixture="$(make_fixture "p0s04-module-pin-$file_path")"
  printf '%s\n' '# P0-S04 module pin drift' >>"$p0s04_module_pin_fixture/$file_path"
  git -C "$p0s04_module_pin_fixture" add "$file_path"
  if (cd "$p0s04_module_pin_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P0-S04 $file_path content drift was accepted"
  fi
done
for file_path in go.mod go.sum; do
  p0s04_module_mode_fixture="$(make_fixture "p0s04-module-mode-$file_path")"
  chmod 755 "$p0s04_module_mode_fixture/$file_path"
  git -C "$p0s04_module_mode_fixture" add "$file_path"
  if (cd "$p0s04_module_mode_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
    fail "P0-S04 $file_path mode drift was accepted"
  fi
done

make_bin="$(type -P make || true)"
[[ "$make_bin" == /* && -x "$make_bin" && -x /usr/bin/perl ]] || fail "trusted make/watchdog unavailable"
reset_p0s04_fixture() {
  local fixture="$1"
  rm -f "$fixture/internal/platform/river/runtime.go" \
    "$fixture/internal/platform/river/migrate.go" \
    "$fixture/internal/platform/river/runtime_test.go"
  rm -rf "$fixture/.git"
}
make_p0s04_fixture() {
  local fixture
  fixture="$(make_fixture "$1")"
  reset_p0s04_fixture "$fixture"
  printf '%s\n' "$fixture"
}
assert_p0s04_pending() {
  local fixture="$1" target="$2" gate output
  gate="${target#p0-s04-}"
  output="$(cd "$fixture" && "$make_bin" -o p0-s04-contract "$target" 2>&1)" ||
    fail "canonical empty P0-S04 $gate gate was rejected without Git"
  grep -Fqx "P0-S04 $gate gate: PENDING (implementation not present)" <<<"$output" ||
    fail "canonical empty P0-S04 $gate gate did not report PENDING"
}

p0s04_empty_fixture="$(make_fixture p0s04-canonical-empty)"
for file in runtime.go migrate.go runtime_test.go; do
  candidate="$p0s04_empty_fixture/internal/platform/river/$file"
  [[ -e "$candidate" || -L "$candidate" ]] || printf '%s\n' 'package platformriver' >"$candidate"
  [[ -f "$candidate" && ! -L "$candidate" ]] || fail "P0-S04 implementation-present fixture is incomplete: $file"
done
reset_p0s04_fixture "$p0s04_empty_fixture"
[[ ! -e "$p0s04_empty_fixture/.git" && ! -L "$p0s04_empty_fixture/.git" ]] ||
  fail "P0-S04 no-Git fixture retained .git"
for file in runtime.go migrate.go runtime_test.go; do
  [[ ! -e "$p0s04_empty_fixture/internal/platform/river/$file" && ! -L "$p0s04_empty_fixture/internal/platform/river/$file" ]] ||
    fail "P0-S04 canonical-empty fixture retained implementation: $file"
done
for target in p0-s04-acceptance p0-s04-integration; do
  assert_p0s04_pending "$p0s04_empty_fixture" "$target"
done
p0s04_coverage_fixture="$(make_p0s04_fixture p0s04-coverage-parser)"
for file in runtime.go migrate.go runtime_test.go; do : >"$p0s04_coverage_fixture/internal/platform/river/$file"; done
printf '%s\n' '#!/usr/bin/env bash' 'exit 0' >"$p0s04_coverage_fixture/acceptance/p0s04/static_contract.sh"
printf '%s\n' '#!/usr/bin/env bash' "printf '%s\\n' 'coverage: 12.5% of statements' 'ok  github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river  0.004s  coverage: [no statements]'" >"$p0s04_coverage_fixture/fake-go"
chmod 755 "$p0s04_coverage_fixture/acceptance/p0s04/static_contract.sh" "$p0s04_coverage_fixture/fake-go"
if (cd "$p0s04_coverage_fixture" && GO="$p0s04_coverage_fixture/fake-go" "$make_bin" -o p0-s04-contract p0-s04-acceptance >/dev/null 2>&1); then
  fail "P0-S04 acceptance accepted fake positive coverage with no statements"
fi
printf '%s\n' 'exit 97' >"$p0s04_empty_fixture/hostile-bash-env"
hostile_output="$(cd "$p0s04_empty_fixture" && BASH_ENV="$p0s04_empty_fixture/hostile-bash-env" ENV="$p0s04_empty_fixture/hostile-bash-env" "$make_bin" -o p0-s04-contract p0-s04-acceptance 2>&1)" ||
  fail "hostile BASH_ENV rejected canonical empty P0-S04 gate"
grep -Fqx 'P0-S04 acceptance gate: PENDING (implementation not present)' <<<"$hostile_output" ||
  fail "hostile BASH_ENV bypassed P0-S04 caller hygiene"
set +e
empty_path_output="$(cd "$p0s04_empty_fixture" && PATH=/nonexistent "$make_bin" -o p0-s04-contract p0-s04-acceptance 2>&1)"
empty_path_status=$?
set -e
[[ "$empty_path_status" -ne 0 ]] || fail "empty PATH P0-S04 gate did not fail closed"
for kind in hidden fifo subdir ancestor_symlink contract_symlink runtime_partial runtime_fifo; do
  fixture="$(make_p0s04_fixture "p0s04-invalid-$kind")"
  case "$kind" in
    hidden) : >"$fixture/internal/platform/river/.unexpected" ;;
    fifo) mkfifo "$fixture/internal/platform/river/unexpected-fifo" ;;
    subdir) mkdir "$fixture/internal/platform/river/unexpected-dir" ;;
    ancestor_symlink) mv "$fixture/internal/platform/river" "$fixture/internal/platform/river.real"; ln -s river.real "$fixture/internal/platform/river" ;;
    contract_symlink) mv "$fixture/internal/platform/river/contract.go" "$fixture/internal/platform/river/contract.go.real"; ln -s contract.go.real "$fixture/internal/platform/river/contract.go" ;;
    runtime_partial) : >"$fixture/internal/platform/river/runtime.go" ;;
    runtime_fifo) mkfifo "$fixture/internal/platform/river/runtime.go" ;;
  esac
  for target in p0-s04-acceptance p0-s04-integration; do
    set +e
    (cd "$fixture" && /usr/bin/perl -e 'alarm 5; exec @ARGV' "$make_bin" -o p0-s04-contract "$target" >/dev/null 2>&1); invalid_status=$?
    set -e
    [[ "$invalid_status" -ne 0 && "$invalid_status" -ne 142 ]] || fail "invalid P0-S04 canonical-empty shape was accepted or timed out: $kind/$target"
  done
done

explicit_key_workflow_fixture="$(make_fixture unexpected-explicit-key-workflow)"
printf '%s\n' \
  'name: forbidden explicit key fixture' \
  'on: [push]' \
  'permissions:' \
  '  contents: read' \
  'jobs:' \
  '  probe:' \
  '    runs-on: ubuntu-latest' \
  '    steps:' \
  '      - ? |-' \
  '          uses' \
  '        : actions/checkout@v4' \
  >"$explicit_key_workflow_fixture/.github/workflows/explicit-key.yml"
git -C "$explicit_key_workflow_fixture" add .github/workflows/explicit-key.yml
if (cd "$explicit_key_workflow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "unexpected workflow with an explicit block uses key was accepted"
fi

tagged_key_workflow_fixture="$(make_fixture unexpected-tagged-key-workflow)"
printf '%s\n' \
  'name: forbidden tagged key fixture' \
  'on: [push]' \
  'permissions:' \
  '  contents: read' \
  'jobs:' \
  '  probe:' \
  '    runs-on: ubuntu-latest' \
  '    steps:' \
  '      - ? !<tag:yaml.org,2002:str> |-' \
  '          uses' \
  '        : actions/checkout@v4' \
  >"$tagged_key_workflow_fixture/.github/workflows/tagged-key.yml"
git -C "$tagged_key_workflow_fixture" add .github/workflows/tagged-key.yml
if (cd "$tagged_key_workflow_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "unexpected workflow with a tagged block uses key was accepted"
fi

escaped_yaml_fixture="$(make_fixture escaped-yaml-policy)"
escaped_yaml_tmp="$escaped_yaml_fixture/.github/workflows/application-go.yml.tmp"
awk '
  { print }
  $0 == "    steps:" {
    slash = sprintf("%c", 92)
    print "      - { \"" slash "x75ses\": \"actions/checkout@" slash "x764\" }"
  }
' "$escaped_yaml_fixture/.github/workflows/application-go.yml" >"$escaped_yaml_tmp"
mv "$escaped_yaml_tmp" "$escaped_yaml_fixture/.github/workflows/application-go.yml"
git -C "$escaped_yaml_fixture" add .github/workflows/application-go.yml
grep -Fq '\x75ses' "$escaped_yaml_fixture/.github/workflows/application-go.yml" ||
  fail "failed to construct YAML escape fixture"
if (cd "$escaped_yaml_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "YAML escape that decodes to an unpinned uses key was accepted"
fi

overwritten_workflows_fixture="$(make_fixture overwritten-central-workflows)"
cp "$overwritten_workflows_fixture/.github/workflows/application-go.yml" \
  "$overwritten_workflows_fixture/.github/workflows/repo-contract.yml"
cp "$overwritten_workflows_fixture/.github/workflows/application-go.yml" \
  "$overwritten_workflows_fixture/.github/workflows/secret-scan.yml"
git -C "$overwritten_workflows_fixture" add .github/workflows
if (cd "$overwritten_workflows_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "overwritten central policy workflows were accepted"
fi

workflow_symlink_fixture="$(make_fixture central-workflow-symlink)"
rm -f "$workflow_symlink_fixture/.github/workflows/repo-contract.yml"
ln -s application-go.yml \
  "$workflow_symlink_fixture/.github/workflows/repo-contract.yml"
git -C "$workflow_symlink_fixture" add .github/workflows/repo-contract.yml
if (cd "$workflow_symlink_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "central policy workflow symlink was accepted"
fi

short_repo_timeout_fixture="$(make_fixture short-repo-contract-timeout)"
sed -i.bak 's/timeout-minutes: 30/timeout-minutes: 10/' \
  "$short_repo_timeout_fixture/.github/workflows/repo-contract.yml"
rm -f "$short_repo_timeout_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$short_repo_timeout_fixture" add .github/workflows/repo-contract.yml
short_repo_workflow_digest="$(git -C "$short_repo_timeout_fixture" show :.github/workflows/repo-contract.yml | sha256sum | awk '{print $1}')"
sed -i.bak -E "/^verify_index_sha256 \.github\/workflows\/repo-contract\.yml/{n;s/[0-9a-f]{64}/$short_repo_workflow_digest/;}" \
  "$short_repo_timeout_fixture/scripts/check_repo_contract.sh"
rm -f "$short_repo_timeout_fixture/scripts/check_repo_contract.sh.bak"
git -C "$short_repo_timeout_fixture" add scripts/check_repo_contract.sh
if (cd "$short_repo_timeout_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "repo-contract workflow with a 10-minute budget was accepted after receipt restaging"
fi

unpinned_fixture="$(make_fixture unpinned-action)"
sed -i.bak -E \
  's#actions/checkout@[0-9a-f]{40}#actions/checkout@v4#' \
  "$unpinned_fixture/.github/workflows/repo-contract.yml"
rm -f "$unpinned_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$unpinned_fixture" add .github/workflows/repo-contract.yml
grep -q 'actions/checkout@v4' \
  "$unpinned_fixture/.github/workflows/repo-contract.yml" ||
  fail "failed to construct unpinned Action fixture"
if (cd "$unpinned_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "unpinned GitHub Action was accepted"
fi

nonhex_fixture="$(make_fixture nonhex-action-ref)"
sed -i.bak -E \
  's#actions/checkout@[0-9a-f]{40}#actions/checkout@zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz#' \
  "$nonhex_fixture/.github/workflows/repo-contract.yml"
rm -f "$nonhex_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$nonhex_fixture" add .github/workflows/repo-contract.yml
if (cd "$nonhex_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "40-character non-hex Action reference was accepted"
fi

quoted_uses_fixture="$(make_fixture quoted-uses-key)"
sed -i.bak -E \
  's/^([[:space:]]*)uses: actions\/checkout@[0-9a-f]{40}/\1"uses": actions\/checkout@v4/' \
  "$quoted_uses_fixture/.github/workflows/repo-contract.yml"
rm -f "$quoted_uses_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$quoted_uses_fixture" add .github/workflows/repo-contract.yml
grep -q '"uses": actions/checkout@v4' \
  "$quoted_uses_fixture/.github/workflows/repo-contract.yml" ||
  fail "failed to construct quoted uses fixture"
if (cd "$quoted_uses_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "quoted uses key with an unpinned Action was accepted"
fi

flow_uses_fixture="$(make_fixture flow-uses-key)"
sed -i.bak \
  '/^    steps:$/a\
      - { name: Unpinned flow, uses: actions/checkout@v4 }
' \
  "$flow_uses_fixture/.github/workflows/repo-contract.yml"
rm -f "$flow_uses_fixture/.github/workflows/repo-contract.yml.bak"
git -C "$flow_uses_fixture" add .github/workflows/repo-contract.yml
grep -q -- 'uses: actions/checkout@v4' \
  "$flow_uses_fixture/.github/workflows/repo-contract.yml" ||
  fail "failed to construct flow-style uses fixture"
if (cd "$flow_uses_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "flow-style uses key with an unpinned Action was accepted"
fi

write_permission_fixture="$(make_fixture write-permission)"
sed -i.bak \
  's/^  contents: read$/  issues: write/' \
  "$write_permission_fixture/.github/workflows/application-go.yml"
rm -f "$write_permission_fixture/.github/workflows/application-go.yml.bak"
git -C "$write_permission_fixture" add .github/workflows/application-go.yml
if (cd "$write_permission_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow issues: write permission was accepted"
fi

quoted_write_fixture="$(make_fixture quoted-write-permission)"
sed -i.bak \
  '/^  contents: read$/a\
  issues: "write"
' \
  "$quoted_write_fixture/.github/workflows/application-go.yml"
rm -f "$quoted_write_fixture/.github/workflows/application-go.yml.bak"
git -C "$quoted_write_fixture" add .github/workflows/application-go.yml
if (cd "$quoted_write_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "quoted workflow issues: write permission was accepted"
fi

write_all_fixture="$(make_fixture write-all-permission)"
sed -i.bak \
  's/^permissions:$/permissions: write-all/' \
  "$write_all_fixture/.github/workflows/application-go.yml"
rm -f "$write_all_fixture/.github/workflows/application-go.yml.bak"
git -C "$write_all_fixture" add .github/workflows/application-go.yml
if (cd "$write_all_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow permissions: write-all was accepted"
fi

secrets_inherit_fixture="$(make_fixture secrets-inherit)"
sed -i.bak \
  's/^    steps:$/    "secrets": inherit/' \
  "$secrets_inherit_fixture/.github/workflows/application-go.yml"
rm -f "$secrets_inherit_fixture/.github/workflows/application-go.yml.bak"
git -C "$secrets_inherit_fixture" add .github/workflows/application-go.yml
if (cd "$secrets_inherit_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow secrets: inherit was accepted"
fi

secrets_context_fixture="$(make_fixture secrets-context)"
sed -i.bak \
  's/^      GOTOOLCHAIN: local$/      GOTOOLCHAIN: ${{ secrets }}/' \
  "$secrets_context_fixture/.github/workflows/application-go.yml"
rm -f "$secrets_context_fixture/.github/workflows/application-go.yml.bak"
git -C "$secrets_context_fixture" add .github/workflows/application-go.yml
if (cd "$secrets_context_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "workflow secrets context was accepted"
fi

envrc_fixture="$(make_fixture envrc-file_path)"
touch "$envrc_fixture/.envrc"
git -C "$envrc_fixture" add -f .envrc
if (cd "$envrc_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail ".envrc file_path was accepted"
fi

runtime_state_fixture="$(make_fixture runtime-state-file_path)"
mkdir -p "$runtime_state_fixture/runtime"
touch "$runtime_state_fixture/runtime/state.json"
git -C "$runtime_state_fixture" add -f runtime/state.json
if (cd "$runtime_state_fixture" && scripts/check_repo_contract.sh >/dev/null 2>&1); then
  fail "root runtime state file_path was accepted"
fi

secret_fixture="$(make_fixture staged-secret)"
safe_readme="$test_root/safe-readme.md"
cp "$secret_fixture/README.md" "$safe_readme"
fake_secret="AKI""A0000000000000000"
sed -i.bak "1s/$/ ${fake_secret}/" "$secret_fixture/README.md"
rm -f "$secret_fixture/README.md.bak"
git -C "$secret_fixture" add README.md
cp "$safe_readme" "$secret_fixture/README.md"
if (cd "$secret_fixture" && scripts/scan_sensitive_paths.sh >/dev/null 2>&1); then
  fail "secret staged in the index but absent from the worktree was accepted"
fi

echo "repo-contract-tests: PASS"
