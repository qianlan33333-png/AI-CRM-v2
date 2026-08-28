#!/usr/bin/env python3
"""Deterministic tests for relevance CI classification, wiring, and merge-gate truth."""

from __future__ import annotations

import importlib.util
import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any

sys.dont_write_bytecode = True

SCRIPT = Path(__file__).resolve()
REPO_ROOT = SCRIPT.parents[2]
CLASSIFIER_PATH = REPO_ROOT / "scripts/ci/classify_changes.py"
MAP_PATH = REPO_ROOT / ".github/ci-map.yml"

spec = importlib.util.spec_from_file_location("ci_classifier", CLASSIFIER_PATH)
if spec is None or spec.loader is None:
    raise RuntimeError("cannot load classifier")
classifier = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = classifier
spec.loader.exec_module(classifier)

ACTIVE_WORKFLOWS = [
    ".github/workflows/ci.yml",
    ".github/workflows/nightly.yml",
]
EXPECTED_JOB_IDS = {
    "classify",
    "secret_diff",
    "go_selected",
    "web",
    "api_codegen",
    "database",
    "shared_regression",
    "ci_self",
    "merge_gate",
}
EXPECTED_CLASSIFY_OUTPUTS = {
    "go",
    "go_selected",
    "go_mode",
    "go_groups",
    "web",
    "web_mode",
    "web_audit",
    "api",
    "database",
    "database_mode",
    "database_groups",
    "shared",
    "ci",
    "vulnerability",
    "security_expanded",
}


def selection_outputs(repo_paths: list[str]) -> dict[str, str]:
    ci_map = classifier.load_ci_map(MAP_PATH)
    return classifier.classify(repo_paths, ci_map).outputs()


class ClassificationMatrixTests(unittest.TestCase):
    def assert_flags(self, repo_paths: list[str], **expected: str) -> dict[str, str]:
        values = selection_outputs(repo_paths)
        for key, value in expected.items():
            self.assertEqual(values[key], value, f"{repo_paths}: {key}")
        return values

    def test_docs_only(self) -> None:
        values = self.assert_flags(
            ["docs/evidence/slices/P4-X.md", "docs/api-mapping.jsonl"],
            basic_only="true",
            go="false",
            web="false",
            api="false",
            database="false",
            shared="false",
            ci="false",
        )
        self.assertEqual(values["fallback_reasons"], "")

    def test_media_only(self) -> None:
        self.assert_flags(
            ["internal/media/app/facets.go"],
            go="true",
            go_selected="true",
            go_mode="selected",
            go_groups="media",
            database="false",
            shared="false",
        )

    def test_media_store_adds_database(self) -> None:
        self.assert_flags(
            ["internal/media/store/queries/facets.sql"],
            go="true",
            go_mode="selected",
            go_groups="media",
            database="true",
            database_mode="selected",
            database_groups="media",
            sqlc="true",
        )

    def test_composition(self) -> None:
        self.assert_flags(
            ["cmd/aicrm/legacy_image_library_api.go"],
            go="true",
            go_mode="selected",
            go_groups="composition",
            shared="false",
        )

    def test_web_and_lock(self) -> None:
        self.assert_flags(
            ["web/src/media.tsx"],
            web="true",
            web_mode="full",
            web_audit="false",
        )
        self.assert_flags(
            ["package-lock.json"],
            web="true",
            web_mode="full",
            web_audit="true",
        )

    def test_openapi_plus_web_build(self) -> None:
        self.assert_flags(
            ["api/openapi.yaml"],
            api="true",
            web="true",
            web_mode="build",
            go="false",
            database="false",
        )

    def test_migration(self) -> None:
        self.assert_flags(
            ["migrations/00047_example.sql"],
            database="true",
            database_mode="selected",
            database_groups="",
            sqlc="true",
        )

    def test_data_migration_harness_selects_bounded_group(self) -> None:
        for path in (
            "internal/migration/store/queries/runtime.sql",
            "acceptance/datamigration/integration_test.go",
        ):
            self.assert_flags(
                [path],
                go="true",
                go_mode="selected",
                go_groups="migration",
                database="true",
                database_mode="selected",
                database_groups="migration",
                sqlc="true",
                shared="false",
            )

    def test_domain_stores_select_database_consumers(self) -> None:
        self.assert_flags(
            ["internal/automation/store/agents.go", "internal/contact/store/channels.go"],
            database="true",
            database_mode="selected",
            database_groups="automation,contact",
            sqlc="true",
        )

    def test_events_store_and_acceptance_select_database(self) -> None:
        for path in (
            "internal/events/store/appender.go",
            "acceptance/events/events_store_integration_test.go",
        ):
            self.assert_flags(
                [path],
                go="true",
                go_mode="selected",
                go_groups="events",
                database="true",
                database_mode="selected",
                database_groups="events",
                sqlc="true",
                shared="false",
            )

    def test_shared_acceptance_fixture_has_known_consumers(self) -> None:
        values = self.assert_flags(
            ["acceptance/mediafixture/image.go"],
            go="true",
            go_mode="selected",
            go_groups="automation,contact,media",
            database="true",
            database_mode="selected",
            database_groups="automation,contact,media",
        )
        self.assertEqual(values["fallback_reasons"], "")

    def test_go_dependencies(self) -> None:
        self.assert_flags(
            ["go.mod"],
            go="true",
            go_selected="false",
            go_mode="full",
            shared="true",
            vulnerability="true",
        )

    def test_openapi_contract_tool_selects_api_without_widening_other_tools(self) -> None:
        self.assert_flags(
            ["tools/openapi-contract/main_test.go"],
            api="true",
            web="false",
            go="true",
            go_mode="full",
            shared="true",
            vulnerability="true",
        )
        self.assert_flags(
            ["tools/p1-reconciliation/main_test.go"],
            api="false",
            web="false",
            go="true",
            go_mode="full",
            shared="true",
            vulnerability="true",
        )

    def test_repository_go_tool_is_known(self) -> None:
        values = self.assert_flags(
            ["scripts/ownership/main.go"],
            go="true",
            go_mode="full",
            shared="true",
        )
        self.assertEqual(values["fallback_reasons"], "")

    def test_platform_and_public_port(self) -> None:
        self.assert_flags(
            ["internal/platform/http/gateway.go"],
            go="true",
            go_mode="full",
            shared="true",
        )
        self.assert_flags(
            ["internal/media/port/port.go"],
            go="true",
            go_mode="full",
            shared="true",
        )

    def test_ci_self_and_business_stack(self) -> None:
        self.assert_flags(
            [".github/workflows/ci.yml"],
            ci="true",
            go="false",
            web="false",
            database="false",
        )
        self.assert_flags(
            [".github/ci-map.yml", "internal/media/app/facets.go"],
            ci="true",
            go="true",
            go_mode="selected",
            go_groups="media",
        )

    def test_security_expansion(self) -> None:
        self.assert_flags(
            [".gitleaks.toml"],
            security_expanded="true",
            go="false",
        )
        self.assert_flags(
            ["internal/auth/app/service.go"],
            security_expanded="true",
            go="true",
            go_mode="selected",
            go_groups="auth",
        )

    def test_unknown_code_fallbacks(self) -> None:
        self.assert_flags(
            ["cmd/experimental/main.go"],
            go="true",
            go_mode="full",
            shared="true",
            fallback_reasons="unknown-go",
        )
        self.assert_flags(
            ["database/custom.sql"],
            database="true",
            database_mode="full",
            fallback_reasons="unknown-sql",
        )
        self.assert_flags(
            ["frontend/app.tsx"],
            web="true",
            web_mode="full",
            fallback_reasons="unknown-web",
        )
        self.assert_flags(
            ["frontend/index.html"],
            web="true",
            web_mode="full",
            fallback_reasons="unknown-web",
        )

    def test_order_is_deterministic(self) -> None:
        first = selection_outputs(["internal/media/app/facets.go", ".github/ci-map.yml"])
        second = selection_outputs([".github/ci-map.yml", "internal/media/app/facets.go"])
        self.assertEqual(first, second)


class MergeGateTests(unittest.TestCase):
    def test_success_and_skipped_pass(self) -> None:
        passed, parsed = classifier.evaluate_merge_gate(
            ["classify=success", "secret-diff=success", "go=skipped", "web=success"]
        )
        self.assertTrue(passed)
        self.assertEqual(parsed[2], ("go", "skipped"))

    def test_failure_and_cancelled_block(self) -> None:
        self.assertFalse(classifier.evaluate_merge_gate(["classify=success", "go=failure"])[0])
        self.assertFalse(classifier.evaluate_merge_gate(["classify=success", "go=cancelled"])[0])

    def test_always_jobs_must_not_be_skipped(self) -> None:
        self.assertFalse(
            classifier.evaluate_merge_gate(["classify=skipped", "secret-diff=success", "go=skipped"])[0]
        )
        self.assertFalse(
            classifier.evaluate_merge_gate(["classify=success", "secret-diff=skipped", "go=skipped"])[0]
        )

    def test_invalid_result_fails_closed(self) -> None:
        with self.assertRaises(classifier.ClassificationError):
            classifier.evaluate_merge_gate(["go="])
        with self.assertRaises(classifier.ClassificationError):
            classifier.evaluate_merge_gate(["go=neutral"])
        with self.assertRaises(classifier.ClassificationError):
            classifier.evaluate_merge_gate([])
        with self.assertRaises(classifier.ClassificationError):
            classifier.evaluate_merge_gate(["go=success", "go=skipped"])


class CliTests(unittest.TestCase):
    def test_valid_and_invalid_shas(self) -> None:
        with tempfile.TemporaryDirectory(prefix="ci-selector-git-") as temporary:
            fixture = Path(temporary)
            subprocess.run(["git", "init", "-q", str(fixture)], check=True)
            subprocess.run(["git", "-C", str(fixture), "config", "user.name", "Selector Test"], check=True)
            subprocess.run(["git", "-C", str(fixture), "config", "user.email", "selector@example.invalid"], check=True)
            (fixture / "docs").mkdir()
            (fixture / "docs/base.md").write_text("base\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(fixture), "add", "."], check=True)
            subprocess.run(["git", "-C", str(fixture), "commit", "-qm", "base"], check=True)
            base = subprocess.check_output(["git", "-C", str(fixture), "rev-parse", "HEAD"], text=True).strip()
            (fixture / "docs/base.md").write_text("head\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(fixture), "commit", "-qam", "head"], check=True)
            head = subprocess.check_output(["git", "-C", str(fixture), "rev-parse", "HEAD"], text=True).strip()

            valid = subprocess.run(
                [
                    sys.executable,
                    str(CLASSIFIER_PATH),
                    "--base",
                    base,
                    "--head",
                    head,
                    "--repo",
                    str(fixture),
                    "--map",
                    str(MAP_PATH),
                    "--print-json",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
            self.assertEqual(valid.returncode, 0, valid.stderr)

            for invalid_base, invalid_head in [
                ("0" * 40, head),
                (base, "bad"),
                (base, base),
            ]:
                failed = subprocess.run(
                    [
                        sys.executable,
                        str(CLASSIFIER_PATH),
                        "--base",
                        invalid_base,
                        "--head",
                        invalid_head,
                        "--repo",
                        str(fixture),
                        "--map",
                        str(MAP_PATH),
                    ],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    check=False,
                )
                self.assertEqual(failed.returncode, 2)
                self.assertIn("ci-classifier:", failed.stderr)


class WorkflowWiringTests(unittest.TestCase):
    def test_radar_tracking_runner_is_selected_and_nightly_gated(self) -> None:
        runner = (REPO_ROOT / "scripts/ci/run_selected_database.sh").read_text(encoding="utf-8")
        self.assertIn("    radar)\n", runner)
        self.assertIn(
            "run_make_acceptance P4_RADAR_TRACKING_TEST_DATABASE_URL p4-radar-local-tracking-acceptance",
            runner,
        )
        go_runner = (REPO_ROOT / "scripts/ci/run_selected_go.sh").read_text(encoding="utf-8")
        self.assertIn("|radar|", go_runner)
        makefile = (REPO_ROOT / "Makefile").read_text(encoding="utf-8")
        self.assertIn("p4-radar-local-tracking-acceptance:", makefile)
        self.assertIn("acceptance/radar/tracking_pg16.sh", makefile)
        manifest = (REPO_ROOT / "docs/ci/go-acceptance-manifest.tsv").read_text(encoding="utf-8")
        self.assertIn("p4-radar-local-tracking|0068|P4_RADAR_TRACKING_TEST_DATABASE_URL", manifest)

    def test_data_migration_harness_runner_is_migration_gated(self) -> None:
        source = (REPO_ROOT / "scripts/ci/run_selected_database.sh").read_text(encoding="utf-8")
        selected_migrations = source.index("run_migration_checks\n\n# A migration-only")
        migration_case = source.index("    migration)\n")
        self.assertLess(selected_migrations, migration_case)
        self.assertIn(
            "run_make_acceptance P4DMH_TEST_DATABASE_URL p4-data-migration-harness-acceptance",
            source,
        )
        makefile = (REPO_ROOT / "Makefile").read_text(encoding="utf-8")
        self.assertIn("p4-data-migration-harness-acceptance:", makefile)
        self.assertIn("acceptance/datamigration/run_pg16.sh", makefile)
        manifest = (REPO_ROOT / "docs/ci/go-acceptance-manifest.tsv").read_text(encoding="utf-8")
        self.assertIn("p4-data-migration-harness|0063|P4DMH_TEST_DATABASE_URL", manifest)

    def test_v1_archive_runner_is_selected_and_nightly_gated(self) -> None:
        ci_map = (REPO_ROOT / ".github/ci-map.yml").read_text(encoding="utf-8")
        self.assertIn('"internal/migration/v1archive/**"', ci_map)
        self.assertIn('"cmd/aicrm-v1-import/**"', ci_map)
        runner = (REPO_ROOT / "scripts/ci/run_selected_database.sh").read_text(encoding="utf-8")
        self.assertIn(
            "run_make_acceptance P4_V1_ARCHIVE_TEST_DATABASE_URL p4-v1-archive-migration-acceptance",
            runner,
        )
        manifest = (REPO_ROOT / "docs/ci/go-acceptance-manifest.tsv").read_text(encoding="utf-8")
        self.assertIn("p4-v1-full-archive|0078|P4_V1_ARCHIVE_TEST_DATABASE_URL", manifest)

    def test_full_database_gate_includes_dm01(self) -> None:
        ci_source = (REPO_ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        self.assertIn(
            "if: needs.classify.outputs.database_mode == 'full' || contains(needs.classify.outputs.database_groups, 'dm01')",
            ci_source,
        )
        runner_source = (REPO_ROOT / "scripts/ci/run_selected_database.sh").read_text(encoding="utf-8")
        full_block = runner_source[runner_source.index('if [[ "$selection_mode" = "full" ]]'):]
        self.assertIn("DM01_SOURCE_TEST_DATABASE_URL is required in full mode", full_block)
        self.assertIn("DM01_TARGET_TEST_DATABASE_URL is required in full mode", full_block)
        self.assertIn("p4-dm01-migration-acceptance", full_block)
        self.assertIn("p4-dm01-two-pg-acceptance", full_block)

        nightly_source = (REPO_ROOT / ".github/workflows/nightly.yml").read_text(encoding="utf-8")
        self.assertIn("Create DM01 source and target databases", nightly_source)
        self.assertIn("DM01_SOURCE_TEST_DATABASE_URL:", nightly_source)
        self.assertIn("DM01_TARGET_TEST_DATABASE_URL:", nightly_source)
        nightly_runner_source = (REPO_ROOT / "scripts/ci/run_full_regression.sh").read_text(encoding="utf-8")
        self.assertIn("p4-dm01-migration-acceptance", nightly_runner_source)
        self.assertIn("p4-dm01-two-pg-acceptance", nightly_runner_source)
        self.assertIn('-survey-unresolved-history-postgres-dsn="$database_url"', nightly_runner_source)
        for flag in (
            "automation-history-test-database-url",
            "signup-tag-history-store-postgres-dsn",
            "customer-state-history-postgres-dsn",
            "profile-catalog-history-store-postgres-dsn",
            "marketing-state-history-postgres-dsn",
            "hxc-history-store-postgres-dsn",
            "static-product-history-postgres-dsn",
            "static-media-history-postgres-dsn",
            "static-cycle-history-postgres-dsn",
        ):
            self.assertIn(f'-{flag}="$database_url"', nightly_runner_source)

    def test_events_selected_database_runner_is_migration_gated(self) -> None:
        source = (REPO_ROOT / "scripts/ci/run_selected_database.sh").read_text(encoding="utf-8")
        selected_migrations = source.index("run_migration_checks\n\n# A migration-only")
        events_case = source.index("    events)\n")
        self.assertLess(selected_migrations, events_case)
        self.assertIn(
            "run_make_acceptance P4INTERNAL_EVENTS_TEST_DATABASE_URL p4-internal-events-0367-0368-acceptance",
            source,
        )
        makefile = (REPO_ROOT / "Makefile").read_text(encoding="utf-8")
        self.assertIn("p4-internal-events-0367-0368-acceptance:", makefile)
        self.assertIn("./acceptance/events -args -database-url", makefile)

    def test_only_active_workflows_remain(self) -> None:
        tracked = subprocess.check_output(
            [
                "git",
                "-C",
                str(REPO_ROOT),
                "ls-files",
                "--",
                ".github/workflows/*.yml",
                ".github/workflows/*.yaml",
            ],
            text=True,
        ).splitlines()
        self.assertEqual(tracked, ACTIVE_WORKFLOWS)
        retired_scripts = subprocess.check_output(
            [
                "git",
                "-C",
                str(REPO_ROOT),
                "ls-files",
                "--",
                "scripts/*promotion*",
                "scripts/*candidate*merge*guard*",
            ],
            text=True,
        ).splitlines()
        self.assertEqual(retired_scripts, [])

    def test_active_workflows_parse_as_yaml(self) -> None:
        completed = subprocess.run(
            [
                "ruby",
                "-e",
                'require "yaml"; ARGV.each { |path| YAML.parse_file(path) }',
                *(str(REPO_ROOT / relative_name) for relative_name in ACTIVE_WORKFLOWS),
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=False,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)

    def test_retired_chain_has_no_executable_reference(self) -> None:
        sources = [REPO_ROOT / relative_name for relative_name in ACTIVE_WORKFLOWS]
        sources.extend([MAP_PATH, CLASSIFIER_PATH, REPO_ROOT / "Makefile"])
        sources.extend(sorted((REPO_ROOT / "scripts/ci").glob("*")))
        sources.extend(
            REPO_ROOT / relative_name
            for relative_name in (
                "scripts/check_repo_contract.sh",
                "scripts/test_repo_contract.sh",
            )
        )
        combined = "\n".join(
            path.read_text(encoding="utf-8")
            for path in sources
            if path.is_file()
        )
        retired_tokens = [
            "ci_" + "promotion",
            "check_ci_" + "promotion_smoke",
            "candidate_" + "merge_" + "guard",
            "candidate-" + "merge-" + "guard",
            "application-" + "go.yml",
            "repo-" + "contract.yml",
            "secret-" + "scan.yml",
            "exact-main " + "verification mode",
            "promotion " + "fingerprints",
        ]
        for token in retired_tokens:
            self.assertNotIn(token, combined, token)

    def test_self_test_and_nightly_contract_commands(self) -> None:
        ci_source = (REPO_ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        root_self_tests = (
            "scripts/check_repo_contract.sh",
            "scripts/test_repo_contract.sh",
        )
        for command in ("python3 scripts/ci/test_selector.py", *root_self_tests):
            self.assertIn(command, ci_source)
        for relative_name in root_self_tests:
            file_name = REPO_ROOT / relative_name
            self.assertTrue(file_name.is_file(), relative_name)
            self.assertTrue(os.access(file_name, os.X_OK), relative_name)

        full_source = (REPO_ROOT / "scripts/ci/run_full_regression.sh").read_text(encoding="utf-8")
        for command in (
            "python3 scripts/ci/test_selector.py",
            "scripts/check_repo_contract.sh",
            "scripts/test_repo_contract.sh",
            "make --no-print-directory ci-go",
            "make --no-print-directory migration-integration",
            "scripts/run_ci_acceptance_manifest.sh",
            "npm run ci",
            "gitleaks git .",
            "scripts/test_gitleaks_config.sh",
            "scripts/scan_sensitive_paths.sh",
            "scripts/test_build_slice_bundle.sh",
        ):
            self.assertIn(command, full_source)
        migration_command = "make --no-print-directory migration-integration"
        ci_go_command = "make --no-print-directory ci-go"
        self.assertEqual(full_source.count(migration_command), 1)
        self.assertLess(full_source.index(migration_command), full_source.index(ci_go_command))

    def test_new_workflows_and_permissions(self) -> None:
        for relative_name in ACTIVE_WORKFLOWS:
            source = (REPO_ROOT / relative_name).read_text(encoding="utf-8")
            self.assertRegex(source, r"(?m)^permissions:\n  contents: read\n")
            permission_entries = re.findall(r"(?m)^  [a-z-]+: (?:read|write|write-all)$", source)
            self.assertEqual(permission_entries, ["  contents: read"])
            if relative_name == ".github/workflows/nightly.yml":
                self.assertEqual(
                    re.findall(r"(?m)^      [a-z-]+: (?:write|write-all)$", source),
                    ["      statuses: write"],
                )
            self.assertNotIn("pull_request_target", source)
            self.assertNotRegex(source, r"(?m)^\s+paths(?:-ignore)?:")
            for action_ref in re.findall(r"(?m)^\s+(?:- )?uses: ([^\s#]+)", source):
                self.assertRegex(action_ref, r"^[^@]+@[0-9a-f]{40}$")

    def test_ci_job_ids_outputs_and_gate(self) -> None:
        source = (REPO_ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
        job_ids = set(re.findall(r"(?m)^  ([a-z][a-z0-9_]*):$", source.split("jobs:\n", 1)[1]))
        self.assertEqual(job_ids, EXPECTED_JOB_IDS)
        self.assertNotRegex(source.split("  classify:\n", 1)[1].split("  secret_diff:\n", 1)[0], r"(?m)^    if:")
        self.assertIn("    name: ci / secret-diff\n    needs: classify\n    if: always()", source)
        self.assertIn("    name: ci / merge-gate\n    if: always()", source)
        needed = set(re.findall(r"(?m)^      - ([a-z][a-z0-9_]*)$", source.split("  merge_gate:\n", 1)[1]))
        self.assertTrue(EXPECTED_JOB_IDS - {"merge_gate"} <= needed)
        referenced_jobs = set(re.findall(r"needs\.([a-z][a-z0-9_]*)", source))
        self.assertTrue(referenced_jobs <= EXPECTED_JOB_IDS)
        classify_block = source.split("    outputs:\n", 1)[1].split("    steps:\n", 1)[0]
        classify_outputs = set(re.findall(r"(?m)^      ([a-z][a-z0-9_]*):", classify_block))
        self.assertEqual(classify_outputs, EXPECTED_CLASSIFY_OUTPUTS)
        referenced_outputs = set(re.findall(r"needs\.classify\.outputs\.([a-z][a-z0-9_]*)", source))
        self.assertTrue(referenced_outputs <= EXPECTED_CLASSIFY_OUTPUTS)
        self.assertIn("classify=", source)
        self.assertIn("secret-diff=", source)
        self.assertIn("go=", source)
        self.assertIn("cancelled", CLASSIFIER_PATH.read_text(encoding="utf-8"))

    def test_nightly_uses_trusted_triggers_and_isolates_status_write(self) -> None:
        source = (REPO_ROOT / ".github/workflows/nightly.yml").read_text(encoding="utf-8")
        self.assertNotIn("pull_request:", source)
        self.assertNotIn("workflow_dispatch:", source)
        self.assertIn("push:", source)
        self.assertIn("schedule:", source)
        self.assertIn("workflow_run:", source)
        self.assertNotIn("ci / merge-gate", source)
        full_regression, publisher = source.split("  publish_block_compatibility:\n", 1)
        self.assertNotIn("statuses: write", full_regression)
        self.assertIn("      statuses: write", publisher)
        self.assertNotIn("actions/checkout", publisher)
        self.assertIn("name: Publish block compatibility status", publisher)
        self.assertIn('"context": "ci / block compatibility"', source)

    def test_workflow_script_references_exist(self) -> None:
        references: set[str] = set()
        for relative_name in ACTIVE_WORKFLOWS:
            source = (REPO_ROOT / relative_name).read_text(encoding="utf-8")
            references.update(re.findall(r"scripts/ci/[A-Za-z0-9_./-]+", source))
        self.assertTrue(references)
        for relative_name in sorted(references):
            file_name = REPO_ROOT / relative_name
            self.assertTrue(file_name.is_file(), relative_name)
            self.assertTrue(os.access(file_name, os.X_OK), relative_name)

    def test_shell_syntax_and_python_compile(self) -> None:
        shell_scripts = sorted((REPO_ROOT / "scripts/ci").glob("*.sh"))
        self.assertTrue(shell_scripts)
        for file_name in shell_scripts:
            completed = subprocess.run(
                ["bash", "-n", str(file_name)],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
            self.assertEqual(completed.returncode, 0, f"{file_name}: {completed.stderr}")
        import py_compile

        with tempfile.TemporaryDirectory(prefix="ci-selector-pyc-") as temporary:
            for index, file_name in enumerate((CLASSIFIER_PATH, SCRIPT), start=1):
                py_compile.compile(
                    str(file_name),
                    cfile=str(Path(temporary) / f"module-{index}.pyc"),
                    doraise=True,
                )

    def test_new_chain_has_no_metadata_policy_inputs(self) -> None:
        sources = [MAP_PATH, CLASSIFIER_PATH]
        sources.extend(REPO_ROOT / relative_name for relative_name in ACTIVE_WORKFLOWS)
        sources.extend(sorted((REPO_ROOT / "scripts/ci").glob("*.sh")))
        combined = "\n".join(file_name.read_text(encoding="utf-8") for file_name in sources).lower()
        forbidden_inputs = [
            "github.event.pull_request." + "title",
            "github.event.pull_request." + "body",
            "evidence" + " status",
            "no_schema" + "_or_external_effect",
        ]
        for forbidden in forbidden_inputs:
            self.assertNotIn(forbidden, combined)


if __name__ == "__main__":
    unittest.main(verbosity=2)
