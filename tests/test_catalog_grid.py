# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""Grid expansion: the compressed hole notation in the shadow library.

`expand` takes a Snap straight out of the shadow library and unfolds its grid
into individual positions. A Snap is plain data, so this needs no download.
"""
import numpy as np
import pytest
from brickmesh_extract.catalog import CatalogEntry, expand, parse_grid, tier_of
from brickmesh_extract.snap import Snap


def snap(grid: str = "", pos=(0.0, 0.0, 0.0), ori=None, **kw) -> Snap:
    return Snap(
        kind=kw.pop("kind", "SNAP_CYL"),
        gender=kw.pop("gender", "F"),
        pos=np.array(pos, float),
        ori=np.eye(3) if ori is None else np.asarray(ori, float),
        grid=grid,
        **kw,
    )


def test_parse_grid_reads_counts_spacings_and_centering():
    # 'C 3 1 20 0': three positions, centered, 20 LDU apart on the first axis.
    assert parse_grid("C 3 1 20 0") == (3, 1, 20.0, 0.0, True, False)
    assert parse_grid("2 2 20 20") == (2, 2, 20.0, 20.0, False, False)
    assert parse_grid("3 C 2 20 10") == (3, 2, 20.0, 10.0, False, True)


def test_parse_grid_defaults_to_a_single_position():
    assert parse_grid("") == (1, 1, 0.0, 0.0, False, False)


def test_expand_without_a_grid_gives_one_position():
    out = expand(snap(pos=(10.0, 0.0, 0.0)))
    assert len(out) == 1
    assert np.allclose(out[0], [10.0, 0.0, 0.0])


def test_centered_grid_straddles_the_origin():
    # Three holes, 20 apart, centered: -20, 0, +20 along local X.
    out = expand(snap(grid="C 3 1 20 0"))
    xs = sorted(float(p[0]) for p in out)
    assert xs == [-20.0, 0.0, 20.0]
    assert all(np.isclose(p[1], 0) and np.isclose(p[2], 0) for p in out)


def test_uncentered_grid_counts_up_from_the_snap():
    out = expand(snap(grid="3 1 20 0"))
    assert sorted(float(p[0]) for p in out) == [0.0, 20.0, 40.0]


def test_two_dimensional_grid_expands_both_axes():
    out = expand(snap(grid="2 2 20 20"))
    assert len(out) == 4
    corners = {(float(p[0]), float(p[2])) for p in out}
    assert corners == {(0.0, 0.0), (20.0, 0.0), (0.0, 20.0), (20.0, 20.0)}


def test_grid_follows_the_snap_orientation():
    """The grid lies in the snap's own frame. With the snap rotated 90 degrees
    about Z, its local X runs along world Y, and the row has to follow."""
    rot_z90 = np.array([[0.0, -1.0, 0.0], [1.0, 0.0, 0.0], [0.0, 0.0, 1.0]])
    out = expand(snap(grid="C 3 1 20 0", ori=rot_z90))
    ys = sorted(float(p[1]) for p in out)
    assert ys == [-20.0, 0.0, 20.0]
    assert all(np.isclose(p[0], 0.0) for p in out)


def test_grid_is_offset_by_the_snap_position():
    out = expand(snap(grid="C 3 1 20 0", pos=(0.0, 8.0, 0.0)))
    assert all(np.isclose(p[1], 8.0) for p in out)


def test_snap_axis_points_along_local_y_before_rotation():
    # LDCad snap cylinders point along +Y; the ori matrix turns them.
    assert np.allclose(snap().axis, [0.0, 1.0, 0.0])
    rot_x90 = np.array([[1.0, 0.0, 0.0], [0.0, 0.0, -1.0], [0.0, 1.0, 0.0]])
    assert np.allclose(snap(ori=rot_x90).axis, [0.0, 0.0, 1.0])


def test_grouped_snaps_are_not_generic():
    """A [group=...] snap is a crane-arm slot or a hinge, not a plain hole.
    Treating it as one gives a liftarm a phantom port along its whole length."""
    assert snap().is_generic
    assert not snap(group="craneArm").is_generic


def test_axle_snaps_are_recognized_by_their_section():
    assert snap(secs="A 8 4").is_axle
    assert not snap(secs="R 8 4").is_axle


def test_catalog_entry_counts_distinct_hole_directions():
    """More than one direction means a perpendicular connector."""
    straight = CatalogEntry(
        part="beam",
        holes=[[0, 0, 0, 0, 1, 0, 0], [20, 0, 0, 0, 1, 0, 0]],
        pins=[], axle_holes=0)
    assert straight.axes == 1
    assert straight.n_holes == 2

    corner = CatalogEntry(
        part="angle",
        holes=[[0, 0, 0, 0, 1, 0, 0], [0, 0, 0, 1, 0, 0, 0]],
        pins=[], axle_holes=0)
    assert corner.axes == 2


def test_opposite_axes_count_as_one_direction():
    # +Y and -Y are the same hole direction.
    e = CatalogEntry(part="x", holes=[[0, 0, 0, 0, 1, 0, 0], [0, 0, 0, 0, -1, 0, 0]],
                     pins=[], axle_holes=0)
    assert e.axes == 1


@pytest.mark.parametrize(
    "title, tier",
    [
        ("Technic Beam 3", 1),
        ("Technic Pin with Friction Ridges", 1),
        ("Technic Bush", 1),
        ("Technic Panel Fairing #1", 2),
        ("Brick 2 x 4", 3),
        ("Horse Barding", 3),
    ],
)
def test_tier_of_prefers_common_technic_parts(title, tier):
    assert tier_of(title) == tier
