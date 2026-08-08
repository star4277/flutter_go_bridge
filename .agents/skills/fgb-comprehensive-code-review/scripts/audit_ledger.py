#!/usr/bin/env python3
"""Create and verify a fingerprinted ledger for exhaustive repository review."""

from __future__ import annotations

import argparse
import csv
import hashlib
import subprocess
import sys
from pathlib import Path, PurePosixPath


SOURCE_SUFFIXES = {
    ".bash", ".bat", ".c", ".cc", ".cmd", ".cpp", ".cs", ".cxx", ".dart",
    ".ets", ".fish", ".go", ".gradle", ".h", ".hh", ".hpp", ".html",
    ".java", ".js", ".jsx", ".kt", ".kts", ".m", ".mjs", ".mm", ".php",
    ".podspec", ".proto", ".ps1", ".py", ".rb", ".rs", ".scss", ".sh",
    ".sql", ".swift", ".template", ".tmpl", ".ts", ".tsx", ".zig", ".zsh",
}

CONFIG_SUFFIXES = {
    ".cmake", ".json", ".json5", ".mod", ".toml", ".xml", ".yaml", ".yml",
}

EXACT_SOURCE_NAMES = {
    "CMakeLists.txt", "Dockerfile", "Makefile", "Rakefile", "build.gradle",
    "build.gradle.kts", "settings.gradle", "settings.gradle.kts",
}

EXCLUDED_NAMES = {
    "package-lock.json", "pnpm-lock.yaml", "pubspec.lock", "yarn.lock",
}

LEDGER_FIELDS = ("path", "fingerprint", "kind", "status", "findings_or_reason")


class LedgerError(RuntimeError):
    pass


def run_git(repo: Path, *args: str) -> bytes:
    completed = subprocess.run(
        ["git", "-C", str(repo), *args],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        detail = completed.stderr.decode("utf-8", errors="replace").strip()
        raise LedgerError(f"git {' '.join(args)} failed: {detail}")
    return completed.stdout


def classify(path: str, mode: str, absolute: Path) -> str | None:
    if mode == "160000":
        return "submodule"

    pure = PurePosixPath(path)
    name = pure.name
    lower_name = name.lower()
    suffix = pure.suffix.lower()

    if name in EXCLUDED_NAMES or lower_name.endswith(".lock") or lower_name.endswith(".sum"):
        return None

    if name in EXACT_SOURCE_NAMES or suffix in SOURCE_SUFFIXES:
        lowered_parts = [part.lower() for part in pure.parts]
        if "template" in lowered_parts or suffix in {".template", ".tmpl"}:
            return "template"
        if "generated" in lower_name or "generated" in lowered_parts:
            return "generated"
        if (
            "test" in lowered_parts
            or "tests" in lowered_parts
            or lower_name.endswith("_test.go")
            or lower_name.endswith("_test.dart")
            or ".test." in lower_name
        ):
            return "test"
        return "source"

    if suffix in CONFIG_SUFFIXES:
        return "build-config"

    try:
        if absolute.is_file():
            with absolute.open("rb") as handle:
                if handle.read(2) == b"#!":
                    return "script"
    except OSError as exc:
        raise LedgerError(f"cannot inspect {path}: {exc}") from exc

    return None


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    try:
        with path.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
    except OSError as exc:
        raise LedgerError(f"cannot hash {path}: {exc}") from exc
    return f"sha256:{digest.hexdigest()}"


def inventory(repo: Path) -> list[dict[str, str]]:
    repo = repo.resolve()
    records = run_git(repo, "ls-files", "--stage", "-z").decode("utf-8").split("\0")
    result: list[dict[str, str]] = []

    for record in records:
        if not record:
            continue
        try:
            metadata, path = record.split("\t", 1)
            mode, object_id, stage = metadata.split(" ", 2)
        except ValueError as exc:
            raise LedgerError(f"unexpected git ls-files record: {record!r}") from exc

        if stage != "0":
            raise LedgerError(f"unmerged index entry must be resolved before review: {path}")

        kind = classify(path, mode, repo / Path(path))
        if kind is None:
            continue

        fingerprint = f"gitlink:{object_id}" if mode == "160000" else sha256_file(repo / Path(path))
        result.append(
            {
                "path": path,
                "fingerprint": fingerprint,
                "kind": kind,
                "status": "pending",
                "findings_or_reason": "",
            }
        )

    result.sort(key=lambda row: row["path"])
    return result


def write_ledger(path: Path, rows: list[dict[str, str]], force: bool) -> None:
    if path.exists() and not force:
        raise LedgerError(f"ledger already exists: {path}; use --force only after preserving review evidence")
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=LEDGER_FIELDS, delimiter="\t")
        writer.writeheader()
        writer.writerows(rows)


def read_ledger(path: Path) -> list[dict[str, str]]:
    try:
        with path.open("r", encoding="utf-8", newline="") as handle:
            reader = csv.DictReader(handle, delimiter="\t")
            if tuple(reader.fieldnames or ()) != LEDGER_FIELDS:
                raise LedgerError(
                    f"ledger header must be exactly: {' | '.join(LEDGER_FIELDS)}"
                )
            return [dict(row) for row in reader]
    except OSError as exc:
        raise LedgerError(f"cannot read ledger {path}: {exc}") from exc


def verify(repo: Path, ledger: Path) -> None:
    current_rows = inventory(repo)
    current = {row["path"]: row for row in current_rows}
    reviewed_rows = read_ledger(ledger)
    reviewed: dict[str, dict[str, str]] = {}
    problems: list[str] = []

    for row in reviewed_rows:
        path = row["path"]
        if path in reviewed:
            problems.append(f"duplicate ledger row: {path}")
        reviewed[path] = row

    for path in sorted(current.keys() - reviewed.keys()):
        problems.append(f"missing ledger row: {path}")
    for path in sorted(reviewed.keys() - current.keys()):
        problems.append(f"extra/stale ledger row: {path}")

    counts: dict[str, int] = {}
    for path in sorted(current.keys() & reviewed.keys()):
        expected = current[path]
        actual = reviewed[path]
        counts[expected["kind"]] = counts.get(expected["kind"], 0) + 1
        if actual["fingerprint"] != expected["fingerprint"]:
            problems.append(f"changed after review: {path}")
        if actual["kind"] != expected["kind"]:
            problems.append(
                f"kind mismatch for {path}: ledger={actual['kind']} current={expected['kind']}"
            )
        if actual["status"].strip().lower() != "reviewed":
            problems.append(f"not reviewed: {path} (status={actual['status']!r})")
        evidence = actual["findings_or_reason"].strip()
        if not evidence or evidence.lower() in {"todo", "pending", "tbd"}:
            problems.append(f"missing finding IDs or explicit 'none': {path}")

    if problems:
        preview = "\n".join(f"- {problem}" for problem in problems[:100])
        remainder = "" if len(problems) <= 100 else f"\n- ... {len(problems) - 100} more"
        raise LedgerError(f"ledger verification failed ({len(problems)} problems):\n{preview}{remainder}")

    summary = ", ".join(f"{kind}={count}" for kind, count in sorted(counts.items()))
    print(f"PASS: {len(current_rows)} auditable entries verified ({summary})")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    init_parser = subparsers.add_parser("init", help="create a pending audit ledger")
    init_parser.add_argument("--repo", type=Path, default=Path("."))
    init_parser.add_argument("--ledger", type=Path, required=True)
    init_parser.add_argument("--force", action="store_true")

    verify_parser = subparsers.add_parser("verify", help="verify coverage and fingerprints")
    verify_parser.add_argument("--repo", type=Path, default=Path("."))
    verify_parser.add_argument("--ledger", type=Path, required=True)

    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        if args.command == "init":
            rows = inventory(args.repo)
            write_ledger(args.ledger, rows, args.force)
            print(f"created {args.ledger} with {len(rows)} pending auditable entries")
        else:
            verify(args.repo, args.ledger)
    except LedgerError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
