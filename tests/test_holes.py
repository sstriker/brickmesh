# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The hole probe: find holes by pushing an axle through the part.

Needs a collision library, so it runs only with the `geometry` extra
installed — `uv sync --extra geometry`.
"""
import pytest

pytest.importorskip("trimesh", reason="needs the geometry extra")
pytest.importorskip("fcl", reason="needs the geometry extra")

from brickmesh_extract import holes

OUTER = 20.0        # the fixture tube is 40 LDU across
BORE = 6.0          # with a 12 LDU bore


def centers(part: str, axis: str = "x"):
    return holes.cluster(holes.find_holes(part, axis=axis, step=2.0))


def test_the_probe_finds_the_bore(parts):
    found = centers("fixtube")
    assert any(abs(y) < 1.0 and abs(z) < 1.0 for y, z in found), \
        f"the bore through the middle was not found: {found}"


def test_a_solid_part_has_no_hole_through_its_middle(parts):
    found = centers("fixcube")
    assert not any(abs(y) < 1.0 and abs(z) < 1.0 for y, z in found), \
        f"a solid cube reported a hole at its center: {found}"


def test_the_probe_also_reports_corner_artifacts(parts):
    """
    Documented limitation, not an accident. Just outside a corner the thin
    probe misses the part while the thick probe still clips it, which is the
    same signature as a hole. Every such hit lies outside the cross-section, so
    a caller can discard them on position; nothing in find_holes does.
    """
    for y, z in centers("fixcube"):
        assert abs(y) > OUTER and abs(z) > OUTER, \
            f"unexpected hit inside a solid part at ({y}, {z})"


def test_probe_radii_bracket_the_bore(parts):
    """
    The two radii are what make the test work: a real axle exactly fills its
    hole, so the thin probe has to be thinner than the bore, and the thick one
    has to be too fat for it.
    """
    assert holes.R_THIN < BORE < holes.R_THICK


def test_clustering_merges_neighboring_hits():
    hits = [(0.0, 0.0), (2.0, 0.0), (0.0, 2.0), (40.0, 40.0)]
    merged = holes.cluster(hits, tol=6.0)
    assert len(merged) == 2
    assert (40.0, 40.0) in merged


def test_clustering_keeps_distinct_holes_apart():
    hits = [(-20.0, 0.0), (0.0, 0.0), (20.0, 0.0)]
    assert len(holes.cluster(hits, tol=6.0)) == 3
