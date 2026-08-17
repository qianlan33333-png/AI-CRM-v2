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
            database_mode="full",
            sqlc="true",
        )

    def test_go_dependencies(self) -> None:
        self.assert_flags(
            ["go.mod"],
            go="true",
            go_selected="false",
            go_mode="full",
            shared="true",
            vulnerability="true",
        )

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
            "scripts/test_repo_fingerprints.sh",
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
            "scripts/test_repo_fingerprints.sh",
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

    def test_new_workflows_and_permissions(self) -> None:
        for relative_name in ACTIVE_WORKFLOWS:
            source = (REPO_ROOT / relative_name).read_text(encoding="utf-8")
            self.assertRegex(source, r"(?m)^permissions:\n  contents: read\n")
            permission_entries = re.findall(r"(?m)^  [a-z-]+: (?:read|write|write-all)$", source)
            self.assertEqual(permission_entries, ["  contents: read"])
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

    def test_nightly_is_not_a_pull_request_gate(self) -> None:
        source = (REPO_ROOT / ".github/workflows/nightly.yml").read_text(encoding="utf-8")
        self.assertNotIn("pull_request:", source)
        self.assertIn("schedule:", source)
        self.assertIn("workflow_dispatch:", source)
        self.assertNotIn("ci / merge-gate", source)

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
