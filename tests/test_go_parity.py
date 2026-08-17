# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The Go extractor against the Python one, part by part.

While the port is in progress both extractors exist, and the only useful
statement about the new one is that it agrees with the old one over the whole
library — not over a fixture, over all 2,649 parts and 21,675 ports. That is
what makes the port checkable rather than a matter of faith, and it is how the
three-axis grid bug was found: Python raised on those specs and build() swallowed
the exception, so eight parts vanished from the catalog without a word.

Needs the real libraries, so it runs with BRICKMESH_LIBRARIES=1.
"""
import json
import shutil
import subprocess
from pathlib import Path

import pytest
from brickmesh_extract import build

pytestmark = pytest.mark.libraries

REPO = Path(__file__).resolve().parents[1]


@pytest.fixture(scope="module")
def go_records(tmp_path_factory) -> list:
    if shutil.which("go") is None:
        pytest.skip("no go toolchain")
    out = tmp_path_factory.mktemp("parity") / "go-catalog.json"
    proc = subprocess.run(
        ["go", "run", "./cmd/brickmesh-extract", "--out", str(out), "--tier", "3"],
        cwd=REPO, capture_output=True, text=True, timeout=900,
    )
    if proc.returncode != 0:
        pytest.fail(f"go extractor failed:\n{proc.stderr}")
    return json.loads(out.read_text())


@pytest.fixture(scope="module")
def py_records() -> list:
    return build.build(max_tier=3)


def by_id(records) -> dict:
    return {r["id"]: r for r in records}


def test_both_extractors_find_the_same_parts(go_records, py_records):
    go, py = by_id(go_records), by_id(py_records)
    assert set(go) == set(py), (
        f"only in go: {sorted(set(go) - set(py))[:10]}; "
        f"only in python: {sorted(set(py) - set(go))[:10]}")


def test_every_record_matches(go_records, py_records):
    go, py = by_id(go_records), by_id(py_records)
    mismatched = [pid for pid in sorted(go) if go[pid] != py[pid]]
    assert not mismatched, f"{len(mismatched)} differ, e.g. {mismatched[:5]}"


def test_the_catalog_is_not_trivially_small(go_records):
    """Guards the comparison itself: two empty catalogs also match."""
    assert len(go_records) > 2000
    ports = sum(len(r["holes"]) + len(r["pins"]) for r in go_records)
    assert ports > 20000


def test_ordering_matches_too(go_records, py_records):
    """Both sort by part id, so the files are comparable directly and a diff of
    the two outputs is readable."""
    assert [r["id"] for r in go_records] == [r["id"] for r in py_records]


def test_the_parts_that_used_to_vanish_are_present(go_records, py_records):
    """
    These eight declare a three-axis grid. Python's parser raised on it and the
    part was dropped whole, taking its valid snaps with it; the Go port scored
    them as a zero-spacing grid and invented duplicate ports. Both now keep the
    stated position and drop the unverified repeats.
    """
    recovered = {"11299", "23444", "32308", "35366", "42936", "4611", "47296", "92910"}
    for records in (by_id(go_records), by_id(py_records)):
        missing = recovered - set(records)
        assert not missing, f"still dropping {sorted(missing)}"
