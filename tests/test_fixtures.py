# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The fixtures themselves.

They are generated, so the committed files and the generator have to agree, and
the meshes have to be real closed solids — a fixture that is quietly
non-manifold would make the tests built on it meaningless.
"""
import pytest
from brickmesh_extract import ldraw
from fixtures import generate_fixtures as gen


def test_committed_parts_match_the_generator():
    for name, text in gen.ldraw_files().items():
        on_disk = (gen.LDRAW_DIR / name).read_text(encoding="utf-8")
        assert on_disk == text, f"{name} is stale; rerun generate_fixtures.py"


def test_committed_shadow_files_match_the_generator():
    for name, text in gen.shadow_files().items():
        on_disk = (gen.SHADOW_DIR / name).read_text(encoding="utf-8")
        assert on_disk == text, f"{name} is stale; rerun generate_fixtures.py"


def test_fixtures_are_not_ldraw_parts():
    """
    The repository ships neither library on purpose, and the shadow library's
    share-alike condition would carry over to anything derived from it. These
    are ours, and they say so.
    """
    for name in gen.ldraw_files():
        text = (gen.LDRAW_DIR / name).read_text(encoding="utf-8")
        assert "NOT an LDraw part" in text
        assert "Apache-2.0" in text


@pytest.mark.parametrize(
    "part, volume",
    [
        ("fixcube", 40 * 40 * 40),
        ("fixbeam", 100 * 20 * 20),
        ("fixdisc", 40 * 40 * 8),
        ("fixtube", 100 * (40 * 40 - 12 * 12)),   # the bore is really open
        ("fixpair", 2 * 40 * 40 * 40),            # two cubes, not one
    ],
)
def test_fixture_meshes_are_closed_solids(parts, part, volume):
    pytest.importorskip("trimesh", reason="needs the geometry extra")
    mesh = ldraw.geometry(part).mesh()
    assert mesh.is_watertight, f"{part} is not closed"
    assert mesh.is_winding_consistent, f"{part} has mixed winding"
    assert mesh.volume == pytest.approx(volume)
