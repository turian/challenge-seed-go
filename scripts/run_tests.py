#!/usr/bin/env python3
"""
SQLLogicTest runner for the SQL Vibe Coding Challenge.

This script runs SQLLogicTest files against your database implementation
and reports pass/fail statistics.
"""

import argparse
import json
import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Iterable, Tuple

# Path to the SQLLogicTest test files
SCRIPT_DIR = Path(__file__).parent
PROJECT_ROOT = SCRIPT_DIR.parent
TEST_DIR = PROJECT_ROOT / "third_party" / "sqllogictest" / "test"
DB_BINARY = PROJECT_ROOT / "sql-challenge"


def run_test_file(test_file: Path, timeout: int) -> Tuple[bool, str]:
    """Run a single test file and return (passed, error_message)."""
    try:
        result = subprocess.run(
            [str(DB_BINARY), str(test_file)],
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        # For now, consider any non-error exit as a pass
        # You'll want to implement proper result checking
        if result.returncode == 0:
            return True, ""
        else:
            return False, result.stderr or result.stdout
    except subprocess.TimeoutExpired:
        return False, "Timeout"
    except Exception as e:
        return False, str(e)


def find_test_files() -> list[Path]:
    """Find all .test files in the test directory."""
    if not TEST_DIR.exists():
        print(f"Error: Test directory not found: {TEST_DIR}")
        print("Make sure you cloned with --recurse-submodules")
        sys.exit(1)

    test_files = []
    for root, _, files in os.walk(TEST_DIR):
        for file in files:
            if file.endswith(".test"):
                test_files.append(Path(root) / file)

    return sorted(test_files)


def resolve_test_file(file_arg: str) -> Path:
    candidate = Path(file_arg)
    if not candidate.is_absolute():
        candidate = PROJECT_ROOT / candidate
    if not candidate.exists():
        print(f"Error: Test file not found: {candidate}")
        sys.exit(1)
    return candidate


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run SQLLogicTest files")
    parser.add_argument(
        "--file",
        dest="file",
        help="Relative path to a specific .test file to run",
    )
    parser.add_argument(
        "--jobs",
        type=int,
        default=1,
        help="Number of parallel jobs to run (default: 1)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=60,
        help="Per-file timeout in seconds (default: 60)",
    )
    parser.add_argument(
        "--stop-after",
        type=int,
        default=0,
        help="Stop after N failures (default: 0 = no limit)",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Emit machine-readable JSON summary to stdout",
    )
    parser.add_argument(
        "--summary",
        action="store_true",
        help="Only print summary (no per-file output)",
    )
    return parser.parse_args()


def run_tests(
    test_files: Iterable[Path],
    *,
    jobs: int,
    timeout: int,
    stop_after: int,
    summary_only: bool,
    json_mode: bool,
) -> Tuple[int, int, list[dict]]:
    passed = 0
    failed = 0
    failures: list[dict] = []

    test_iter = iter(test_files)
    jobs = max(1, jobs)
    stop_after = max(0, stop_after)

    with ThreadPoolExecutor(max_workers=jobs) as executor:
        futures = {}

        def submit_next():
            try:
                path = next(test_iter)
            except StopIteration:
                return False
            futures[executor.submit(run_test_file, path, timeout)] = path
            return True

        for _ in range(jobs):
            if not submit_next():
                break

        while futures:
            for future in as_completed(list(futures)):
                test_file = futures.pop(future)
                success, error = future.result()
                try:
                    relative_path = test_file.relative_to(PROJECT_ROOT)
                except ValueError:
                    relative_path = test_file

                if success:
                    passed += 1
                    if not (summary_only or json_mode):
                        print(f"PASS: {relative_path}")
                else:
                    failed += 1
                    failures.append(
                        {"file": str(relative_path), "output": (error or "")}
                    )
                    if not (summary_only or json_mode):
                        print(f"FAIL: {relative_path}")
                        if error:
                            print(f"      {error[:100]}")

                if stop_after and failed >= stop_after:
                    for pending in futures:
                        pending.cancel()
                    futures.clear()
                    return passed, failed, failures

                submit_next()

    return passed, failed, failures


def main():
    args = parse_args()

    # Check if binary exists
    if not DB_BINARY.exists():
        print(f"Error: Database binary not found: {DB_BINARY}")
        print("Run 'make build' first")
        sys.exit(1)

    if args.file:
        test_files = [resolve_test_file(args.file)]
    else:
        test_files = find_test_files()

    if not args.json:
        print(f"Found {len(test_files)} test files")
        print()

    passed, failed, failures = run_tests(
        test_files,
        jobs=args.jobs,
        timeout=args.timeout,
        stop_after=args.stop_after,
        summary_only=args.summary,
        json_mode=args.json,
    )

    # Print summary
    total = passed + failed
    percentage = (passed / total * 100) if total > 0 else 0

    if args.json:
        print(
            json.dumps(
                {
                    "files_passed": passed,
                    "files_failed": failed,
                    "failures": failures,
                }
            )
        )
    else:
        print()
        print("=" * 50)
        print(f"Results: {passed}/{total} files passed ({percentage:.1f}%)")
        print("=" * 50)

    # Exit with error if not all tests passed
    sys.exit(0 if failed == 0 else 1)


if __name__ == "__main__":
    main()
