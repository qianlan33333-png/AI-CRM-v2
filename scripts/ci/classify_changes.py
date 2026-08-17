#!/usr/bin/env python3
"""Classify a pull request from exact Git SHAs and a repository-owned map."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Iterable, Sequence

SHA_RE = re.compile(r"^[0-9a-f]{40}$")
GROUP_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
KNOWN_BOOLEAN_EFFECTS = {
    "go",
    "web",
    "api",
    "database",
    "shared",
    "ci",
    "web_audit",
    "vulnerability",
    "security_expanded",
    "sqlc",
}
KNOWN_LIST_EFFECTS = {"go_groups", "database_groups"}
KNOWN_MODE_EFFECTS = {"go_mode", "web_mode", "database_mode"}
WEB_SUFFIXES = {
    ".cjs",
    ".css",
    ".htm",
    ".html",
    ".js",
    ".jsx",
    ".less",
    ".mjs",
    ".sass",
    ".scss",
    ".svelte",
    ".ts",
    ".tsx",
    ".vue",
}
ALWAYS_GATE_JOBS = {"classify", "secret-diff"}
ALLOWED_GATE_RESULTS = {"success", "skipped"}
ALL_GATE_RESULTS = ALLOWED_GATE_RESULTS | {"failure", "cancelled"}


class ClassificationError(RuntimeError):
    """A fail-closed classification error."""


@dataclass
class Selection:
    changed_paths: list[str]
    booleans: dict[str, bool] = field(
        default_factory=lambda: {name: False for name in sorted(KNOWN_BOOLEAN_EFFECTS)}
    )
    modes: dict[str, str] = field(
        default_factory=lambda: {
            "go_mode": "none",
            "web_mode": "none",
            "database_mode": "none",
        }
    )
    groups: dict[str, set[str]] = field(
        default_factory=lambda: {"go_groups": set(), "database_groups": set()}
    )
    matched_rules: set[str] = field(default_factory=set)
    fallback_reasons: set[str] = field(default_factory=set)

    def outputs(self) -> dict[str, str]:
        values: dict[str, str] = {
            "changed_count": str(len(self.changed_paths)),
            "changed_paths_json": json.dumps(self.changed_paths, ensure_ascii=False, separators=(",", ":")),
            "basic_only": _bool_text(not any(self.booleans.values())),
            "go_selected": _bool_text(self.booleans["go"] and not self.booleans["shared"]),
        }
        values.update({name: _bool_text(value) for name, value in self.booleans.items()})
        values.update(self.modes)
        values.update({name: ",".join(sorted(value)) for name, value in self.groups.items()})
        values["matched_rules"] = ",".join(sorted(self.matched_rules))
        values["fallback_reasons"] = ",".join(sorted(self.fallback_reasons))
        return values


def _bool_text(value: bool) -> str:
    return "true" if value else "false"


def _run_git(arguments: Sequence[str], repo_root: Path, *, binary: bool = False) -> bytes | str:
    try:
        completed = subprocess.run(
            ["git", "-C", str(repo_root), *arguments],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    except (OSError, subprocess.CalledProcessError) as exc:
        detail = ""
        if isinstance(exc, subprocess.CalledProcessError):
            detail = exc.stderr.decode("utf-8", "replace").strip()
        raise ClassificationError(f"git command failed: {' '.join(arguments)}{': ' + detail if detail else ''}") from exc
    if binary:
        return completed.stdout
    return completed.stdout.decode("utf-8", "strict")


def _verify_commit(sha: str, label: str, repo_root: Path) -> None:
    if not SHA_RE.fullmatch(sha):
        raise ClassificationError(f"{label} must be a full lowercase commit SHA")
    _run_git(["rev-parse", "--verify", "--quiet", f"{sha}^{{commit}}"], repo_root)


def _validate_repo_path(value: str) -> str:
    if not value or value.startswith(("/", "./")) or "\\" in value or "//" in value:
        raise ClassificationError(f"non-canonical changed path: {value!r}")
    if any(character in value for character in ("\x00", "\n", "\r", "\t")):
        raise ClassificationError("changed path contains a control character")
    segments = value.split("/")
    if any(segment in {"", ".", ".."} for segment in segments):
        raise ClassificationError(f"changed path contains traversal: {value!r}")
    return value


def changed_paths(base_sha: str, head_sha: str, repo_root: Path) -> list[str]:
    _verify_commit(base_sha, "base SHA", repo_root)
    _verify_commit(head_sha, "head SHA", repo_root)
    raw = _run_git(
        [
            "-c",
            "core.quotePath=false",
            "diff",
            "--name-status",
            "--no-renames",
            "--diff-filter=ACDMRTUXB",
            "-z",
            base_sha,
            head_sha,
            "--",
        ],
        repo_root,
        binary=True,
    )
    assert isinstance(raw, bytes)
    fields = raw.split(b"\x00")
    if fields and fields[-1] == b"":
        fields.pop()
    if len(fields) % 2:
        raise ClassificationError("git diff returned a malformed name-status stream")
    paths: set[str] = set()
    for offset in range(0, len(fields), 2):
        status = fields[offset].decode("ascii", "strict")
        if not re.fullmatch(r"[ACDMRTUXB]", status):
            raise ClassificationError(f"unsupported diff status: {status!r}")
        value = fields[offset + 1].decode("utf-8", "strict")
        paths.add(_validate_repo_path(value))
    if not paths:
        raise ClassificationError("base..head contains no changed files")
    return sorted(paths)


def _glob_regex(pattern: str) -> re.Pattern[str]:
    if not pattern or pattern.startswith(("/", "./")) or "\\" in pattern or "//" in pattern:
        raise ClassificationError(f"non-canonical map pattern: {pattern!r}")
    result = ["^"]
    position = 0
    while position < len(pattern):
        character = pattern[position]
        if character == "*":
            if position + 1 < len(pattern) and pattern[position + 1] == "*":
                position += 2
                if position < len(pattern) and pattern[position] == "/":
                    result.append("(?:.*/)?")
                    position += 1
                else:
                    result.append(".*")
                continue
            result.append("[^/]*")
        elif character == "?":
            result.append("[^/]")
        else:
            result.append(re.escape(character))
        position += 1
    result.append("$")
    return re.compile("".join(result))


@dataclass(frozen=True)
class Rule:
    name: str
    patterns: tuple[re.Pattern[str], ...]
    excludes: tuple[re.Pattern[str], ...]
    effects: dict[str, Any]

    def matches(self, repo_path: str) -> bool:
        return any(pattern.fullmatch(repo_path) for pattern in self.patterns) and not any(
            pattern.fullmatch(repo_path) for pattern in self.excludes
        )


@dataclass(frozen=True)
class CiMap:
    rules: tuple[Rule, ...]
    mode_order: dict[str, tuple[str, ...]]


def load_ci_map(map_path: Path) -> CiMap:
    if not map_path.is_file() or map_path.is_symlink():
        raise ClassificationError(f"CI map must be a regular file: {map_path}")
    try:
        source = json.loads(map_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise ClassificationError(f"CI map is not valid JSON-compatible YAML: {exc.__class__.__name__}") from exc
    if not isinstance(source, dict) or source.get("version") != 1:
        raise ClassificationError("CI map version must be 1")
    raw_mode_order = source.get("mode_order")
    expected_mode_order = {
        "go_mode": ["none", "selected", "full"],
        "web_mode": ["none", "build", "full"],
        "database_mode": ["none", "selected", "full"],
    }
    if raw_mode_order != expected_mode_order:
        raise ClassificationError("CI map mode_order is not canonical")
    mode_order = {key: tuple(values) for key, values in expected_mode_order.items()}
    raw_rules = source.get("rules")
    if not isinstance(raw_rules, list) or not raw_rules:
        raise ClassificationError("CI map must contain rules")
    names: set[str] = set()
    rules: list[Rule] = []
    ci_self_boundary = False
    for raw_rule in raw_rules:
        if not isinstance(raw_rule, dict) or set(raw_rule) - {"name", "patterns", "exclude", "effects"}:
            raise ClassificationError("CI map rule has unknown keys")
        name = raw_rule.get("name")
        if not isinstance(name, str) or not GROUP_RE.fullmatch(name) or name in names:
            raise ClassificationError(f"CI map rule name is invalid or duplicated: {name!r}")
        names.add(name)
        raw_patterns = raw_rule.get("patterns")
        raw_excludes = raw_rule.get("exclude", [])
        if not isinstance(raw_patterns, list) or not raw_patterns or not all(isinstance(item, str) for item in raw_patterns):
            raise ClassificationError(f"CI map rule patterns are invalid: {name}")
        if not isinstance(raw_excludes, list) or not all(isinstance(item, str) for item in raw_excludes):
            raise ClassificationError(f"CI map rule excludes are invalid: {name}")
        effects = raw_rule.get("effects")
        if not isinstance(effects, dict):
            raise ClassificationError(f"CI map rule effects are invalid: {name}")
        if name == "ci-self":
            required_patterns = {".github/**", "scripts/ci/**"}
            ci_self_boundary = required_patterns.issubset(set(raw_patterns)) and effects.get("ci") is True
        unknown_effects = set(effects) - KNOWN_BOOLEAN_EFFECTS - KNOWN_LIST_EFFECTS - KNOWN_MODE_EFFECTS
        if unknown_effects:
            raise ClassificationError(f"CI map rule has unknown effects: {name}: {sorted(unknown_effects)}")
        for key, value in effects.items():
            if key in KNOWN_BOOLEAN_EFFECTS and not isinstance(value, bool):
                raise ClassificationError(f"CI map boolean effect is invalid: {name}:{key}")
            if key in KNOWN_LIST_EFFECTS:
                if not isinstance(value, list) or not all(isinstance(item, str) and GROUP_RE.fullmatch(item) for item in value):
                    raise ClassificationError(f"CI map group effect is invalid: {name}:{key}")
            if key in KNOWN_MODE_EFFECTS and value not in mode_order[key]:
                raise ClassificationError(f"CI map mode effect is invalid: {name}:{key}")
        semantic_requirements = {
            "go_mode": "go",
            "go_groups": "go",
            "web_mode": "web",
            "web_audit": "web",
            "database_mode": "database",
            "database_groups": "database",
            "sqlc": "database",
            "shared": "go",
            "vulnerability": "go",
        }
        for dependent, prerequisite in semantic_requirements.items():
            if dependent in effects and effects[dependent] not in (False, [], "none") and effects.get(prerequisite) is not True:
                raise ClassificationError(
                    f"CI map effect {dependent} requires {prerequisite}=true: {name}"
                )
        rules.append(
            Rule(
                name=name,
                patterns=tuple(_glob_regex(item) for item in raw_patterns),
                excludes=tuple(_glob_regex(item) for item in raw_excludes),
                effects=effects,
            )
        )
    if not ci_self_boundary:
        raise ClassificationError("CI map lost the mandatory self-test boundary")
    return CiMap(rules=tuple(rules), mode_order=mode_order)


def _merge_mode(current: str, incoming: str, order: tuple[str, ...]) -> str:
    return incoming if order.index(incoming) > order.index(current) else current


def classify(repo_paths: Iterable[str], ci_map: CiMap) -> Selection:
    canonical_paths = sorted({_validate_repo_path(value) for value in repo_paths})
    if not canonical_paths:
        raise ClassificationError("classification requires at least one changed path")
    selection = Selection(changed_paths=canonical_paths)
    for repo_path in canonical_paths:
        path_effects: set[str] = set()
        for rule in ci_map.rules:
            if not rule.matches(repo_path):
                continue
            selection.matched_rules.add(rule.name)
            path_effects.update(rule.effects)
            for key, value in rule.effects.items():
                if key in KNOWN_BOOLEAN_EFFECTS:
                    selection.booleans[key] = selection.booleans[key] or value
                elif key in KNOWN_LIST_EFFECTS:
                    selection.groups[key].update(value)
                elif key in KNOWN_MODE_EFFECTS:
                    selection.modes[key] = _merge_mode(selection.modes[key], value, ci_map.mode_order[key])
        suffix = Path(repo_path).suffix.lower()
        if suffix == ".go" and not ({"go", "go_mode"} & path_effects):
            selection.booleans["go"] = True
            selection.booleans["shared"] = True
            selection.modes["go_mode"] = "full"
            selection.fallback_reasons.add("unknown-go")
        if suffix == ".sql" and not ({"database", "database_mode"} & path_effects):
            selection.booleans["database"] = True
            selection.booleans["sqlc"] = True
            selection.modes["database_mode"] = "full"
            selection.fallback_reasons.add("unknown-sql")
        if suffix in WEB_SUFFIXES and not ({"web", "web_mode"} & path_effects):
            selection.booleans["web"] = True
            selection.modes["web_mode"] = "full"
            selection.fallback_reasons.add("unknown-web")
    if selection.booleans["go"] and selection.modes["go_mode"] == "none":
        selection.modes["go_mode"] = "selected"
    if selection.booleans["web"] and selection.modes["web_mode"] == "none":
        selection.modes["web_mode"] = "full"
    if selection.booleans["database"] and selection.modes["database_mode"] == "none":
        selection.modes["database_mode"] = "full"
    if selection.booleans["shared"]:
        selection.booleans["go"] = True
        selection.modes["go_mode"] = "full"
    if selection.modes["go_mode"] == "full":
        selection.groups["go_groups"].clear()
    if selection.modes["database_mode"] == "full":
        selection.groups["database_groups"].clear()
    return selection


def evaluate_merge_gate(raw_results: Sequence[str]) -> tuple[bool, list[tuple[str, str]]]:
    if not raw_results:
        raise ClassificationError("merge gate requires job results")
    parsed: list[tuple[str, str]] = []
    seen_names: set[str] = set()
    for index, raw in enumerate(raw_results, start=1):
        if "=" in raw:
            name, result = raw.split("=", 1)
        else:
            name, result = f"job-{index}", raw
        if not GROUP_RE.fullmatch(name):
            raise ClassificationError(f"merge gate job name is invalid: {name!r}")
        if name in seen_names:
            raise ClassificationError(f"merge gate job name is duplicated: {name}")
        seen_names.add(name)
        if result not in ALL_GATE_RESULTS:
            raise ClassificationError(f"merge gate result is invalid for {name}: {result!r}")
        parsed.append((name, result))
    passed = all(result in ALLOWED_GATE_RESULTS for _, result in parsed)
    passed = passed and all(
        result == "success" for name, result in parsed if name in ALWAYS_GATE_JOBS
    )
    return passed, parsed


def _write_outputs(output_path: Path, values: dict[str, str]) -> None:
    if output_path.is_symlink():
        raise ClassificationError("GitHub output path must not be a symlink")
    try:
        with output_path.open("a", encoding="utf-8", newline="\n") as target:
            for key in sorted(values):
                value = values[key]
                if "\n" in value or "\r" in value:
                    raise ClassificationError(f"output contains a newline: {key}")
                target.write(f"{key}={value}\n")
    except OSError as exc:
        raise ClassificationError(f"cannot write GitHub outputs: {exc.__class__.__name__}") from exc


def _write_summary(summary_path: Path, base_sha: str, head_sha: str, selection: Selection) -> None:
    values = selection.outputs()
    jobs = [
        ("go", values["go"] == "true" and values["shared"] != "true"),
        ("web", values["web"] == "true"),
        ("api-codegen", values["api"] == "true"),
        ("database", values["database"] == "true"),
        ("shared-regression", values["shared"] == "true"),
        ("self-test", values["ci"] == "true"),
    ]
    selected = [name for name, enabled in jobs if enabled]
    skipped = [name for name, enabled in jobs if not enabled]
    lines = [
        "## CI relevance selection",
        "",
        f"- Base: `{base_sha}`",
        f"- Head: `{head_sha}`",
        f"- Changed files: `{len(selection.changed_paths)}`",
        f"- Matched rules: `{values['matched_rules'] or '-'}`",
        f"- Conservative fallbacks: `{values['fallback_reasons'] or '-'}`",
        "- Always: `classify`, `secret-diff`, `merge-gate`",
        f"- Selected: `{', '.join(selected) if selected else 'none'}`",
        f"- Skipped as unrelated: `{', '.join(skipped) if skipped else 'none'}`",
        "",
        "### Changed paths",
    ]
    lines.extend(f"- `{value}`" for value in selection.changed_paths)
    try:
        with summary_path.open("a", encoding="utf-8", newline="\n") as target:
            target.write("\n".join(lines) + "\n")
    except OSError as exc:
        raise ClassificationError(f"cannot write job summary: {exc.__class__.__name__}") from exc


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--base")
    mode.add_argument("--merge-gate", nargs="+")
    parser.add_argument("--head")
    parser.add_argument("--map", dest="map_path", default=".github/ci-map.yml")
    parser.add_argument("--repo", default=".")
    parser.add_argument("--github-output")
    parser.add_argument("--summary")
    parser.add_argument("--print-json", action="store_true")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    arguments = _parser().parse_args(argv)
    try:
        if arguments.merge_gate is not None:
            passed, parsed = evaluate_merge_gate(arguments.merge_gate)
            for name, result in parsed:
                print(f"merge-gate: {name}={result}")
            print(f"merge-gate: {'PASS' if passed else 'FAIL'}")
            return 0 if passed else 1
        if not arguments.head:
            raise ClassificationError("--head is required with --base")
        repo_root = Path(arguments.repo).resolve(strict=True)
        if not (repo_root / ".git").exists():
            _run_git(["rev-parse", "--show-toplevel"], repo_root)
        map_path = Path(arguments.map_path)
        if not map_path.is_absolute():
            map_path = repo_root / map_path
        ci_map = load_ci_map(map_path)
        paths = changed_paths(arguments.base, arguments.head, repo_root)
        selection = classify(paths, ci_map)
        values = selection.outputs()
        if arguments.github_output:
            _write_outputs(Path(arguments.github_output), values)
        if arguments.summary:
            _write_summary(Path(arguments.summary), arguments.base, arguments.head, selection)
        if arguments.print_json or not arguments.github_output:
            print(json.dumps(values, ensure_ascii=False, sort_keys=True, indent=2))
        return 0
    except (ClassificationError, OSError, UnicodeError, ValueError) as exc:
        print(f"ci-classifier: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
