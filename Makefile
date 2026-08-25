SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
unexport BASH_ENV ENV

GO ?= go
TOOLS_MOD := tools/go.mod
ORVAL ?= ./node_modules/.bin/orval

.PHONY: bootstrap-tools version-check generate generate-openapi generate-sqlc generate-orval orval-tool-check orval-check generate-check gitless-generate-test p4-dm01-migration-acceptance p4-dm01-two-pg-acceptance
.PHONY: mod-check migration-validate migration-guard-negative migration-integration
.PHONY: fmt-check vet test build vuln p0-s01-acceptance p0-s02-contract p0-s02-acceptance p0-s03-contract p0-s03-acceptance ci-go
.PHONY: p0-s04-contract p0-s04-acceptance p0-s04-integration
.PHONY: p4-h01a1-media-acceptance p4-h03-media-acceptance p4-miniprogram-library-ab-acceptance p4-hxc-sender-read-acceptance p4-delivery-lineage-0308-acceptance p4-customer-profile-tags-0301-acceptance p4-i01b-product-entitlement-acceptance
.PHONY: p4-f01a-survey-acceptance p4-f01ab-survey-acceptance p4-group-ops-acceptance
.PHONY: p4-c01-channel-acceptance p4-contact-policy-acceptance
.PHONY: p4-f01a-survey-acceptance
.PHONY: p4-c01-channel-acceptance p4-b02ab-tag-acceptance
.PHONY: p4-channel-entrants-acceptance
.PHONY: p4-outbound-campaign-handoff-acceptance
.PHONY: p4-outbound-campaign-dispatch-acceptance
.PHONY: p4-j01-coupon-acceptance p4-coupon-ab-acceptance p4-i03-order-acceptance p4-order-ab-acceptance p4-message-archive-ab-acceptance p4-operation-cycle-ab-acceptance p4-automation-agents-ab-acceptance p4-adminops-jobs-ab-acceptance p4-push-center-0421-0422-acceptance p4-admin-shell-ab-acceptance p4-execution-runtime-ab-acceptance
.PHONY: p4-ee01-internal-event-safe-export-acceptance
.PHONY: p4-rp01-release-plane-acceptance
.PHONY: p4-external-effects-runtime-acceptance
.PHONY: p4-data-migration-harness-acceptance
.PHONY: p4-b01-wecom-inbound-acceptance
.PHONY: p4-pe01-wechat-pay-settlement-acceptance
.PHONY: p4-automation-rules-runtime-acceptance
.PHONY: p2-s04-acceptance
.PHONY: p3-c07c-r3b-storage-acceptance p3-c07c-r3c-behavior-acceptance p3-o1a-r3-acceptance p3-o2-enqueue-one-acceptance p3-o3-enqueue-batch-acceptance p3-o4-sender-acceptance p3-o5-status-acceptance p3-o6a-retry-acceptance p3-o6b1-cancel-acceptance p3-o6b2-manual-retry-acceptance p3-o7-legacy-api-acceptance p4-w0-d01-automation-acceptance p4-w0-l01-stats-acceptance p4-a01-auth-acceptance p4-si00b-auth-acceptance
.PHONY: p2-s05-acceptance
.PHONY: p2-s07-acceptance
.PHONY: p2-s08-acceptance
.PHONY: p2-s09-acceptance
.PHONY: p2-s10-acceptance
.PHONY: p2-s11-acceptance
.PHONY: p2-s14-acceptance
.PHONY: p2-s15-acceptance
.PHONY: p2-s16-acceptance
.PHONY: p2-s18-acceptance
.PHONY: p3-c00-acceptance p3-c01a-contract p3-c01a-migration-acceptance p3-c02a-acceptance p3-c02b-acceptance p3-c02d-acceptance p3-c02e-acceptance p3-c03-migration-acceptance p3-c07a-acceptance p3-c07b2-acceptance p3-r4b-identity-storage-acceptance p3-s05a-acceptance p3-s05b-acceptance p3-w1-acceptance p3-w2-acceptance p3-w3-acceptance p3-w4-acceptance p3-w5-acceptance
.PHONY: g2-runtime-image-acceptance
.PHONY: g2-release-archive-contract
.PHONY: g2-web-edge-contract
.PHONY: arch-import-lint arch-import-lint-test
.PHONY: ownership-lint ownership-lint-test
.PHONY: acceptance-fixtures
.PHONY: source-policy-lint source-policy-lint-test
.PHONY: slice-input-contract slice-input-contract-test
.PHONY: snapshot-gate snapshot-gate-test
.PHONY: legacy-route-export-test
.PHONY: feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion
.PHONY: migration-mapping-contract migration-mapping-p1-completion
.PHONY: p1-reconciliation-contract
.PHONY: openapi-p1-contract
.PHONY: query-plan-gate query-plan-gate-test
.PHONY: replacement-baseline-contract
.PHONY: p3-c06a1-contract p3-c06a2-contract

version-check:
	@test "$$($(GO) env GOVERSION)" = "go1.26.6"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/jackc/pgx/v5)" = "v5.9.2"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/go-chi/chi/v5)" = "v5.3.1"
	@test "$$($(GO) list -m -f '{{.Version}}' golang.org/x/text)" = "v0.39.0"
	@test "$$($(GO) list -m -f '{{.Version}}' github.com/oapi-codegen/runtime)" = "v1.2.0"
	@test "$$($(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen --version | tail -n 1)" = "v2.6.0"
	@test "$$($(GO) tool -modfile=$(TOOLS_MOD) sqlc version)" = "v1.28.0"
	@test "$$($(GO) tool -modfile=$(TOOLS_MOD) goose -version)" = "goose version: v3.25.0"
	@version_output="$$($(GO) tool -modfile=$(TOOLS_MOD) govulncheck -version)"; \
		printf '%s\n' "$$version_output" | grep -Fqx 'Scanner: govulncheck@v1.6.0'

bootstrap-tools:
	@command -v $(GO) >/dev/null 2>&1 || { echo "bootstrap-tools: missing Go 1.26.6; install versions from .tool-versions" >&2; exit 2; }
	@command -v node >/dev/null 2>&1 || { echo "bootstrap-tools: missing Node.js 24.18.0; install versions from .tool-versions" >&2; exit 2; }
	@command -v npm >/dev/null 2>&1 || { echo "bootstrap-tools: missing npm 11.12.1; install versions from package.json" >&2; exit 2; }
	@test "$$(node --version)" = "v24.18.0" || { echo "bootstrap-tools: expected Node.js 24.18.0 from .tool-versions, got $$(node --version)" >&2; exit 2; }
	@test "$$(npm --version)" = "11.12.1" || { echo "bootstrap-tools: expected npm 11.12.1 from package.json, got $$(npm --version)" >&2; exit 2; }
	@GOWORK=off $(GO) mod download || { echo "bootstrap-tools: failed to download root Go dependencies" >&2; exit 2; }
	@GOWORK=off $(GO) mod download -modfile=$(TOOLS_MOD) || { echo "bootstrap-tools: failed to install pinned oapi-codegen, sqlc, goose, and govulncheck tools" >&2; exit 2; }
	@npm ci --ignore-scripts --no-audit --no-fund || { echo "bootstrap-tools: failed to install pinned Orval 7.21.0 and web tools" >&2; exit 2; }
	@$(MAKE) --no-print-directory version-check orval-tool-check

generate: generate-openapi generate-sqlc generate-orval

generate-openapi:
	@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \
		--config api/oapi-codegen.yaml api/openapi.yaml
	@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \
		--config api/oapi-codegen-p1-candidate.yaml api/openapi.yaml

generate-sqlc:
	@$(GO) tool -modfile=$(TOOLS_MOD) sqlc generate

orval-tool-check:
	@test -x "$(ORVAL)" || { echo "orval-tool-check: missing pinned Orval 7.21.0 at $(ORVAL); run 'make bootstrap-tools'" >&2; exit 2; }
	@test "$$($(ORVAL) --version 2>/dev/null)" = "7.21.0" || { echo "orval-tool-check: expected Orval 7.21.0 at $(ORVAL); run 'make bootstrap-tools'" >&2; exit 2; }

generate-orval: orval-tool-check
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
		grep -Fq 'ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 is required' "$$output"; \
		secret='dsn-password-sentinel'; \
		set +e; \
		ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 \
			MIGRATION_TEST_DATABASE_URL="postgres://postgres:$${secret}@127.0.0.1:55432/aicrm_test?sslmode=disable" \
			$(MAKE) --no-print-directory migration-integration >"$$output" 2>&1; \
		status=$$?; \
		set -e; \
		test "$$status" -eq 2; \
		grep -Fq 'MIGRATION_TEST_DATABASE_URL failed safe acceptance validation' "$$output"; \
		! grep -Fq "$$secret" "$$output"

migration-integration:
	@test "$${ALLOW_DESTRUCTIVE_MIGRATION_TEST:-}" = "1" || { \
		echo "ALLOW_DESTRUCTIVE_MIGRATION_TEST=1 is required" >&2; exit 2; }
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
		$(GO) run ./acceptance/fixtures/cmd/validate-database-url || exit 2
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres \
		"$$MIGRATION_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
		$(GO) test -race -count=1 -timeout=180s ./acceptance/campaign \
		-args -database-url "$$MIGRATION_TEST_DATABASE_URL"
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres \
		"$$MIGRATION_TEST_DATABASE_URL" down
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres \
		"$$MIGRATION_TEST_DATABASE_URL" up

p4-dm01-migration-acceptance:
	@test -n "$${P4_DM01_TEST_DATABASE_URL:-}" || { echo "P4_DM01_TEST_DATABASE_URL is required" >&2; exit 2; }
	@acceptance/contact/dm01_migration_pg16.sh

p4-dm01-two-pg-acceptance:
	@test -n "$${DM01_SOURCE_TEST_DATABASE_URL:-}" || { echo "DM01_SOURCE_TEST_DATABASE_URL is required" >&2; exit 2; }
	@test -n "$${DM01_TARGET_TEST_DATABASE_URL:-}" || { echo "DM01_TARGET_TEST_DATABASE_URL is required" >&2; exit 2; }
	@acceptance/dm01/run_two_pg16.sh

fmt-check:
	@files="$$(git ls-files '*.go')"; \
		test -z "$$files" || test -z "$$(gofmt -l $$files)"

vet:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) vet $$packages

test:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) test -race $$packages

build:
	@packages="$$(GOWORK=off $(GO) list -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) build $$packages

vuln:
	@packages="$$(GOWORK=off $(GO) list ./... | grep -Ev '(^|/)([.]git|node_modules|vendor)(/|$$)')"; test -n "$$packages"; $(GO) tool -modfile=$(TOOLS_MOD) govulncheck $$packages

p0-s01-acceptance:
	@if [[ -f cmd/aicrm/main.go || \
		-f cmd/aicrm/components.go || \
		-f internal/platform/runtime/cli.go || \
		-f internal/platform/runtime/run.go || \
		-f internal/platform/runtime/runtime_test.go ]]; then \
		acceptance/p0s01/static_contract.sh && \
		$(GO) test -race -timeout=15s -tags=p0s01_acceptance ./acceptance/p0s01 && \
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

p2-s04-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=45s ./acceptance/p2s04

p2-s05-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=45s ./acceptance/p2s05

p2-s07-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s07

p2-s08-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=45s ./acceptance/p2s08

p2-s09-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s09

p2-s10-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=30s ./acceptance/p2s10

p2-s11-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s11

p2-s14-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s14

p2-s15-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=60s ./acceptance/p2s15

p2-s16-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=60s ./acceptance/p2s16
	@/bin/bash -eu -o pipefail -c 'env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run ./acceptance/p2s16/snapshotgen | env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./snapshot-gate compare ../acceptance/snapshots/catalog.v1.json'

p3-w1-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./internal/wecom/callback ./internal/config ./cmd/aicrm

p3-w2-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./internal/wecom/callback ./internal/events/dispatcher ./cmd/aicrm

p3-w3-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./internal/wecom/client

p3-w4-acceptance:
	@test -n "$${WECOM_SYNC_TEST_DATABASE_URL:-}" || { echo "WECOM_SYNC_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$WECOM_SYNC_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly WECOM_SYNC_TEST_DATABASE_URL="$$WECOM_SYNC_TEST_DATABASE_URL" $(GO) test -race -count=1 -timeout=45s ./internal/wecom/client ./internal/wecom/app ./internal/wecom/store ./acceptance/wecom

p3-w5-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./internal/wecom/app

p2-s18-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 ./internal/platform/deployment ./cmd/aicrm-config
	@env -u BASH_ENV -u ENV acceptance/p2s18/test_tier_config.sh

p3-c00-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./acceptance/p3c00

p3-s05a-acceptance: override SHELL := /bin/bash
p3-s05a-acceptance: override .SHELLFLAGS := -eu -o pipefail -c
p3-s05a-acceptance:
	@test -n "$${SEGMENT_REFRESH_TEST_DATABASE_URL:-}" || { echo "SEGMENT_REFRESH_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$SEGMENT_REFRESH_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly SEGMENT_REFRESH_TEST_DATABASE_URL="$$SEGMENT_REFRESH_TEST_DATABASE_URL" $(GO) test -race -count=1 -run '^TestRefreshRequestRepositoryPersistsAcceptedReceiptAndHeavyJob$$' ./acceptance/segment

p3-s05b-acceptance: override SHELL := /bin/bash
p3-s05b-acceptance: override .SHELLFLAGS := -eu -o pipefail -c
p3-s05b-acceptance:
	@test -n "$${SEGMENT_CRUD_TEST_DATABASE_URL:-}" || { echo "SEGMENT_CRUD_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/segment/s05b_dedicated_database.sh

g2-runtime-image-acceptance:
	@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 ./cmd/aicrm-river-migrate
	@env -u BASH_ENV -u ENV acceptance/g2image/test_runtime_image.sh

g2-release-archive-contract:
	@env -u BASH_ENV -u ENV scripts/test_package_release_archive.sh

g2-web-edge-contract:
	@env -u BASH_ENV -u ENV scripts/test_g2_web_edge.sh

arch-import-lint:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/check_arch_imports.go -root .

arch-import-lint-test:
	@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_arch_imports.sh

ownership-lint:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/ownership/main.go -root .

ownership-lint-test:
	@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_ownership.sh

acceptance-fixtures: override SHELL := /bin/bash
acceptance-fixtures: override .SHELLFLAGS := -eu -o pipefail -c
acceptance-fixtures: override GO := go
acceptance-fixtures:
	@test -n "$${ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:-}" || { echo "ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$$ACCEPTANCE_FIXTURES_TEST_DATABASE_URL" $(GO) test -race -count=1 -timeout=30s ./acceptance/fixtures

source-policy-lint:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/sourcepolicy/main.go -root .

source-policy-lint-test:
	@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_source_policy.sh

slice-input-contract slice-input-contract-test: override SHELL := /bin/bash
slice-input-contract slice-input-contract-test: override .SHELLFLAGS := -eu -o pipefail -c

slice-input-contract:
	@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_slice_inputs.sh

slice-input-contract-test: slice-input-contract
	@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/test_slice_inputs.sh

snapshot-gate:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./snapshot-gate validate ../acceptance/snapshots/catalog.v1.json

snapshot-gate-test: snapshot-gate
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./snapshot-gate
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=15s ./snapshot-gate
	@env -u BASH_ENV -u ENV GO="$(GO)" acceptance/p0s10/test_snapshot_gate.sh

legacy-route-export-test:
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./legacy-route-export
	@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=15s ./legacy-route-export

feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: override SHELL := /bin/bash
feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: override .SHELLFLAGS := -eu -o pipefail -c

feature-matrix-contract:
	@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_feature_matrix_contract.sh

feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: feature-matrix-contract
	@phase="$@"; phase="$${phase#feature-matrix-}"; phase="$${phase%-completion}"; /usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_feature_matrix_contract.sh --completion "$$phase"

migration-mapping-contract migration-mapping-p1-completion: override SHELL := /bin/bash
migration-mapping-contract migration-mapping-p1-completion: override .SHELLFLAGS := -eu -o pipefail -c
migration-mapping-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./migration-mapping
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=15s ./migration-mapping
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./migration-mapping
migration-mapping-p1-completion: migration-mapping-contract
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./migration-mapping --completion

replacement-baseline-contract: override SHELL := /bin/bash
replacement-baseline-contract: override .SHELLFLAGS := -eu -o pipefail -c
replacement-baseline-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./replacement-baseline
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./replacement-baseline
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./replacement-baseline -check

p1-reconciliation-contract: override SHELL := /bin/bash
p1-reconciliation-contract: override .SHELLFLAGS := -eu -o pipefail -c
p1-reconciliation-contract: override GO := go
p1-reconciliation-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./p1-reconciliation
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=45s ./p1-reconciliation
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./p1-reconciliation

openapi-p1-contract: override SHELL := /bin/bash
openapi-p1-contract: override .SHELLFLAGS := -eu -o pipefail -c
openapi-p1-contract: override GO := go
openapi-p1-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./openapi-contract
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=120s ./openapi-contract
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./openapi-contract
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=20s ./acceptance/p1s11

query-plan-gate query-plan-gate-test: override SHELL := /bin/bash
query-plan-gate query-plan-gate-test: override .SHELLFLAGS := -eu -o pipefail -c
query-plan-gate query-plan-gate-test: override GO := go

query-plan-gate:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./query-plan-gate -root .. -base "$$QUERY_PLAN_BASE_SHA" -head "$$QUERY_PLAN_HEAD_SHA" -database-url "$$QUERY_PLAN_TEST_DATABASE_URL"

query-plan-gate-test: query-plan-gate
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./query-plan-gate
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./query-plan-gate
	@/usr/bin/env -u BASH_ENV -u ENV GO="$(GO)" /bin/bash scripts/test_query_plan_gate.sh

p3-c01a-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./internal/contact/acceptance ./acceptance/p3c01a

p3-c01a-migration-acceptance:
	@test -n "$${P3C01A_TEST_DATABASE_URL:-}" || { echo "P3C01A_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=30s ./acceptance/p3c01a -args -database-url "$$P3C01A_TEST_DATABASE_URL"

p3-r4b-identity-storage-acceptance:
	@test -n "$${P3R4B_TEST_DATABASE_URL:-}" || { echo "P3R4B_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=45s ./acceptance/identity -args -database-url "$$P3R4B_TEST_DATABASE_URL"

p3-c07c-r3b-storage-acceptance:
	@test -n "$${P3C07C_R3B_TEST_DATABASE_URL:-}" || { echo "P3C07C_R3B_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=45s ./acceptance/contact -args -external-event-storage-database-url "$$P3C07C_R3B_TEST_DATABASE_URL"

p3-c07c-r3c-behavior-acceptance:
	@test -n "$${P3C07C_R3C_TEST_DATABASE_URL:-}" || { echo "P3C07C_R3C_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s -run '^TestExternalEvent' ./acceptance/contact -args -database-url "$$P3C07C_R3C_TEST_DATABASE_URL"

p3-o1a-r3-acceptance:
	@test -n "$${P3O1A_R3_TEST_DATABASE_URL:-}" || { echo "P3O1A_R3_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=60s ./acceptance/outbound -args -database-url "$$P3O1A_R3_TEST_DATABASE_URL"

p3-o2-enqueue-one-acceptance:
	@test -n "$${P3O2_ENQUEUE_ONE_TEST_DATABASE_URL:-}" || { echo "P3O2_ENQUEUE_ONE_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=60s ./acceptance/outbound -args -database-url "$$P3O2_ENQUEUE_ONE_TEST_DATABASE_URL"

p3-o3-enqueue-batch-acceptance:
	@test -n "$${P3O3_ENQUEUE_BATCH_TEST_DATABASE_URL:-}" || { echo "P3O3_ENQUEUE_BATCH_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./acceptance/outbound -args -database-url "$$P3O3_ENQUEUE_BATCH_TEST_DATABASE_URL"

p3-o4-sender-acceptance:
	@test -n "$${P3O4_SENDER_TEST_DATABASE_URL:-}" || { echo "P3O4_SENDER_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=60s -run '^(TestOutboundStorageCatalogWaterlineAndIdentity|TestSender)' ./acceptance/outbound -args -database-url "$$P3O4_SENDER_TEST_DATABASE_URL"

p3-o5-status-acceptance:
	@test -n "$${P3O5_STATUS_TEST_DATABASE_URL:-}" || { echo "P3O5_STATUS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s -run '^(TestOutboundStorageCatalogWaterlineAndIdentity|TestSender)' ./acceptance/outbound -args -database-url "$$P3O5_STATUS_TEST_DATABASE_URL"

p3-o6a-retry-acceptance:
	@test -n "$${P3O6A_RETRY_TEST_DATABASE_URL:-}" || { echo "P3O6A_RETRY_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/outbound/o6a_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s ./internal/outbound/app ./internal/outbound/worker
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s -run '^(TestOutboundStorageCatalogWaterlineAndIdentity|TestSender)' ./acceptance/outbound -args -database-url "$$P3O6A_RETRY_TEST_DATABASE_URL"
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=45s -run '^TestOutboundOutcomeUnknownIsNotRetriedByRealRiver$$' ./acceptance/outbound -args -database-url "$$P3O6A_RETRY_TEST_DATABASE_URL" -o6a-real-river

p3-o6b1-cancel-acceptance:
	@test -n "$${P3O6B1_CANCEL_TEST_DATABASE_URL:-}" || { echo "P3O6B1_CANCEL_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/outbound/o6b1_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s -run '^(TestCancel|TestOutboundStorageCatalogWaterlineAndIdentity)' ./acceptance/outbound -args -database-url "$$P3O6B1_CANCEL_TEST_DATABASE_URL"

p3-o6b2-manual-retry-acceptance:
	@test -n "$${P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL:-}" || { echo "P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/outbound/o6b2_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s -run '^(TestManualRetry|TestOutboundStorageCatalogWaterlineAndIdentity)' ./acceptance/outbound -args -database-url "$$P3O6B2_MANUAL_RETRY_TEST_DATABASE_URL"

p3-o7-legacy-api-acceptance:
	@test -n "$${P3O7_LEGACY_API_TEST_DATABASE_URL:-}" || { echo "P3O7_LEGACY_API_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s -run '^TestO7' ./acceptance/outbound -args -database-url "$$P3O7_LEGACY_API_TEST_DATABASE_URL"

p4-outbound-campaign-handoff-acceptance:
	@test -n "$${P4OUTBOUNDCAMPAIGNHANDOFF_TEST_DATABASE_URL:-}" || { echo "P4OUTBOUNDCAMPAIGNHANDOFF_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/outbound/campaign_handoff_migration_compatibility.sh

p4-outbound-campaign-dispatch-acceptance:
	@test -n "$${P4OUTBOUNDCAMPAIGNDISPATCH_TEST_DATABASE_URL:-}" || { echo "P4OUTBOUNDCAMPAIGNDISPATCH_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/outbound/campaign_dispatch_pg16.sh

p4-w0-d01-automation-acceptance:
	@test -n "$${P4W0D01_AUTOMATION_TEST_DATABASE_URL:-}" || { echo "P4W0D01_AUTOMATION_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/automation/d01_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/automation/store ./internal/events/dispatcher
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./acceptance/automation -args -database-url "$$P4W0D01_AUTOMATION_TEST_DATABASE_URL"

p4-w0-l01-stats-acceptance:
	@test -n "$${P4W0L01_STATS_TEST_DATABASE_URL:-}" || { echo "P4W0L01_STATS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/stats/l01_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s ./internal/stats/store ./internal/events/dispatcher
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=300s ./acceptance/stats -args -database-url "$$P4W0L01_STATS_TEST_DATABASE_URL"

p4-a01-auth-acceptance:
	@test -n "$${P4A01_AUTH_TEST_DATABASE_URL:-}" || { echo "P4A01_AUTH_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/auth/a01_migration_compatibility.sh
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$P4A01_AUTH_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s -run '^TestA01' ./cmd/aicrm -args -p4-a01-database-url "$$P4A01_AUTH_TEST_DATABASE_URL"

p4-admin-shell-ab-acceptance:
	@test -n "$${P4ADMINSHELL_TEST_DATABASE_URL:-}" || { echo "P4ADMINSHELL_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$P4ADMINSHELL_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s -run '^TestA01HumanOAuthSessionRBACCSRFFullChainOnPostgreSQL$$' ./cmd/aicrm -args -p4-a01-database-url "$$P4ADMINSHELL_TEST_DATABASE_URL"

p4-execution-runtime-ab-acceptance:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/adminops/app ./internal/auth/app ./internal/auth/port

p4-internal-events-0367-0368-acceptance:
	@test -n "$${P4INTERNAL_EVENTS_TEST_DATABASE_URL:-}" || { echo "P4INTERNAL_EVENTS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s ./acceptance/events -args -database-url "$$P4INTERNAL_EVENTS_TEST_DATABASE_URL"

p4-ee01-internal-event-safe-export-acceptance:
	@test -n "$${P4EE01_TEST_DATABASE_URL:-}" || { echo "P4EE01_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/events/internal_event_safe_export_pg16.sh

p4-rp01-release-plane-acceptance:
	@test -n "$${P4RP01_TEST_DATABASE_URL:-}" || { echo "P4RP01_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/release/release_plane_pg16.sh

p4-external-effects-runtime-acceptance:
	@test -n "$${P4EER_TEST_DATABASE_URL:-}" || { echo "P4EER_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/externaleffects/external_effects_pg16.sh

p4-data-migration-harness-acceptance:
	@test -n "$${P4DMH_TEST_DATABASE_URL:-}" || { echo "P4DMH_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/datamigration/run_pg16.sh

p4-pe01-wechat-pay-settlement-acceptance:
	@test -n "$${P4PE01_TEST_DATABASE_URL:-}" || { echo "P4PE01_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/order/pe01_migration_compatibility.sh

p4-automation-rules-runtime-acceptance:
	@test -n "$${P4AUTOMATIONRULES_TEST_DATABASE_URL:-}" || { echo "P4AUTOMATIONRULES_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/automation/a01_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/automation/... ./cmd/aicrm

p4-si00b-auth-acceptance:
	@test -n "$${P4SI00B_AUTH_TEST_DATABASE_URL:-}" || { echo "P4SI00B_AUTH_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/auth/si00b_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s ./internal/auth/app ./internal/auth/store
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ACCEPTANCE_FIXTURES_TEST_DATABASE_URL="$$P4SI00B_AUTH_TEST_DATABASE_URL" $(GO) test -race -count=1 -timeout=90s ./acceptance/p2s09 ./acceptance/p2s16
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s -run '^(TestA01|TestHumanOAuth|TestHumanAuth|TestFinalRouter)' ./cmd/aicrm -args -p4-a01-database-url "$$P4SI00B_AUTH_TEST_DATABASE_URL"

p4-i01a-product-acceptance:
	@test -n "$${P4I01A_PRODUCT_TEST_DATABASE_URL:-}" || { echo "P4I01A_PRODUCT_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/product/i01a_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/product/...

p4-i01b-product-entitlement-acceptance:
	@test -n "$${P4I01B_PRODUCT_TEST_DATABASE_URL:-}" || { echo "P4I01B_PRODUCT_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/product/i01b_fresh_database_compatibility.sh

p4-h01a1-media-acceptance:
	@test -n "$${P4H01A1_MEDIA_TEST_DATABASE_URL:-}" || { echo "P4H01A1_MEDIA_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/media/h01a1_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/media/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-h03-media-acceptance:
	@test -n "$${P4H03_MEDIA_TEST_DATABASE_URL:-}" || { echo "P4H03_MEDIA_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/media/h03_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/media/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-image-delete-0362-acceptance:
	@test -n "$${P4IMAGEDELETE_TEST_DATABASE_URL:-}" || { echo "P4IMAGEDELETE_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$${P4IMAGEDELETE_TEST_DATABASE_URL}" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s -run '^TestImageDelete0362' ./acceptance/media -args -database-url "$${P4IMAGEDELETE_TEST_DATABASE_URL}"

p4-miniprogram-library-ab-acceptance:
	@test -n "$${P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL:-}" || { echo "P4MINIPROGRAMLIBRARY_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/media/miniprogram_migration_compatibility.sh

p4-hxc-sender-read-acceptance:
	@test -n "$${P4HXC_SENDER_TEST_DATABASE_URL:-}" || { echo "P4HXC_SENDER_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/hxc/sender_read_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/hxc/... ./internal/contact/store

p4-delivery-lineage-0308-acceptance:
	@test -n "$${P4DELIVERYLINEAGE_TEST_DATABASE_URL:-}" || { echo "P4DELIVERYLINEAGE_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$P4DELIVERYLINEAGE_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/outbound/app ./internal/outbound/store ./internal/events/app ./internal/events/store
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./acceptance/deliverylineage -args -database-url "$$P4DELIVERYLINEAGE_TEST_DATABASE_URL"

p4-customer-profile-tags-0301-acceptance:
	@test -n "$${P4CUSTOMERPROFILETAGS_TEST_DATABASE_URL:-}" || { echo "P4CUSTOMERPROFILETAGS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$P4CUSTOMERPROFILETAGS_TEST_DATABASE_URL" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/contact/app ./internal/contact/store
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s -run '^TestCustomerProfileTags0301' ./acceptance/contact -args -database-url "$$P4CUSTOMERPROFILETAGS_TEST_DATABASE_URL"

p4-f01a-survey-acceptance:
	@test -n "$${P4F01A_SURVEY_TEST_DATABASE_URL:-}" || { echo "P4F01A_SURVEY_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/survey/f01a_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/survey/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-group-ops-acceptance:
	@test -n "$${P4GROUP_OPS_TEST_DATABASE_URL:-}" || { echo "P4GROUP_OPS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" /usr/bin/env bash acceptance/groupops/p4_group_ops_migration_compatibility.sh

p4-f01ab-survey-acceptance:
	@test -n "$${P4F01AB_SURVEY_TEST_DATABASE_URL:-}" || { echo "P4F01AB_SURVEY_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/survey/f01ab_migration_compatibility.sh
	@P4SURVEY_PUBLIC_TEST_DATABASE_URL="$$P4F01AB_SURVEY_TEST_DATABASE_URL" GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/survey/public_anonymous_migration_compatibility.sh
	@AICRM_SURVEY_TEST_DATABASE_URL="$$P4F01AB_SURVEY_TEST_DATABASE_URL" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s -run '^TestPublicRepositoryPostgreSQLRoundTrip$$' ./internal/survey/store
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/survey/... ./internal/events/store ./internal/platform/http ./internal/auth/... ./acceptance/survey

p4-c01-channel-acceptance:
	@test -n "$${P4C01_CHANNEL_TEST_DATABASE_URL:-}" || { echo "P4C01_CHANNEL_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/contact/c01_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/contact/app ./internal/contact/store ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-channel-entrants-acceptance:
	@test -n "$${P4CHANNELENTRANTS_TEST_DATABASE_URL:-}" || { echo "P4CHANNELENTRANTS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$$P4CHANNELENTRANTS_TEST_DATABASE_URL" up
	@P4CHANNELENTRANTS_TEST_DATABASE_URL="$$P4CHANNELENTRANTS_TEST_DATABASE_URL" /usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s -run '^TestChannelEntrantsPG16Integration$$' ./internal/contact/acceptance

p4-contact-policy-acceptance:
	@test -n "$${P4CONTACTPOLICY_TEST_DATABASE_URL:-}" || { echo "P4CONTACTPOLICY_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/contact/contact_policy_migration_compatibility.sh

p4-b02ab-tag-acceptance:
	@test -n "$${P4B02AB_TAG_TEST_DATABASE_URL:-}" || { echo "P4B02AB_TAG_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/contact/b02ab_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=240s -run '^TestB02AB' ./acceptance/contact -args -database-url "$${P4B02AB_TAG_TEST_DATABASE_URL}"

p4-j01-coupon-acceptance:
	@test -n "$${P4J01_COUPON_TEST_DATABASE_URL:-}" || { echo "P4J01_COUPON_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/coupon/j01_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/coupon/... ./internal/product/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-coupon-ab-acceptance:
	@test -n "$${P4COUPONAB_TEST_DATABASE_URL:-}" || { echo "P4COUPONAB_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/coupon/ab_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/coupon/... ./internal/product/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-i03-order-acceptance:
	@test -n "$${P4I03_ORDER_TEST_DATABASE_URL:-}" || { echo "P4I03_ORDER_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/order/i03_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=120s ./internal/order/... ./internal/contact/store ./internal/product/store ./internal/auth/...

p4-order-ab-acceptance:
	@test -n "$${P4ORDERAB_TEST_DATABASE_URL:-}" || { echo "P4ORDERAB_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/order/ab_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/order/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-message-archive-ab-acceptance:
	@test -n "$${P4MESSAGEARCHIVE_TEST_DATABASE_URL:-}" || { echo "P4MESSAGEARCHIVE_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" /usr/bin/env bash acceptance/wecom/message_archive_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/wecom/app ./internal/wecom/store ./internal/identity/app ./internal/identity/store ./internal/events/store ./internal/platform/store ./internal/auth/...

p4-b01-wecom-inbound-acceptance:
	@test -n "$${P4B01_WECOM_INBOUND_TEST_DATABASE_URL:-}" || { echo "P4B01_WECOM_INBOUND_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" /usr/bin/env bash acceptance/wecom/b01_inbound_pg16.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/wecom/app ./internal/wecom/store ./internal/wecom/worker ./internal/identity/http

p4-operation-cycle-ab-acceptance:
	@test -n "$${P4OPERATIONCYCLE_TEST_DATABASE_URL:-}" || { echo "P4OPERATIONCYCLE_TEST_DATABASE_URL is required" >&2; exit 2; }
	@$(GO) tool -modfile=$(TOOLS_MOD) goose -dir migrations postgres "$${P4OPERATIONCYCLE_TEST_DATABASE_URL}" up
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/operationcycle/... ./internal/events/store ./internal/auth/...
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./acceptance/operationcycle -args -database-url "$${P4OPERATIONCYCLE_TEST_DATABASE_URL}"

p4-automation-agents-ab-acceptance:
	@test -n "$${P4AUTOMATIONAGENTSAB_TEST_DATABASE_URL:-}" || { echo "P4AUTOMATIONAGENTSAB_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/automation/agents_ab_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/automation/... ./internal/events/store ./internal/platform/http ./internal/auth/...

p4-adminops-jobs-ab-acceptance:
	@test -n "$${P4ADMINOPS_TEST_DATABASE_URL:-}" || { echo "P4ADMINOPS_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/adminops/control_plane_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/adminops/... ./internal/platform/store ./internal/platform/http ./internal/auth/...

p4-push-center-0421-0422-acceptance:
	@test -n "$${P4PUSHCENTER_TEST_DATABASE_URL:-}" || { echo "P4PUSHCENTER_TEST_DATABASE_URL is required" >&2; exit 2; }
	@GO="$(GO)" TOOLS_MOD="$(TOOLS_MOD)" acceptance/pushcenter/sections_stats_migration_compatibility.sh
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s ./internal/pushcenter/... ./internal/platform/store ./internal/platform/http ./internal/auth/...

p3-c02a-acceptance:
	@test -n "$${ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:-}" || { echo "ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=45s ./acceptance/p3c02a

p3-c02b-acceptance:
	@test -n "$${ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:-}" || { echo "ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=45s ./acceptance/p3c02b

p3-c02d-acceptance:
	@test -n "$${ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:-}" || { echo "ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s ./acceptance/p3c02d

p3-c02e-acceptance:
	@test -n "$${ACCEPTANCE_FIXTURES_TEST_DATABASE_URL:-}" || { echo "ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s ./acceptance/p3c02e

p3-c03-migration-acceptance:
	@test -n "$${P3C03_TEST_DATABASE_URL:-}" || { echo "P3C03_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=45s ./acceptance/contact -args -database-url "$$P3C03_TEST_DATABASE_URL"

p3-c07a-acceptance:
	@test -n "$${P3C07A_TEST_DATABASE_URL:-}" || { echo "P3C07A_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=90s ./acceptance/contact -args -database-url "$$P3C07A_TEST_DATABASE_URL"

p3-c07b2-acceptance:
	@test -n "$${P3C07B2_TEST_DATABASE_URL:-}" || { echo "P3C07B2_TEST_DATABASE_URL is required" >&2; exit 2; }
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 -timeout=180s -run '^TestLineageTimelineGenericPlanUsesExistingIndexes$$' ./acceptance/contact -args -database-url "$$P3C07B2_TEST_DATABASE_URL"

p3-c06a1-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 ./cmd/aicrm-contact-perf-data
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) vet ./cmd/aicrm-contact-perf-data

p3-c06a2-contract:
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -count=1 ./cmd/aicrm-contact-perf
	@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) vet ./cmd/aicrm-contact-perf

ci-go: version-check generate-check gitless-generate-test mod-check migration-validate migration-guard-negative fmt-check vet test build vuln p0-s01-acceptance p0-s02-acceptance p0-s03-acceptance p0-s04-acceptance p2-s04-acceptance p2-s05-acceptance p2-s07-acceptance p2-s08-acceptance p2-s09-acceptance p2-s10-acceptance p2-s11-acceptance p2-s14-acceptance p2-s15-acceptance p2-s16-acceptance p2-s18-acceptance p3-c00-acceptance p3-c01a-contract p3-c06a1-contract p3-c06a2-contract g2-runtime-image-acceptance g2-release-archive-contract g2-web-edge-contract arch-import-lint arch-import-lint-test ownership-lint ownership-lint-test acceptance-fixtures source-policy-lint source-policy-lint-test slice-input-contract-test snapshot-gate-test legacy-route-export-test feature-matrix-contract migration-mapping-contract p1-reconciliation-contract openapi-p1-contract query-plan-gate-test replacement-baseline-contract
