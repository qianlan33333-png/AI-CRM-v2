#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
  echo "repo-contract: $*" >&2
  exit 1
}

git diff --quiet -- . ||
  fail "working tree differs from the staged index; contract would be ambiguous"

required=(
  README.md
  AGENTS.md
  CLAUDE.md
  CONTRIBUTING.md
  SECURITY.md
  NOTICE
  .tool-versions
  .gitleaks.toml
  package.json
  package-lock.json
  Makefile
  go.mod
  go.sum
  tools/go.mod
  tools/go.sum
  .github/CODEOWNERS
  .github/pull_request_template.md
  .github/workflows/application-go.yml
  .github/workflows/repo-contract.yml
  .github/workflows/secret-scan.yml
  api/openapi.yaml
  api/oapi-codegen.yaml
  api/oapi-codegen-p1-candidate.yaml
  sqlc.yaml
  migrations/00001_bootstrap.sql
  internal/platform/http/contract.go
  internal/platform/runtime/contract.go
  internal/platform/store/contract.go
  internal/platform/river/contract.go
  web/index.html
  web/src/main.tsx
  web/src/main.test.tsx
  web/src/api/generated/health.ts
  acceptance/p0s01/runtime_contract_test.go
  acceptance/p0s01/process_blackbox.sh
  acceptance/p0s01/static_contract.sh
  acceptance/p0s02/health_contract_test.go
  acceptance/p0s02/static_contract.sh
  acceptance/p0s02/test_static_contract.sh
  acceptance/p0s03/query_contract_test.go
  acceptance/p0s03/source_contract.go
  acceptance/p0s03/static_contract.sh
  acceptance/p0s03/test_contract.sh
  acceptance/p0s04/contract_test.go
  acceptance/p0s04/source_contract.go
  acceptance/p0s04/test_source_contract.sh
  acceptance/p0s04/static_contract.sh
  acceptance/p0s04/test_static_contract.sh
  acceptance/p0s10/test_snapshot_gate.sh
  acceptance/snapshots/catalog.v1.json
  tools/snapshot-gate/main.go
  tools/snapshot-gate/main_test.go
  tools/legacy-route-export/main.go
  tools/legacy-route-export/main_test.go
  docs/execution/slices/P1-S01.md
  docs/evidence/p1/legacy-routes-6cb989c.json
  docs/feature-matrix.csv
  docs/evidence/p1/feature-matrix-id-anchor.v1
  docs/execution/slices/P1-S08.md
  scripts/check_feature_matrix_contract.sh
  docs/migration-mapping.md
  docs/migration-mapping.jsonl
  docs/evidence/p1/migration-lifecycle-index-6cb989c.json
  docs/execution/slices/P1-S09.md
  docs/execution/slices/P1-C02.md
  tools/migration-mapping/main.go
  tools/migration-mapping/main_test.go
  docs/api-mapping.jsonl
  docs/evidence/p1/route-triage.csv
  docs/evidence/p1/g1-decisions.md
  docs/execution/slices/G1-D01.md
  tools/p1-reconciliation/main.go
  tools/p1-reconciliation/main_test.go
  docs/execution/slices/P1-C03.md
  tools/openapi-contract/main.go
  tools/openapi-contract/main_test.go
  acceptance/p1s11/contracts_test.go
  acceptance/p1s11/doc.go
  docs/execution/slices/P1-S11.md
  internal/api/generated/server.gen.go
  internal/api/candidate/generated/server.gen.go
  internal/auth/port/port.go
  internal/contact/port/port.go
  internal/identity/port/port.go
  internal/platform/port/uow.go
  tools/query-plan-gate/main.go
  tools/query-plan-gate/main_test.go
  scripts/build_slice_bundle.sh
  scripts/check_arch_imports.go
  scripts/ownership/main.go scripts/test_ownership.sh
  scripts/sourcepolicy/main.go scripts/test_source_policy.sh
  scripts/check_slice_inputs.sh scripts/test_slice_inputs.sh
  scripts/check_generated_sources.sh
  scripts/check_repo_contract.sh
  scripts/generated-sources.sha256
  scripts/scan_sensitive_paths.sh
  scripts/test_gitleaks_config.sh
  scripts/test_build_slice_bundle.sh
  scripts/test_gitless_generated_check.sh
  scripts/test_orval_generated_check.sh
  scripts/test_arch_imports.sh
  scripts/test_query_plan_gate.sh
  scripts/test_repo_contract.sh
  docs/architecture/canonical.md
  docs/adr/ADR-001.md
  docs/adr/ADR-010.md
  docs/adr/ADR-011.md
  docs/architecture/port-contracts.md
  docs/architecture/table-ownership.yml
  docs/governance/limitations.md
  docs/execution/slice-card-template.md
  docs/execution/slice-ledger.yml
  docs/execution/slices/P0-S02.md
  docs/execution/slices/P0-S03.md
  docs/execution/slices/P0-S04.md
  docs/execution/slices/M0-1.md
  docs/execution/slices/M0-2.md
  docs/execution/slices/M0-3.md
  docs/execution/slices/M0-5.md
  docs/execution/slices/M0-6.md
  docs/execution/slices/M0-7.md
  docs/spec/AI-CRM-v2-执行方案.md
  docs/spec/AI-CRM-v2-执行方案-v2-至P3.md
  docs/spec/AI-CRM-v2-重构详细设计.md
  docs/spec/SHA256SUMS
)

for path in "${required[@]}"; do
  [[ -f "$path" ]] || fail "missing required file: $path"
  [[ "$(git ls-files -s -- "$path" | wc -l | tr -d ' ')" = "1" ]] ||
    fail "required file is missing or has an ambiguous index entry: $path"
  index_mode="$(git ls-files -s -- "$path" | awk '{print $1}')"
  case "$index_mode" in
    100644|100755) ;;
    *) fail "required path must be a regular tracked file: $path (mode $index_mode)" ;;
  esac
done

verify_index_mode() {
  local path="$1"
  local expected="$2"
  local actual
  actual="$(git ls-files -s -- "$path" | awk 'NR == 1 { print $1 }')"
  [[ "$actual" = "$expected" ]] ||
    fail "pinned repository mode drifted: $path ($actual)"
}

while IFS=' ' read -r expected path; do
  verify_index_mode "$path" "$expected"
done <<'EOF'
100644 Makefile
100644 go.mod
100644 go.sum
100644 tools/go.mod
100644 tools/go.sum
100644 package.json
100644 package-lock.json
100644 .gitleaks.toml
100644 web/index.html
100644 web/src/main.tsx
100644 web/src/main.test.tsx
100644 web/src/api/generated/health.ts
100644 .github/workflows/application-go.yml
100755 scripts/check_repo_contract.sh
100644 scripts/check_arch_imports.go
100755 scripts/test_arch_imports.sh
100644 scripts/ownership/main.go
100755 scripts/test_ownership.sh
100644 scripts/sourcepolicy/main.go
100755 scripts/test_source_policy.sh
100755 scripts/test_gitless_generated_check.sh
100755 scripts/check_slice_inputs.sh
100755 scripts/test_slice_inputs.sh
100644 tools/snapshot-gate/main.go
100644 tools/snapshot-gate/main_test.go
100644 acceptance/snapshots/catalog.v1.json
100644 tools/legacy-route-export/main.go
100644 tools/legacy-route-export/main_test.go
100644 docs/execution/slices/P1-S01.md
100644 docs/evidence/p1/legacy-routes-6cb989c.json
100644 docs/feature-matrix.csv
100644 docs/evidence/p1/feature-matrix-id-anchor.v1
100644 docs/execution/slices/P1-S08.md
100755 scripts/check_feature_matrix_contract.sh
100644 docs/migration-mapping.md
100644 docs/migration-mapping.jsonl
100644 docs/evidence/p1/migration-lifecycle-index-6cb989c.json
100644 docs/execution/slices/P1-S09.md
100644 docs/execution/slices/P1-C02.md
100644 tools/migration-mapping/main.go
100644 tools/migration-mapping/main_test.go
100644 docs/api-mapping.jsonl
100644 docs/evidence/p1/route-triage.csv
100644 docs/evidence/p1/g1-decisions.md
100644 docs/execution/slices/G1-D01.md
100644 tools/p1-reconciliation/main.go
100644 tools/p1-reconciliation/main_test.go
100644 docs/execution/slices/P1-C03.md
100644 api/openapi.yaml
100644 api/oapi-codegen.yaml
100644 api/oapi-codegen-p1-candidate.yaml
100644 internal/api/generated/server.gen.go
100644 internal/api/candidate/generated/server.gen.go
100644 tools/openapi-contract/main.go
100644 tools/openapi-contract/main_test.go
100644 acceptance/p1s11/contracts_test.go
100644 acceptance/p1s11/doc.go
100644 docs/execution/slices/P1-S11.md
100644 internal/auth/port/port.go
100644 internal/contact/port/port.go
100644 internal/identity/port/port.go
100644 internal/platform/port/uow.go
100644 tools/query-plan-gate/main.go
100644 tools/query-plan-gate/main_test.go
100755 acceptance/p0s10/test_snapshot_gate.sh
100644 docs/architecture/table-ownership.yml
100755 scripts/test_orval_generated_check.sh
100755 scripts/test_gitleaks_config.sh
100755 scripts/test_repo_contract.sh
100755 scripts/test_query_plan_gate.sh
100755 acceptance/p0s02/static_contract.sh
100755 acceptance/p0s02/test_static_contract.sh
100644 internal/platform/river/contract.go
100644 acceptance/p0s04/contract_test.go
100644 acceptance/p0s04/source_contract.go
100755 acceptance/p0s04/test_source_contract.sh
100755 acceptance/p0s04/static_contract.sh
100755 acceptance/p0s04/test_static_contract.sh
100644 docs/execution/slices/P0-S04.md
100644 docs/execution/slices/M0-1.md
100644 docs/execution/slices/M0-2.md
100644 docs/execution/slices/M0-3.md
100644 docs/execution/slices/M0-5.md
100644 docs/execution/slices/M0-6.md
100644 docs/execution/slices/M0-7.md
100644 docs/adr/ADR-001.md
100644 docs/adr/ADR-010.md
100644 docs/adr/ADR-011.md
100644 docs/spec/AI-CRM-v2-执行方案.md
100644 docs/spec/AI-CRM-v2-执行方案-v2-至P3.md
100644 docs/spec/AI-CRM-v2-重构详细设计.md
100644 docs/spec/SHA256SUMS
EOF

verify_index_sha256() {
  local path="$1"
  local expected="$2"
  local actual
  actual="$(git show ":$path" | sha256sum | awk '{print $1}')"
  [[ "$actual" = "$expected" ]] ||
    fail "pinned repository content drifted: $path ($actual)"
}

verify_index_sha256 Makefile \
  84ea2eae13dc537a78fbc480bb1ea3a45811dbea104372e4888e5db5e1d5da12
verify_index_sha256 .github/CODEOWNERS \
  bb2c40eaad8b8b3dd83cd2d81f58360717ab6dbaeb773afe6d65b7ae18e4f5cb
verify_index_sha256 go.mod \
  d670d0dbe18623fdaf0792f6bcae0bca07cba70e66f555ef35164a79d6b2219c
verify_index_sha256 go.sum \
  8fa975442d8a0f2e15719d7b7558f206656592735c983a0ccd51eaeb588dbdc6
verify_index_sha256 package.json \
  0eba96dc7c5cb99afa7334da44ebff47e004d10465cb5f9b2ce31f1993bb3d47
verify_index_sha256 package-lock.json \
  64f32f2bc22dbde74f3e0e82fbfa91c1160621fc1a771832a0a0b06fb11e2892
verify_index_sha256 web/src/api/generated/health.ts \
  5b3c0fd655dfb30964998a473cb1d9569983b44bdebfc01e3472c47b77ede60b
verify_index_sha256 .github/workflows/application-go.yml \
  eb5a445954504d93ddf0b2b3e6bcbe90dd1344ae3f9a6b63312ee82c107d09bf
verify_index_sha256 .github/workflows/repo-contract.yml \
  32ae51c23bffdc930bbf2cbec4098089d4eb46c879fb79b141665523f93547e5
verify_index_sha256 .github/workflows/secret-scan.yml \
  e3077f509e0cfe5a9b70c4064cc666f53258c62cda590f191ea401d1734d02fe
verify_index_sha256 .gitleaks.toml \
  b220c3b1e00671ed5d45f796b341a586a533659b7eecadf4906516769414ff74
verify_index_sha256 scripts/test_gitleaks_config.sh \
  2c4da4f3e1fc926910a516593513c8f1e2f51445879bd7a9a5574ce47396dcf3
verify_index_sha256 docs/execution/slices/M0-7.md \
  0b9cd7cbd3ae679b57b54361d8d7d9f0ff34e1568f55bf118505a048c9e229a4
verify_index_sha256 scripts/check_generated_sources.sh \
  e6dd0def9500ac96aaff3bb4c1737d29212612a6fa36db9078229183994035c7
verify_index_sha256 scripts/test_gitless_generated_check.sh \
  88ca0a11cd975e488dcc408b1c50cf6b575d367926a7c633c94a5e42f634612e
verify_index_sha256 scripts/generated-sources.sha256 \
  33eaa83c609577fde602e42c59746443ac67e1d7191a2841b5d33cadd815c58a
verify_index_sha256 scripts/test_orval_generated_check.sh \
  ae16d4f7696baccf354b6debc0645afeac32e8475491d4d4b4cfe281c201e587
verify_index_sha256 scripts/check_arch_imports.go \
  7467b6857b05e89793bb2001745650bc04750bd5a59072e3c7a2a7be0f011b18
verify_index_sha256 scripts/test_arch_imports.sh \
  68cdf909235c8961a91ffe6560461fb1e174bc67fdb3b3eb24b22bf43c25d0b7
verify_index_sha256 scripts/ownership/main.go \
  e1dfe40e7ccc9ec40cc7a6cb2c10cb8473e373c2751e1a88f61480b539f64241
verify_index_sha256 scripts/test_ownership.sh \
  d239565f77afe42155e4a09657fdff0abd6c59823aa60f1ec4ff6c565b9087df
verify_index_sha256 scripts/sourcepolicy/main.go \
  350924119f5f190d1e399d2e84f8f163d5c5ea7b0dbfc2a0652ba9b7a3c077c0
verify_index_sha256 scripts/test_source_policy.sh \
  ea5b70241c85adeed28bd6b4f0ad1f887630615b882aac209af4e42e15cc184e
verify_index_sha256 AGENTS.md \
  665b9c6e15c4e95feb6fba1c9a7702a094cf84c46e7d1f2d685de7e1881cc8a0
verify_index_sha256 scripts/check_slice_inputs.sh \
  b7b1711da73974b0a89c79bab020e519095fc8aaf36f737b036027ec3a08cb25
verify_index_sha256 scripts/test_slice_inputs.sh \
  32ec32f2c3f0c7ec7c29aabfada4bc4c1f29dc39a37fc0aacfb51e30d304d8a7
verify_index_sha256 docs/execution/slices/M0-1.md \
  1bb201f3550ef638ea85f8f7f5de585b57825177c400a7f5cab3094ef68f6043
verify_index_sha256 docs/execution/slices/M0-2.md \
  97d9acda27150c905af7bc52eeca623034ae1c529d89aac56c253f18d426df59
verify_index_sha256 tools/go.mod \
  6a64133379331817837ab27823bcf7c672d3a300b2cf9d3c2c79c56f7f90e7ef
verify_index_sha256 tools/go.sum \
  2515f9dd3dbc17c77f98550be06a8cdf072538e6d8eb296077b6ad91120d2753
verify_index_sha256 tools/query-plan-gate/main.go \
  ef737d9397fde907f0aadc404733413c8a867911463c31a3218d45c44a9dcaf7
verify_index_sha256 tools/query-plan-gate/main_test.go \
  379fdaecbfac5c3f7107edace3edc7aa9381ea4fe12623fafb94a5308251518a
verify_index_sha256 scripts/test_query_plan_gate.sh \
  61a1ce22acc6358b697c50e191c02a6d2e8a0fe20b9d00c2070cececdc8bb497
verify_index_sha256 docs/execution/slices/M0-3.md \
  c14caaed56bc85a386ece43639053b10556d3d4eae25816fbefe334430d6ba0f
verify_index_sha256 docs/spec/AI-CRM-v2-执行方案.md \
  210f6d3c9d0434cba6426ab71fc1cc64bc3a6d3a1a184e55af5f1273c21a8099
verify_index_sha256 docs/spec/AI-CRM-v2-执行方案-v2-至P3.md \
  816f04447e1af046d4fe6ef24b436aa062b535decc32d6a463055121dd3f6a46
verify_index_sha256 docs/spec/AI-CRM-v2-重构详细设计.md \
  a0917b9d2d119a68ba9c32e2d458c7b9a3775f846037748947715fdcfee77ee6
verify_index_sha256 docs/spec/SHA256SUMS \
  2b6a02de88ef52160af312a2d5b52677364ccfa64125eb67cf5bb1e155caec82
verify_index_sha256 tools/snapshot-gate/main.go \
  425cb0ea7702d9aeb817687487f97db27b7e3c03b8a5a95df722aedd8390992c
verify_index_sha256 tools/snapshot-gate/main_test.go \
  77771f548652fc2ffe556b8f8fd31a8f394cc0e90d3e57cb7014711894a29d9b
verify_index_sha256 acceptance/snapshots/catalog.v1.json \
  4dbb86ff637cd42a75d6a1a6ca952930af1e6b145e7c2367c6a5bff6981b1d9d
verify_index_sha256 tools/legacy-route-export/main.go \
  a7734a3207ec6e7a58c83a397dc6300fe262e5430b03c12b488262dbc126954e
verify_index_sha256 tools/legacy-route-export/main_test.go \
  1a7073ac6488e5d55873d9d2711b1096e2b08402272cd151babd9efa2c81221b
verify_index_sha256 docs/execution/slices/P1-S01.md \
  39571f878f83ed4b1537342dc49efe54bb7ea5d7720950002f12e45581a67726
verify_index_sha256 docs/evidence/p1/legacy-routes-6cb989c.json \
  fb6aa066c985af4d0c5e3abe8a33c88b5c3a6a56dda9ee7fd6e51aca8cb5f231
verify_index_sha256 docs/evidence/p1/feature-matrix-id-anchor.v1 \
  1ab849cb10518e55f5c95716c1fab6f2c9e47477d17ad7f3f125edcc7e01ad75
verify_index_sha256 docs/execution/slices/P1-S08.md \
  707667f4058d212ed628d8b868e02225c93a3948c9d90a229856162c15b13d99
verify_index_sha256 docs/migration-mapping.jsonl \
  d87e602bff788b097f6b502843b3d815419889e022da21292434670f537da28e
verify_index_sha256 docs/evidence/p1/migration-lifecycle-index-6cb989c.json \
  404083ca07522a993f349b9a53331663375b53b3344743ddcb238966e3ff2540
verify_index_sha256 docs/migration-mapping.md \
  9e3bbf63d9357291c19071b98e79f16ea76002e398e13bedb7b2ac2d89fb1e32
verify_index_sha256 tools/migration-mapping/main.go \
  634cae2168f86dce0f58604ecbf9357dcafe99850c12cef7436234ee52f2804a
verify_index_sha256 tools/migration-mapping/main_test.go \
  762bc2621cbcff28530354ebcc47d102ee579eb1bc364fbec70c8efada929ddb
verify_index_sha256 docs/execution/slices/P1-C02.md \
  c7b8c7236f20279d244286f0f3facaab28ba831a64e7f662755a6b9700abac43
verify_index_sha256 docs/api-mapping.jsonl \
  8e4929f79b7809b4d1a9621ce0e94531b7d46cedcaeb437fc34d71e7f8f1ae6d
verify_index_sha256 docs/evidence/p1/route-triage.csv \
  875596da33d316c31bff9a6103725affa58c44be399ec98239d9e294c34c069b
verify_index_sha256 docs/evidence/p1/g1-decisions.md \
  96686173d77d2a70fd1f4114cb1cd32bbdd4185cda5b2db83c9571825c48e1b3
verify_index_sha256 docs/execution/slices/G1-D01.md \
  1d2f47dd1af0d27533184e772de2a28d9cc4a663b923bf1bd7ddd6121417be8b
verify_index_sha256 tools/p1-reconciliation/main.go \
  47a6c9a6dae5b4a4e082a86ad13251edd8cdc370e396abd42d557bae1d9b78be
verify_index_sha256 tools/p1-reconciliation/main_test.go \
  b7c9c62315059476905bdf8765ccb741066307ffbc030e2a9492ff8284fb435a
verify_index_sha256 docs/execution/slices/P1-C03.md \
  cd9e0441d79b9e1887030087bb4dd800a0a3ca3529275008083d00c577572ffc
verify_index_sha256 api/openapi.yaml \
  646b65f055ce20eba013bd16154fa216b054d36eac85a7ecbbdf9355dad15487
verify_index_sha256 api/oapi-codegen.yaml \
  78abf754fe91788d5cbdab2286ba66dc32d5e13ed1735ffeee9119e473fd4a2b
verify_index_sha256 api/oapi-codegen-p1-candidate.yaml \
  6a4bc4d7afa720c2175b8b59754dd68a1e6321020bd63a33029dc7bbacc65e69
verify_index_sha256 internal/api/generated/server.gen.go \
  a199091028a584df54844b2d761bda8f5010f64e326bae1526c71d9fd15c9c82
verify_index_sha256 internal/api/candidate/generated/server.gen.go \
  d40e331971601433321458e19717ef2aa83b98891dc62cbe20693bda5ef51874
verify_index_sha256 tools/openapi-contract/main.go \
  35bc3bc136964388c71ec90a1706553671b77505a7716ab5bfe8071a06ea772f
verify_index_sha256 tools/openapi-contract/main_test.go \
  defc65938826a478ff4093deb86ca6242a108d9da171afecb50aac9f480ff994
verify_index_sha256 acceptance/p1s11/contracts_test.go \
  148b70a42a8b6a6ec5ec7dbac9ee47e89f0a8cfc0cb07da857d2df4568061cd9
verify_index_sha256 acceptance/p1s11/doc.go \
  8a7f18c253c7b95d9714845c8a98d548c5730bde49de5d8bae156bc3967727d9
verify_index_sha256 docs/execution/slices/P1-S11.md \
  5866fe52a0039f310c10add3d8cfa77eaba9d748dcf518d71df04dac2354a872
verify_index_sha256 internal/auth/port/port.go \
  3bf6bb9affe0c102bd5c64b01d824a75eb35ef3958f99c2787cf30319436dd4f
verify_index_sha256 internal/contact/port/port.go \
  c25b7d5551878d8e8b1a33617f11d8080dbd02aba9c28e254346c752bb0dc0cc
verify_index_sha256 internal/identity/port/port.go \
  321d6518b3e5fec57f3591307334e9fac67c06018bec727790f45e0e55ab5627
verify_index_sha256 internal/platform/port/uow.go \
  9c751db2adab03f18c342fa5ab6487020084f704b0fe96203010e1f9f5c03e2b
verify_index_sha256 scripts/check_feature_matrix_contract.sh \
  d554c955b66a539a6fed395abd4dbd207fc71fce294f2fb1965dc66169b0759b
verify_index_sha256 acceptance/p0s10/test_snapshot_gate.sh \
  f412452642b3b03f9f776ad471996e1b6d2df962c7a8b0016122b6a430f1d91f
verify_index_sha256 docs/adr/ADR-001.md \
  e4da265cf5ffd9962d1f77f2410538e09e8b41d1cb37c1584c3d57265480e28d
verify_index_sha256 docs/adr/ADR-010.md \
  5fdbd62214938e6485322a84858c488a8bac812e00f312318186a5b3ec9b72dc
verify_index_sha256 docs/adr/ADR-011.md \
  3fb1954942b0de9da1989276d535af090c0dcd22841437dbb7d6e49e54b7f92d
verify_index_sha256 docs/execution/slices/M0-5.md \
  c5a4f1991b8f3ecbb1a3a024c6131aea5fc3d6813ddeb34b323faf2948229609
verify_index_sha256 docs/execution/slices/M0-6.md \
  96f5131c60d2eec508557f03ba1322af88c2002a259ec8d455569024d2013125
verify_index_sha256 docs/architecture/canonical.md \
  ac61872a4ad45e368ba8ebf40ec3da2f6e07399c1ad8487ce695f987275861f2
verify_index_sha256 docs/architecture/table-ownership.yml \
  c89e1fec21a83f2a94d2bd98e786905bb75a26fafc2c7a30728ce8b24fe998d8
verify_index_sha256 scripts/test_repo_contract.sh \
  251d853fd4a228511c94bb93b31daf162c1b84bc33f5fd3e2a7964d6b83d9129
verify_index_sha256 acceptance/p0s02/static_contract.sh \
  0102039e07ddb8e55abaa57663ec8885d827fc184aea4042ed5138fc7da50b57
verify_index_sha256 acceptance/p0s02/test_static_contract.sh \
  1d3c89bdb0dffabf298777965896128c2fa4116b134deacf8c1499b18a45c7a2
verify_index_sha256 internal/platform/store/contract.go \
  747683b0f430da2ee29f001abaebe5fe621561aa3dd99b5b9db6b7d871895165
verify_index_sha256 acceptance/p0s03/query_contract_test.go \
  c980ae9264f1eadf69fd98734259dcb92a9bf3f5ddd899518a36ee636a64fd42
verify_index_sha256 acceptance/p0s03/source_contract.go \
  239802f1fea13e0640ca4e3d1eda8f8428f8393f2cd4919deecc0d6ab311cd79
verify_index_sha256 acceptance/p0s03/static_contract.sh \
  666b174b0017e44e774eaea1b784d1e0ba93e308632f42f7ace768917d9a3c84
verify_index_sha256 acceptance/p0s03/test_contract.sh \
  030c0ca1f901e95d802b3dee37ca0b472fceecb5fd147263a59c6bd786b946aa
verify_index_sha256 internal/platform/river/contract.go \
  f03a64b78f9fa0f809b869a7d473f42a9edecc41201805fec461f4ba0f1cb292
verify_index_sha256 acceptance/p0s04/contract_test.go \
  969e834043c841f533c3405da13f2048178cb31982ac0619aec04f77ed600340
verify_index_sha256 acceptance/p0s04/source_contract.go \
  8d02d5d5fdd76a31999ed76a9eb77d8343b09d4a0cd48c3531f893d29331f9d0
verify_index_sha256 acceptance/p0s04/test_source_contract.sh \
  5d92037d3a54cc450b7862d08d60015ac9b1a776d11230d1d373d0169a516697
verify_index_sha256 acceptance/p0s04/static_contract.sh \
  8cf9ee7189be9a859c660301a5a801cef50495cfcf394d1ed6854b4e73a39c5d
verify_index_sha256 acceptance/p0s04/test_static_contract.sh \
  8b53e98baeba0c106a489ad3ea9e3b65d618433a3e740b4109ec032e0dd6b97e
verify_index_sha256 docs/execution/slices/P0-S04.md \
  bb8cc8f7ff0ef4d6e76c8c124bafc661f562a230ec25826b8318a82311281608

makefile="$(git show ':Makefile')"; alternate="$(find . -maxdepth 1 \( -name GNUmakefile -o -name makefile \) -print -quit)"; [[ -z "$alternate" ]] || fail "alternate Make entrypoint is forbidden: ${alternate#./}"
! grep -Eq '^[[:space:]]*(-?include|sinclude)([[:space:]]|$)' <<<"$makefile" && ! awk 'index($0,"$") && substr($0,1,1) != "\t" && $0 !~ /^[[:space:]]*#/ { bad=1 } END { exit bad ? 0 : 1 }' <<<"$makefile" ||
  fail "Makefile must not construct or import rules dynamically"
printf '%s\n' "$makefile" | grep -Eq '^p0-s03-contract:[[:space:]]*$' ||
  fail "Makefile is missing the P0-S03 contract target"
p0_s03_contract_recipe="$(
  awk '
    /^p0-s03-contract:[[:space:]]*$/ { capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
  ' <<<"$makefile"
)"
printf '%s\n' "$p0_s03_contract_recipe" |
  grep -Fqx $'\t@GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s03/test_contract.sh' ||
  fail "P0-S03 contract target must run the gitless contract tests"
printf '%s\n' "$makefile" |
  grep -Eq '^p0-s03-acceptance:[[:space:]]+p0-s03-contract([[:space:]]|$)' ||
  fail "P0-S03 acceptance target must depend on the contract target"
ci_go_target="$(printf '%s\n' "$makefile" | grep -E '^ci-go:[[:space:]]' || true)"
[[ "$ci_go_target" =~ (^|[[:space:]])p0-s03-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S03 acceptance target"

require_make_line() {
  local line="$1"
  local label="$2"
  [[ "$(printf '%s\n' "$makefile" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "$label"
}

require_unique_make_target() {
  local target="$1"
  local count
  count="$(awk -v target="$target" '
    /^[^[:space:]#][^:]*:/ && $0 !~ /:[[:space:]]+override[[:space:]]+(SHELL|[.]SHELLFLAGS|GO)[[:space:]]*:=/ {
      header = $0
      sub(/:.*/, "", header)
      fields = split(header, names, /[[:space:]]+/)
      for (position = 1; position <= fields; position++) {
        if (names[position] == target) count++
      }
    }
    END { print count + 0 }
  ' <<<"$makefile")"
  [[ "$count" = "1" ]] || fail "Makefile target must be unique: $target"
}

make_target_recipe() {
  local header="$1"
  awk -v header="$header" '
    $0 == header { seen++; capture = 1; next }
    capture && /^[^[:space:]]/ { exit }
    capture { print }
    END { exit !(seen == 1) }
  ' <<<"$makefile"
}

for target in vet test build vuln; do
  recipe="$(make_target_recipe "$target:")" ||
    fail "Makefile target must be unique: $target"
  [[ "$(grep -Fc 'GOWORK=off $(GO) list ./...' <<<"$recipe" || true)" = "1" ]] ||
    fail "$target must use Go to discover buildable packages"
  [[ "$(grep -Fc "grep -Ev '(^|/)([.]git|node_modules|vendor)(/|\$\$)'" <<<"$recipe" || true)" = "1" ]] ||
    fail "$target must exclude .git, node_modules, and vendor packages"
  case "$target" in
    vet) expected='$(GO) vet $$packages' ;;
    test) expected='$(GO) test -race $$packages' ;;
    build) expected='$(GO) build $$packages' ;;
    vuln) expected='$(GO) tool -modfile=$(TOOLS_MOD) govulncheck $$packages' ;;
  esac
  grep -Fq "$expected" <<<"$recipe" ||
    fail "$target must consume the filtered Go package set"
done

require_make_line 'generate: generate-openapi generate-sqlc generate-orval' \
  "make generate must include the frozen Orval client"
require_unique_make_target generate-openapi
openapi_generate_recipe="$(make_target_recipe 'generate-openapi:')" ||
  fail "OpenAPI generate target must be unique"
expected_openapi_generate_recipe=$'\t@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \\\n\t\t--config api/oapi-codegen.yaml api/openapi.yaml\n\t@$(GO) tool -modfile=$(TOOLS_MOD) oapi-codegen \\\n\t\t--config api/oapi-codegen-p1-candidate.yaml api/openapi.yaml'
[[ "$openapi_generate_recipe" = "$expected_openapi_generate_recipe" ]] ||
  fail "OpenAPI generation lost the runtime or candidate boundary"
require_unique_make_target generate-orval
require_unique_make_target orval-check
orval_generate_recipe="$(make_target_recipe 'generate-orval:')" ||
  fail "Orval generate target must be unique"
for fragment in \
  'PATH="$(dir $(abspath $(ORVAL))):$$PATH" $(ORVAL)' \
  '--input api/openapi.yaml --output web/src/api/generated/health.ts' \
  '--client fetch --mode single --clean web/src/api/generated --prettier'; do
  grep -Fq -- "$fragment" <<<"$orval_generate_recipe" ||
    fail "Orval generation lost a frozen input, output, client, or clean boundary"
done
orval_check_recipe="$(make_target_recipe 'orval-check:')" ||
  fail "Orval check target must be unique"
[[ "$(grep -Fc '$(MAKE) --no-print-directory generate-orval' <<<"$orval_check_recipe" || true)" = "2" ]] ||
  fail "Orval check must generate twice"
[[ "$(grep -Fc 'git diff --exit-code -- web/src/api/generated' <<<"$orval_check_recipe" || true)" = "2" ]] ||
  fail "Orval check must reject tracked generated drift after both passes"
[[ "$(grep -Fc 'git ls-files --others --exclude-standard -- web/src/api/generated' <<<"$orval_check_recipe" || true)" = "2" ]] ||
  fail "Orval check must reject untracked generated drift after both passes"

package_json="$(git show ':package.json')"
for fragment in \
  '"orval": "7.21.0"' \
  '"prettier": "3.9.6"' \
  '"js-yaml": "4.3.1"' \
  '"lodash": "4.18.1"' \
  '"orval:generate": "make generate-orval"' \
  '"orval:check": "make orval-check"' \
  '"orval:contract": "scripts/test_orval_generated_check.sh"' \
  'npm run orval:check && npm run orval:contract'; do
  [[ "$(grep -Fc "$fragment" <<<"$package_json" || true)" = "1" ]] ||
    fail "package.json lost a frozen P0-S06 Orval version, security override, or gate"
done

require_make_line 'unexport BASH_ENV ENV' \
  "Makefile must unexport BASH_ENV and ENV before starting recipes"
require_make_line '.PHONY: fmt-check vet test build vuln p0-s01-acceptance p0-s02-contract p0-s02-acceptance p0-s03-contract p0-s03-acceptance ci-go' \
  "Makefile must declare the P0-S02 contract and acceptance targets"
require_unique_make_target p0-s02-contract
require_unique_make_target p0-s02-acceptance
require_make_line 'p0-s02-acceptance: p0-s02-contract' \
  "P0-S02 acceptance target must depend on the contract target"
p0_s02_contract_recipe="$(make_target_recipe 'p0-s02-contract:')" ||
  fail "P0-S02 contract target must be unique"
[[ "$p0_s02_contract_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s02/test_static_contract.sh' ]] ||
  fail "P0-S02 contract target must run only the frozen static-contract runner"
[[ "$ci_go_target" =~ (^|[[:space:]])p0-s02-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S02 acceptance target"
require_make_line '.PHONY: p0-s04-contract p0-s04-acceptance p0-s04-integration' \
  "Makefile must declare the P0-S04 targets exactly once"
require_unique_make_target p0-s04-contract
require_unique_make_target p0-s04-acceptance
require_unique_make_target p0-s04-integration
require_make_line 'p0-s04-contract:' \
  "Makefile is missing or overriding the P0-S04 contract target"
require_make_line 'p0-s04-acceptance: p0-s04-contract' \
  "P0-S04 acceptance target must depend only on the contract target"
require_make_line 'p0-s04-integration: p0-s04-contract' \
  "P0-S04 integration target must depend only on the contract target"

p0_s04_contract_recipe="$(make_target_recipe 'p0-s04-contract:')" ||
  fail "P0-S04 contract target must be unique"
[[ "$p0_s04_contract_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_source_contract.sh\n\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/test_static_contract.sh' ]] ||
  fail "P0-S04 contract target must run only the frozen source and static runners"

p0_s04_empty_lines=(
  $'\t@shopt -s nullglob dotglob; \\'
  $'\t\triver_entries=(internal/platform/river/*); \\'
  $'\tif [[ -d internal && ! -L internal && \\'
  $'\t\t-d internal/platform && ! -L internal/platform && \\'
  $'\t\t-d internal/platform/river && ! -L internal/platform/river && \\'
  $'\t\t-f internal/platform/river/contract.go && ! -L internal/platform/river/contract.go && \\'
  $'\t\t"$${#river_entries[@]}" -eq 1 && "$${river_entries[0]}" = "internal/platform/river/contract.go" && \\'
  $'\t\t! -e internal/platform/river/runtime.go && ! -L internal/platform/river/runtime.go && \\'
  $'\t\t! -e internal/platform/river/migrate.go && ! -L internal/platform/river/migrate.go && \\'
  $'\t\t! -e internal/platform/river/runtime_test.go && ! -L internal/platform/river/runtime_test.go ]]; then \\'
)

p0_s04_acceptance_recipe="$(make_target_recipe 'p0-s04-acceptance: p0-s04-contract')" ||
  fail "P0-S04 acceptance target must be unique"
for line in "${p0_s04_empty_lines[@]}" \
  $'\t\techo "P0-S04 acceptance gate: PENDING (implementation not present)"; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \\' \
  $'\t\tcoverage_output="$$(env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -cover -timeout=15s ./internal/platform/river 2>&1)" || { status=$$?; printf \'%s\\n\' "$$coverage_output"; exit "$$status"; }; \\' \
  $'\t\tif ! printf \'%s\\n\' "$$coverage_output" | awk \'$$1 == "ok" && $$2 == "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river" { matches++; value = $$0; if (value !~ /coverage: [0-9]+([.][0-9]+)?% of statements$$/) invalid = 1; else { sub(/^.*coverage: /, "", value); sub(/% of statements$$/, "", value); if (value + 0 <= 0) invalid = 1 } } END { exit !(matches == 1 && !invalid) }\'; then echo "P0-S04 acceptance gate: internal/platform/river must report positive numeric coverage" >&2; exit 1; fi; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=15s -tags=p0s04_acceptance -run \'^(TestPinnedRiverPublicAPISurface|TestRuntimeLifecycleContract|TestRuntimeStartContextIsolated|TestRuntimeCancellationWinsSimultaneousStopped|TestInvalidMigrationDirection)$$\' ./acceptance/p0s04; \\'; do
  printf '%s\n' "$p0_s04_acceptance_recipe" | grep -Fqx "$line" ||
    fail "P0-S04 acceptance target lost a frozen static, coverage, or non-PG call"
done

p0_s04_integration_recipe="$(make_target_recipe 'p0-s04-integration: p0-s04-contract')" ||
  fail "P0-S04 integration target must be unique"
for line in "${p0_s04_empty_lines[@]}" \
  $'\t\techo "P0-S04 integration gate: PENDING (implementation not present)"; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly acceptance/p0s04/static_contract.sh || exit $$?; \\' \
  $'\t\ttest "$${ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST:-}" = "1" || { echo "ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 is required" >&2; exit 2; }; \\' \
  $'\t\tenv -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST=1 $(GO) test -race -timeout=45s -tags=p0s04_acceptance -run \'^TestOfficialMigrationUpDownUp$$\' ./acceptance/p0s04; \\'; do
  printf '%s\n' "$p0_s04_integration_recipe" | grep -Fqx "$line" ||
    fail "P0-S04 integration target lost a frozen static, guard, or PG call"
done

[[ "$(printf '%s\n' "$makefile" | grep -Ec '^ci-go:[[:space:]]' || true)" = "1" && "$ci_go_target" =~ (^|[[:space:]])p0-s04-acceptance($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P0-S04 acceptance target"

require_make_line '.PHONY: arch-import-lint arch-import-lint-test' \
  "Makefile must declare the architecture import lint targets"
require_unique_make_target arch-import-lint
require_unique_make_target arch-import-lint-test
arch_import_recipe="$(make_target_recipe 'arch-import-lint:')" ||
  fail "architecture import lint target must be unique"
[[ "$arch_import_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/check_arch_imports.go -root .' ]] ||
  fail "architecture import lint target lost the frozen checker call"
arch_import_test_recipe="$(make_target_recipe 'arch-import-lint-test:')" ||
  fail "architecture import lint test target must be unique"
[[ "$arch_import_test_recipe" = $'\t@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_arch_imports.sh' ]] ||
  fail "architecture import lint test target lost the frozen runner call"
for target in arch-import-lint arch-import-lint-test; do
  [[ "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] ||
    fail "ci-go must depend on $target"
done
require_make_line '.PHONY: ownership-lint ownership-lint-test' \
  "Makefile must declare the ownership lint targets"
for target in ownership-lint ownership-lint-test; do
  require_unique_make_target "$target"
  [[ "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] || fail "ci-go must depend on $target"
done
[[ "$(make_target_recipe 'ownership-lint:')" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/ownership/main.go -root .' ]] ||
  fail "ownership lint target lost the frozen checker call"
[[ "$(make_target_recipe 'ownership-lint-test:')" = $'\t@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_ownership.sh' ]] ||
  fail "ownership lint test target lost the frozen runner call"
require_make_line '.PHONY: source-policy-lint source-policy-lint-test' \
  "Makefile must declare the source policy lint targets"
for target in source-policy-lint source-policy-lint-test; do
  require_unique_make_target "$target"
  [[ "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] || fail "ci-go must depend on $target"
done
[[ "$(make_target_recipe 'source-policy-lint:')" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) run scripts/sourcepolicy/main.go -root .' ]] ||
  fail "source policy lint target lost the frozen checker call"
[[ "$(make_target_recipe 'source-policy-lint-test:')" = $'\t@env -u BASH_ENV -u ENV GO="$(GO)" scripts/test_source_policy.sh' ]] ||
  fail "source policy lint test target lost the frozen runner call"
require_make_line '.PHONY: slice-input-contract slice-input-contract-test' \
  "Makefile must declare the slice input contract targets"
require_make_line 'slice-input-contract slice-input-contract-test: override SHELL := /bin/bash' \
  "slice input contract targets must use absolute Bash"
require_make_line 'slice-input-contract slice-input-contract-test: override .SHELLFLAGS := -eu -o pipefail -c' \
  "slice input contract targets must pin fail-closed Bash flags"
for target in slice-input-contract slice-input-contract-test; do
  require_unique_make_target "$target"
done
[[ "$(make_target_recipe 'slice-input-contract:')" = $'\t@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_slice_inputs.sh' ]] ||
  fail "slice input contract target lost the frozen checker call"
require_make_line 'slice-input-contract-test: slice-input-contract' \
  "slice input tests must depend on the canonical checker"
[[ "$(make_target_recipe 'slice-input-contract-test: slice-input-contract')" = $'\t@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/test_slice_inputs.sh' ]] ||
  fail "slice input contract tests lost the frozen runner call"
[[ "$ci_go_target" =~ (^|[[:space:]])slice-input-contract-test($|[[:space:]]) ]] ||
  fail "ci-go must depend on slice-input-contract-test"
require_make_line '.PHONY: snapshot-gate snapshot-gate-test' \
  "Makefile must declare the snapshot gate targets"
require_unique_make_target snapshot-gate
require_unique_make_target snapshot-gate-test
require_make_line 'snapshot-gate-test: snapshot-gate' \
  "snapshot gate tests must depend on canonical catalog validation"
[[ "$ci_go_target" =~ (^|[[:space:]])snapshot-gate-test($|[[:space:]]) ]] ||
  fail "ci-go must depend on snapshot gate tests"
snapshot_gate_recipe="$(make_target_recipe 'snapshot-gate:')" ||
  fail "snapshot gate target must be unique"
[[ "$snapshot_gate_recipe" = $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./snapshot-gate validate ../acceptance/snapshots/catalog.v1.json' ]] ||
  fail "snapshot gate target lost canonical catalog validation"
! grep -Fq 'contract-replay' <<<"$(git show ':Makefile')" ||
  fail "Makefile restored the retired contract replay target"
snapshot_gate_test_recipe="$(make_target_recipe 'snapshot-gate-test: snapshot-gate')" ||
  fail "snapshot gate test target must be unique"
for line in \
  $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./snapshot-gate' \
  $'\t@env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=15s ./snapshot-gate' \
  $'\t@env -u BASH_ENV -u ENV GO="$(GO)" acceptance/p0s10/test_snapshot_gate.sh'; do
  printf '%s\n' "$snapshot_gate_test_recipe" | grep -Fqx "$line" ||
    fail "snapshot gate tests lost a frozen vet, race, or gitless call"
done

require_make_line '.PHONY: legacy-route-export-test' \
  "Makefile must declare the P1-S01 exporter test target"
require_unique_make_target legacy-route-export-test
[[ "$ci_go_target" =~ (^|[[:space:]])legacy-route-export-test($|[[:space:]]) ]] ||
  fail "ci-go must depend on the P1-S01 exporter tests"
p1_s01_recipe="$(make_target_recipe 'legacy-route-export-test:')" ||
  fail "P1-S01 exporter test target must be unique"
for call in '$(GO) -C tools vet ./legacy-route-export' '$(GO) -C tools test -race -timeout=15s ./legacy-route-export'; do
  [[ "$(grep -Fc "$call" <<<"$p1_s01_recipe" || true)" = "1" ]] ||
    fail "P1-S01 exporter lost a frozen vet or race test call"
done

require_make_line '.PHONY: feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion' \
  "Makefile must declare the feature matrix contract and completion targets"
require_make_line 'feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: override SHELL := /bin/bash' \
  "feature matrix targets must use the absolute Bash interpreter"
require_make_line 'feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: override .SHELLFLAGS := -eu -o pipefail -c' \
  "feature matrix targets must pin fail-closed Bash flags"
for target in feature-matrix-contract feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion; do
  require_unique_make_target "$target"
done
feature_matrix_recipe="$(make_target_recipe 'feature-matrix-contract:')" ||
  fail "feature matrix contract target must be unique"
[[ "$feature_matrix_recipe" = $'\t@/usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_feature_matrix_contract.sh' ]] ||
  fail "feature matrix contract target lost the frozen validator call"
completion_header='feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion: feature-matrix-contract'
completion_recipe="$(make_target_recipe "$completion_header")" ||
  fail "feature matrix completion targets must share one frozen recipe"
[[ "$completion_recipe" = $'\t@phase="$@"; phase="$${phase#feature-matrix-}"; phase="$${phase%-completion}"; /usr/bin/env -u BASH_ENV -u ENV /bin/bash scripts/check_feature_matrix_contract.sh --completion "$$phase"' ]] ||
  fail "feature matrix completion targets lost phase isolation"
[[ "$ci_go_target" =~ (^|[[:space:]])feature-matrix-contract($|[[:space:]]) ]] ||
  fail "ci-go must depend on feature-matrix-contract"
for target in feature-matrix-p1-completion feature-matrix-p4-completion feature-matrix-p5-completion; do
  [[ ! "$ci_go_target" =~ (^|[[:space:]])$target($|[[:space:]]) ]] || fail "completion targets must stay outside base ci-go: $target"
done

require_make_line '.PHONY: migration-mapping-contract migration-mapping-p1-completion' "Makefile must declare the migration mapping gates"
for target in migration-mapping-contract migration-mapping-p1-completion; do require_unique_make_target "$target"; done
mapping_recipe="$(make_target_recipe 'migration-mapping-contract:')" || fail "migration mapping contract target must be unique"
for call in '$(GO) -C tools vet ./migration-mapping' '$(GO) -C tools test -race -timeout=15s ./migration-mapping' '$(GO) -C tools run ./migration-mapping'; do [[ "$(grep -Fc "$call" <<<"$mapping_recipe" || true)" = "1" ]] || fail "migration mapping contract lost a frozen Go call"; done
require_make_line 'migration-mapping-p1-completion: migration-mapping-contract' "migration mapping completion must depend on its contract"
[[ "$(make_target_recipe 'migration-mapping-p1-completion: migration-mapping-contract')" = *'$(GO) -C tools run ./migration-mapping --completion'* ]] || fail "migration mapping completion lost its pending gate"
[[ "$ci_go_target" =~ (^|[[:space:]])migration-mapping-contract($|[[:space:]]) && ! "$ci_go_target" =~ (^|[[:space:]])migration-mapping-p1-completion($|[[:space:]]) ]] || fail "ci-go must run only the base migration mapping contract"

require_make_line '.PHONY: p1-reconciliation-contract' "Makefile must declare the P1 reconciliation gate"
require_make_line 'p1-reconciliation-contract: override SHELL := /bin/bash' "P1 reconciliation must use absolute Bash"
require_make_line 'p1-reconciliation-contract: override .SHELLFLAGS := -eu -o pipefail -c' "P1 reconciliation must pin fail-closed Bash flags"
require_make_line 'p1-reconciliation-contract: override GO := go' "P1 reconciliation must reject hostile GO overrides"
require_unique_make_target 'p1-reconciliation-contract'
reconciliation_recipe="$(make_target_recipe 'p1-reconciliation-contract:')" || fail "P1 reconciliation target must be unique"
for line in \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./p1-reconciliation' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./p1-reconciliation' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./p1-reconciliation'; do
  printf '%s\n' "$reconciliation_recipe" | grep -Fqx "$line" || fail "P1 reconciliation lost a frozen Go call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])p1-reconciliation-contract($|[[:space:]]) ]] || fail "ci-go must depend on p1-reconciliation-contract"

require_make_line '.PHONY: openapi-p1-contract' "Makefile must declare the P1-S11 OpenAPI gate"
require_make_line 'openapi-p1-contract: override SHELL := /bin/bash' "P1-S11 OpenAPI gate must use absolute Bash"
require_make_line 'openapi-p1-contract: override .SHELLFLAGS := -eu -o pipefail -c' "P1-S11 OpenAPI gate must pin fail-closed Bash flags"
require_make_line 'openapi-p1-contract: override GO := go' "P1-S11 OpenAPI gate must reject hostile GO overrides"
require_unique_make_target 'openapi-p1-contract'
openapi_p1_recipe="$(make_target_recipe 'openapi-p1-contract:')" || fail "P1-S11 OpenAPI target must be unique"
for line in \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./openapi-contract' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./openapi-contract' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./openapi-contract' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) test -race -timeout=20s ./acceptance/p1s11'; do
  printf '%s\n' "$openapi_p1_recipe" | grep -Fqx "$line" || fail "P1-S11 OpenAPI gate lost a frozen validation call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])openapi-p1-contract($|[[:space:]]) ]] || fail "ci-go must depend on openapi-p1-contract"

require_make_line '.PHONY: query-plan-gate query-plan-gate-test' "Makefile must declare the query plan gates"
require_make_line 'query-plan-gate query-plan-gate-test: override SHELL := /bin/bash' "query plan targets must use absolute Bash"
require_make_line 'query-plan-gate query-plan-gate-test: override .SHELLFLAGS := -eu -o pipefail -c' "query plan targets must pin fail-closed Bash flags"
require_make_line 'query-plan-gate query-plan-gate-test: override GO := go' "query plan targets must reject hostile GO overrides"
for target in query-plan-gate query-plan-gate-test; do require_unique_make_target "$target"; done
query_plan_recipe="$(make_target_recipe 'query-plan-gate:')" || fail "query plan gate target must be unique"
[[ "$query_plan_recipe" = $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools run ./query-plan-gate -root .. -base "$$QUERY_PLAN_BASE_SHA" -head "$$QUERY_PLAN_HEAD_SHA" -database-url "$$QUERY_PLAN_TEST_DATABASE_URL"' ]] ||
  fail "query plan gate lost its frozen Git and database inputs"
require_make_line 'query-plan-gate-test: query-plan-gate' "query plan tests must depend on the real gate"
query_plan_test_recipe="$(make_target_recipe 'query-plan-gate-test: query-plan-gate')" || fail "query plan test target must be unique"
for line in \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools vet ./query-plan-gate' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly $(GO) -C tools test -race -timeout=20s ./query-plan-gate' \
  $'\t@/usr/bin/env -u BASH_ENV -u ENV GO="$(GO)" /bin/bash scripts/test_query_plan_gate.sh'; do
  printf '%s\n' "$query_plan_test_recipe" | grep -Fqx "$line" || fail "query plan tests lost a frozen static or PostgreSQL call"
done
[[ "$ci_go_target" =~ (^|[[:space:]])query-plan-gate-test($|[[:space:]]) ]] || fail "ci-go must depend on query-plan-gate-test"

application_go_workflow="$(git show ':.github/workflows/application-go.yml')"
verify_postgres_step="$(
  awk '
    $0 == "      - name: Verify PostgreSQL version and migrations" { seen++; capture = 1; next }
    capture && /^      - name:/ { exit }
    capture { print }
    END { exit !(seen == 1) }
  ' <<<"$application_go_workflow"
)" || fail "application workflow must contain one PostgreSQL verification step"
for line in \
  '          ALLOW_DESTRUCTIVE_RIVER_MIGRATION_TEST: "1"' \
  '          make migration-integration' \
  '          make p0-s04-integration'; do
  [[ "$(printf '%s\n' "$verify_postgres_step" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost the fixed P0-S04 integration environment or call"
done
for line in '      BASH_ENV: ""' '      ENV: ""'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow must clear BASH_ENV and ENV"
done
for line in \
  '          QUERY_PLAN_TEST_DATABASE_URL: postgres://postgres:postgres@127.0.0.1:5432/aicrm_test?sslmode=disable' \
  '          QUERY_PLAN_BASE_SHA: ${{ github.event.pull_request.base.sha || github.event.before || github.sha }}' \
  '          QUERY_PLAN_HEAD_SHA: ${{ github.sha }}'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost a frozen query plan gate input"
done
for line in \
  '  web:' \
  '    name: application / web' \
  '        uses: actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38' \
  '          node-version: 24.18.0' \
  '        run: npm install --global npm@11.12.1 --ignore-scripts --no-audit --no-fund' \
  '          npm ci --ignore-scripts --no-audit --no-fund' \
  '          npm run ci'; do
  [[ "$(printf '%s\n' "$application_go_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "application workflow lost a frozen P0-S05 web gate: $line"
done

secret_scan_workflow="$(git show ':'.github/workflows/secret-scan.yml)"
for line in \
  '        run: gitleaks git . --config .gitleaks.toml --redact --no-banner --exit-code 1' \
  '        run: scripts/test_gitleaks_config.sh'; do
  [[ "$(printf '%s\n' "$secret_scan_workflow" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "secret scan workflow lost its pinned config or false-positive regression: $line"
done
gitleaks_config="$(git show ':'.gitleaks.toml)"
for line in \
  'useDefault = true' \
  'targetRules = ["generic-api-key"]' \
  'condition = "AND"' \
  'regexTarget = "line"' \
  "paths = ['''(^|/)web/src/api/generated/[^/]+\.ts$''']" \
  "regexes = ['''^\s*\*\s+OpenAPI spec version:\s+[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?\s*$''']"; do
  [[ "$(printf '%s\n' "$gitleaks_config" | grep -Fxc "$line" || true)" = "1" ]] ||
    fail "gitleaks false-positive boundary drifted: $line"
done

expected_workflows="$({
  printf '%s\n' .github/workflows/application-go.yml
  printf '%s\n' .github/workflows/repo-contract.yml
  printf '%s\n' .github/workflows/secret-scan.yml
} | LC_ALL=C sort)"
actual_workflows="$(
  git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml' |
    LC_ALL=C sort
)"
[[ "$actual_workflows" = "$expected_workflows" ]] ||
  fail "workflow file set drifted; every workflow requires a Codex-owned hash update"

for number in $(seq -w 1 11); do
  [[ -f "docs/adr/ADR-0${number}.md" ]] || fail "missing ADR-0${number}"
done

if git ls-files tools/contract-replay acceptance/p0s10/test_contract_replay.sh | grep -q .; then
  fail "retired contract replay implementation is still tracked"
fi
[[ "$(git ls-files acceptance/snapshots)" = "acceptance/snapshots/catalog.v1.json" ]] ||
  fail "snapshot catalog inventory drifted; tracked actual responses are forbidden"
grep -Fq 'ADR-011' <<<"$(git show ':docs/adr/ADR-001.md')" || fail "ADR-001 lost its ADR-011 supersession marker"
grep -Fq 'ADR-011' <<<"$(git show ':docs/adr/ADR-010.md')" || fail "ADR-010 lost its ADR-011 supersession marker"
grep -Fq '不能证明新旧系统行为一致' <<<"$(git show ':docs/adr/ADR-011.md')" || fail "ADR-011 lost the snapshot capability boundary"

(cd docs/spec && sha256sum -c SHA256SUMS)

v1_plan="$(git show ':docs/spec/AI-CRM-v2-执行方案.md')"
v2_plan="$(git show ':docs/spec/AI-CRM-v2-执行方案-v2-至P3.md')"
design="$(git show ':docs/spec/AI-CRM-v2-重构详细设计.md')"
grep -Fq '> P0–P3 范围以《AI-CRM-v2-执行方案-v2-至P3.md》为准。' <<<"$v1_plan" ||
  fail "v1 plan must defer P0-P3 to the v2 plan"
grep -Fq 'b3613f635692c932021036f8f81babf24fca8222' <<<"$v2_plan" ||
  fail "v2 plan lost the frozen actual baseline footnote"
customers_ddl="$(awk '/^CREATE TABLE customers \(/ { capture=1 } capture { print } capture && /^\);/ { exit }' <<<"$design")"
[[ -n "$customers_ddl" ]] || fail "design lost the customers DDL"
! grep -Eq '^[[:space:]]+(external_userid|unionid|openid|phone)[[:space:]]' <<<"$customers_ddl" ||
  fail "customers DDL restored a forbidden external identity column"
! grep -Fq 'idx_customers_unionid' <<<"$design" || fail "design restored the forbidden customers unionid index"
grep -Eq '^[[:space:]]+scope[[:space:]]+TEXT[[:space:]]+NOT NULL' <<<"$design" ||
  fail "design identities scope must remain required"
! grep -Fq '/tools/contract-replay' <<<"$design" || fail "design restored the retired contract replay path"
grep -Fq '/acceptance/snapshots' <<<"$design" || fail "design lost the snapshot gate path"
grep -Fq '快照只防新系统自身回归，不能防新旧行为不一致' <<<"$design" ||
  fail "design lost the snapshot capability limitation"

forbidden_path_pattern='(^|/)(\.env[^/]*|node_modules|vendor|dist|build|coverage|\.cache|playwright-report|test-results|\.auth|\.browser)(/|$)|^(data|runtime|logs|uploads|tmp)(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx|db|sqlite|sqlite3|dump|zip)$'
if git ls-files | grep -E "$forbidden_path_pattern" >/dev/null; then
  git ls-files | grep -E "$forbidden_path_pattern" >&2
  fail "forbidden generated, credential, data, or binary path is tracked"
fi

if git ls-files | grep -E '(^|/)handoffs(/|$)|AI-CRM-v2-v3\.(patch|zip)$' >/dev/null; then
  fail "rejected Pro handoff artifacts must not enter the repository"
fi

if git grep --cached -n 'pull_request_target' -- .github/workflows; then
  fail "pull_request_target is forbidden"
fi

if git grep --cached -n -F '\' -- .github/workflows; then
  fail "workflow backslashes are forbidden because YAML escapes bypass text policy"
fi

if git grep --cached -n -E '[&*]|!!|<<[[:space:]]*:' -- .github/workflows; then
  fail "workflow YAML anchors, aliases, tags, and merge keys are forbidden"
fi

if git grep --cached -n -E \
  "[\"'][[:alnum:]_-]+[\"'][[:space:]]*:" \
  -- .github/workflows; then
  fail "quoted workflow keys are forbidden because they bypass policy scanners"
fi

if git grep --cached -n -i -E \
  '(^|[^[:alnum:]_])write(-all)?([^[:alnum:]_]|$)|(^|[^[:alnum:]_])environment([^[:alnum:]_]|$)|(^|[^[:alnum:]_])deploy(ment|ments|ing)?([^[:alnum:]_]|$)' \
  -- .github/workflows; then
  fail "workflow write permission, environment, or deployment is forbidden during bootstrap"
fi

if git grep --cached -n -i -E \
  '(^|[^[:alnum:]_])secrets([^[:alnum:]_]|$)' \
  -- .github/workflows; then
  fail "workflow secrets context is forbidden during bootstrap"
fi

while IFS= read -r workflow; do
  awk '
    /^permissions:[[:space:]]*$/ { in_top_permissions = 1; saw_permissions = 1; next }
    in_top_permissions && /^[^[:space:]]/ { in_top_permissions = 0 }
    in_top_permissions && /^[[:space:]]*($|#)/ { next }
    in_top_permissions {
      permission_entries++
      if ($0 != "  contents: read") invalid_permission = 1
    }
    END {
      exit !(saw_permissions && permission_entries == 1 && !invalid_permission)
    }
  ' "$workflow" ||
    fail "$workflow must declare only top-level contents: read"

  [[ "$(grep -Ec '^[[:space:]]*permissions[[:space:]]*:' "$workflow")" -eq 1 ]] ||
    fail "$workflow must have exactly one canonical permissions key"

  canonical_uses_pattern='^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*[^[:space:]#]+([[:space:]]*#.*)?$'
  uses_key_pattern='(^|[^[:alnum:]_])uses[[:space:]]*:'
  while IFS= read -r workflow_line; do
    [[ ! "$workflow_line" =~ $uses_key_pattern ]] ||
      [[ "$workflow_line" =~ $canonical_uses_pattern ]] ||
      fail "$workflow contains a non-canonical uses mapping"
  done < "$workflow"
done < <(git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml')

while IFS= read -r action_ref; do
  [[ -z "$action_ref" ]] && continue
  [[ "$action_ref" != ./* ]] || continue
  action_name="${action_ref%@*}"
  action_sha="${action_ref##*@}"
  [[ -n "$action_name" && "$action_ref" == *@* &&
    "$action_sha" =~ ^[0-9a-f]{40}$ ]] ||
    fail "GitHub Action is not pinned to a full lowercase hex commit SHA: $action_ref"
done < <(
  git grep --cached -h -E '^[[:space:]]*(-[[:space:]]*)?uses:' \
    -- .github/workflows |
    sed -E 's/^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*([^[:space:]#]+).*/\2/'
)

scripts/scan_sensitive_paths.sh

echo "repo-contract: PASS"
