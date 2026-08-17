# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""Reading .dat files: titles, geometry resolution, and subfile expansion."""
import numpy as np
import pytest
from brickmesh_extract import core, ldraw


def test_title_comes_from_the_first_comment_line(parts):
    assert ldraw.geometry("fixcube").title == "Test Cube 40 LDU"
    # "0 Name:" and "0 !" lines are metadata, not the title.
    assert ldraw.geometry("fixbeam").title == "Test Beam 5 x 1 x 1"


def test_name_gets_the_dat_suffix_and_is_lowercased(parts):
    assert ldraw.geometry("FixCube").name == "fixcube.dat"
    assert ldraw.geometry("fixcube.dat").name == "fixcube.dat"


def test_geometry_is_cached_per_part(parts):
    assert ldraw.geometry("fixcube") is ldraw.geometry("fixcube")


def test_bbox_and_size(parts):
    g = ldraw.geometry("fixbeam")
    lo, hi = g.bbox
    assert np.allclose(lo, [-50, -10, -10])
    assert np.allclose(hi, [50, 10, 10])
    assert np.allclose(g.size, [100, 20, 20])


def test_quads_become_two_triangles_each(parts):
    # Six faces, one quad each, split on the diagonal.
    assert len(ldraw.geometry("fixcube").tris) == 12


def test_unknown_part_raises_rather_than_guessing(parts):
    with pytest.raises(ldraw.PartNotFound):
        ldraw.geometry("nosuchpart")


def test_repeated_subfiles_are_not_deduplicated(parts):
    """
    fixpair references the same cube twice, 80 LDU apart. Collapsing repeated
    references would leave one cube: the exact failure _resolve warns about,
    where a gear built from one tooth primitive instantiated N times shrinks to
    a sliver.
    """
    pair = ldraw.geometry("fixpair")
    assert np.allclose(pair.size, [120, 40, 40]), "the second cube went missing"
    assert len(pair.tris) == 2 * len(ldraw.geometry("fixcube").tris)


def test_subfile_transforms_are_applied(parts):
    xs = sorted({round(float(x), 1) for x in ldraw.geometry("fixpair").verts[:, 0]})
    assert xs == [-60.0, -20.0, 20.0, 60.0]


def test_thin_axis_is_the_shortest_dimension(parts):
    assert ldraw.geometry("fixdisc").thin_axis == 2      # 40 x 40 x 8
    assert ldraw.geometry("fixbeam").thin_axis == 1      # 100 x 20 x 20


def test_native_axis_of_a_disc_is_its_short_axis(parts):
    # Two equal large dimensions and one small: a gear or a bush.
    idx, confidence = core.infer_native_axis(ldraw.geometry("fixdisc"))
    assert (idx, confidence) == (2, "high")


def test_native_axis_of_a_rod_is_its_long_axis(parts):
    # Two equal small dimensions and one large: an axle.
    idx, confidence = core.infer_native_axis(ldraw.geometry("fixbeam"))
    assert (idx, confidence) == (0, "high")


def test_a_cube_gives_no_confident_axis(parts):
    """Nothing distinguishes the axes, so the heuristic says so rather than
    picking one and sounding sure about it."""
    _, confidence = core.infer_native_axis(ldraw.geometry("fixcube"))
    assert confidence == "check"


def test_gear_plane_finds_the_widest_feature(parts):
    # The tube is uniform along X, so the widest cross-section can be anywhere
    # along it; what matters is that it stays inside the part.
    x = core.gear_plane(ldraw.geometry("fixtube"), axis=0)
    assert -50 <= x <= 50


def test_radial_profile_tracks_the_cross_section(parts):
    prof = core.radial_profile(ldraw.geometry("fixtube"), axis=0, bins=8)
    assert prof
    # Every sample is a corner-to-axis distance of a 40x40 section at most.
    for _, radius in prof:
        assert 0 < radius <= np.hypot(20, 20) + 1e-9
