SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
unexport BASH_ENV ENV

GO ?= go
TOOLS_MOD := tools/go.mod
ORVAL ?= ./node_modules/.bin/orval

.PHONY: version-check generate generate-openapi generate-sqlc generate-orval orval-check generate-check gitless-generate-test
.PHONY: mod-check migration-validate migration-guard-negative migration-integration
.PHONY: fmt-check vet test build vuln p0-s01-acceptance p0-s02-contract p0-s02-acceptance p0-s03-contract p0-s03-acceptance ci-go
.PHONY: p0-s04-contract p0-s04-acceptance p0-s04-integration
.PHONY: arch-import-lint arch-import-lint-test
.PHONY: ownership-lint ownership-lint-test

version-check:
	@test "$$($(GO) env GOVERSION)" = "go1.26.5"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/jackc/pgx/v5)" = "v5.9.2"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/go-chi/chi/v5)" = "v5.2.3"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/oapi-codegen/runtime)" = "v1.2.0"
	@test "$$($(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen --version | tail -n 1)" = "v2.6.0"
	@test "$$($(GO) tool -modfile=$(TOOLS_MOD) sqlc version)" = "v1.28.0"
	@test "$$($(GO) tool -modfile=$(TOOLS_MOD) goose -version)" = "goose version: v3.25.0"
	@version_output="$$($(GO) tool -modfile=$(TOOLS_MOD) govulncheck -version)"; \
		printf '%s\n' "$$version_output" | grep -Fqx 'Scanner: govulncheck@v1.6.0'

generate: generate-openapi generate-sqlc generate-orval

generate-openapi:
	@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \
		--config api/oapi-codegen.yaml api/openapi.yaml

generate-sqlc:
	@$(GO) tool -modfile=$(TOOLS_MOD) sqlc generate

generate-orval:
	@PATH="$(dir $(abspath $(ORVAL))):$$PATH" $(ORVAL) \
		--input api/openapi.yaml --output web/src/api/generated/health.ts \
		--client fetch --mode single --clean web/src/api/generated --prettier

orval-check:
	@$(MAKE) --no-print-directory generate-orval
	@git diff --exit-code -- web/src/api/generated
	@test -z "$$(git ls-files --others --exclude-standard -- web/src/api/generated)"
	@$(MAKE) --no-print-directory generate-orval
	@git diff --exit-code -- web/src/api/generated
	@test -z "$$(git ls-files --others --exclude-standard -- web/src/api/generated)"

generate-check:
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" scripts/check_generated_sources.sh

gitless-generate-test:
	@scripts/test_gitless_generated_check.sh

mod-check:
	@GOWORK=off $(GO) mod tidy -diff
	@GOWORK=off $(GO) -C tools mod tidy -diff

migration-validate:
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations validate

migration-guard-negative:
	@output="$$(mktemp -t aicrm-v2-migration-guard.XXXXXX)"; \
		trap 'rm -f "$$output"' EXIT; \
		set +e; \
		env -u ALLOW_DESTRUCTIVE_MIGRATION_TEST \
			-u MIGRATION_TEST_DATABASE_URL \
			DATABASE_URL=postgres://127.0.0.1:1/production \
			$(MAKE) --no-print-directory migration-integration >"$$output" 2>&1; \
		status=$$?; \
		set -e; \
		test "$$status" -eq 2; \
		grep -Fq 'ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 is required' "$$output"

migration-integration:
	@test "$${ALLOW_DESTRUCTIVE_MIGRATION_TEST:-}" = "1" || { \
		echo "ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 is required" >&2; exit 2; }
	@test "$${MIGRATION_TEST_DATABASE_URL:-}" = \
		"postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable" || { \
		echo "MIGRATION_TEST_DATABASE_URL must be the fixed loopback aicrm_test DSN" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres \
		"$$MIGRATION_TEST_DATABASE_URL" up
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres \
		"$$MIGRATION_TEST_DATABASE_URL" down
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres \
		"$$MIGRATION_TEST_DATABASE_URL" up

fmt-check:
	@files="$$(git ls-files '*.go')"; \
		test -z "$$files" || test -z "$$(gofmt -l $$files)"

vet:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) vet $$packages

test:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) test -race $$packages

build:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) build $$packages

vuln:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) tool -modfile=$(TOOLS_MOD) govulncheck $$packages

p0-s01-acceptance:
	@if [[ -f cmd/aicrm/main.go || \
		-f cmd/aicrm/components.go || \
		-f internal/platform/runtime/cli.go || \
		-f internal/platform/runtime/run.go || \
		-f internal/platform/runtime/runtime_test.go ]]; then \
		acceptance/p0s01/static_contract.sh; \
		$(GO) test -race -timeout=15s -tags=p0s01_acceptance ./acceptance/p0s01; \
		acceptance/p0s01/process_blackbox.sh; \
	else \
		echo "P0-S01 completion gate: PENDING (implementation not present)"; \
	fi

p0-s02-contract:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s02/test_static_contract.sh

p0-s02-acceptance: p0-s02-contract
	@if [[ -e internal/platform/http/health.go || -L internal/platform/http/health.go || \
		-e internal/platform/http/health_test.go || -L internal/platform/http/health_test.go ]]; then \
		acceptance/p0s02/static_contract.sh && \
		GOFLAGS=-mod=readonly $(GO) test -race -timeout=15s -tags=p0s02_acceptance ./acceptance/p0s02; \
	else \
		echo "P0-S02 completion gate: PENDING (implementation not present)"; \
	fi

p0-s03-contract:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s03/test_contract.sh

p0-s03-acceptance: p0-s03-contract
	@if [[ ! -e internal/platform/store/ping.go && ! -L internal/platform/store/ping.go && \
		! -e internal/platform/store/ping_test.go && ! -L internal/platform/store/ping_test.go ]]; then \
		echo "P0-S03 completion gate: PENDING (implementation not present)"; \
	else \
		acceptance/p0s03/static_contract.sh || exit $$?; \
		coverage_output="$$(GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -cover ./internal/platform/store 2>&1)" || { status=$$?; printf '%s\n' "$$coverage_output"; exit "$$status"; }; \
		printf '%s\n' "$$coverage_output"; \
		printf '%s\n' "$$coverage_output" | grep -Eq 'coverage: 100\.0% of statements' || { echo "P0-S03 completion gate: internal/platform/store coverage must be 100%" >&2; exit 1; }; \
		GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=15s -tags=p0s03_acceptance ./acceptance/p0s03; \
	fi

p0-s04-contract:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_static_contract.sh

p0-s04-acceptance: p0-s04-contract
	@shopt -s nullglob dotglob; \
		river_entries=(internal/platform/river/*); \
	if [[ -d internal && ! -L internal && \
		-d internal/platform && ! -L internal/platform && \
		-d internal/platform/river && ! -L internal/platform/river && \
		-f internal/platform/river/contract.go && ! -L internal/platform/river/contract.go && \
		"$${#river_entries[@]}" -eq 1 && "$${river_entries[0]}" = "internal/platform/river/contract.go" && \
		! -e internal/platform/river/runtime.go && ! -L internal/platform/river/runtime.go && \
		! -e internal/platform/river/migrate.go && ! -L internal/platform/river/migrate.go && \
		! -e internal/platform/river/runtime_test.go && ! -L internal/platform/river/runtime_test.go ]]; then \
		echo "P0-S04 acceptance gate: PENDING (implementation not present)"; \
	else \
		env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \
		coverage_output="$$(env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -cover -timeout=15s ./internal/platform/river 2>&1)" || { status=$$?; printf '%s\n' "$$coverage_output"; exit "$$status"; }; \
		printf '%s\n' "$$coverage_output"; \
		if ! printf '%s\n' "$$coverage_output" | awk '$$1 == "ok" && $$2 == "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river" { matches++; value = $$0; if (value !~ /coverage: [0-9]+([.][0-9]+)?% of statements$$/) invalid = 1; else { sub(/^.*coverage: /, "", value); sub(/% of statements$$/, "", value); if (value + 0 <= 0) invalid = 1 } } END { exit !(matches == 1 && !invalid) }'; then echo "P0-S04 acceptance gate: internal/platform/river must report positive numeric coverage" >&2; exit 1; fi; \
		env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=15s -tags=p0s04_acceptance -run '^(TestPinnedRiverPublicAPISurface|TestRuntimeLifecycleContract|TestRuntimeStartContextIsolated|TestRuntimeCancellationWinsSimultaneousStopped|TestInvalidMigrationDirection)$$' ./acceptance/p0s04; \
	fi

p0-s04-integration: p0-s04-contract
	@shopt -s nullglob dotglob; \
		river_entries=(internal/platform/river/*); \
	if [[ -d internal && ! -L internal && \
		-d internal/platform && ! -L internal/platform && \
		-d internal/platform/river && ! -L internal/platform/river && \
		-f internal/platform/river/contract.go && ! -L internal/platform/river/contract.go && \
		"$${#river_entries[@]}" -eq 1 && "$${river_entries[0]}" = "internal/platform/river/contract.go" && \
		! -e internal/platform/river/runtime.go && ! -L internal/platform/river/runtime.go && \
		! -e internal/platform/river/migrate.go && ! -L internal/platform/river/migrate.go && \
		! -e internal/platform/river/runtime_test.go && ! -L internal/platform/river/runtime_test.go ]]; then \
		echo "P0-S04 integration gate: PENDING (implementation not present)"; \
	else \
		env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \
		test "$${ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST:-}" = "1" || { echo "ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 is required" >&2; exit 2; }; \
		env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 $(GO) test -race -timeout=45s -tags=p0s04_acceptance -run '^TestOfficialMigrationUpDownUp$$' ./acceptance/p0s04; \
	fi

arch-import-lint:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/check_arch_imports.go -root .

arch-import-lint-test:
	@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_arch_imports.sh

ownership-lint:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/ownership/main.go -root .

ownership-lint-test:
	@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_ownership.sh

ci-go: version-check generate-check gitless-generate-test mod-check migration-validate migration-guard-negative fmt-check vet test build vuln p0-s01-acceptance p0-s02-acceptance p0-s03-acceptance p0-s04-acceptance arch-import-lint arch-import-lint-test ownership-lint ownership-lint-test
