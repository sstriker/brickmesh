# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The occupancy grid.

The claim the whole coarse collision filter rests on is that it rasterizes the
SURFACE and not the volume, so a Technic hole stays a hole and an axle can run
through it without counting as a collision. The fixture tube has a 12 LDU bore
through a 40 x 40 section, which is exactly that case.
"""
import numpy as np
from brickmesh_extract import voxel


def cells_of(part: str, rot: int = 0) -> set:
    return {tuple(c) for c in voxel.voxels(part, rot).tolist()}


def test_a_solid_box_is_hollow_in_the_grid(parts):
    """Surface, not volume: the middle of a solid cube is not occupied."""
    cells = cells_of("fixcube")
    assert (0, 0, 0) not in cells
    # ... while its faces are. The cube spans -20..20, so cell 4 is the +X face.
    assert (4, 0, 0) in cells


def test_the_grid_spans_the_part(parts):
    c = voxel.voxels("fixcube", 0)
    assert c.min(axis=0).tolist() == [-4, -4, -4]
    assert c.max(axis=0).tolist() == [4, 4, 4]


def test_the_bore_stays_open(parts):
    """
    The four cells inside the 12 LDU bore must be empty. If the voxelizer
    filled volumes, every hole would silt up and the search would never find a
    bearing anywhere.
    """
    section = {(y, z) for (x, y, z) in cells_of("fixtube") if x == 0}
    for cell in ((0, 0), (0, -1), (-1, 0), (-1, -1)):
        assert cell not in section, f"the bore is blocked at {cell}"


def test_the_bore_walls_are_occupied(parts):
    # The bore surface sits at +/-6 LDU, which lands in cells 1 and -2.
    section = {(y, z) for (x, y, z) in cells_of("fixtube") if x == 0}
    assert (1, 0) in section and (-2, 0) in section
    # And the outer skin at +/-20 is there too.
    assert (4, 4) in section


def test_pitch_is_four_cells_to_the_stud(parts):
    assert voxel.STUD / voxel.PITCH == 4


def test_symmetry_reduces_the_orientations_to_try(parts):
    """
    24 lattice rotations, but symmetric parts repeat. Fewer orientations is
    directly less search space.

    Note the reduction is measured on the rasterized cells, not on exact
    symmetry, so a cube does not collapse all the way to one: sampling on the
    grid boundary differs slightly between orientations. It is a search-space
    optimization, not a claim about the part.
    """
    for part in ("fixcube", "fixbeam", "fixdisc", "fixtube"):
        keep = voxel.axis_aligned_rotations(part)
        assert 1 <= len(keep) < len(voxel.ROTATIONS)
        assert len(set(keep)) == len(keep)


def test_a_long_part_has_fewer_distinct_orientations_than_a_flat_one(parts):
    # A beam is symmetric about its length; a disc is not symmetric in the same
    # way, so it keeps more distinct orientations.
    assert len(voxel.axis_aligned_rotations("fixbeam")) < \
        len(voxel.axis_aligned_rotations("fixdisc"))


def test_rotations_are_the_24_proper_ones():
    assert len(voxel.ROTATIONS) == 24
    for m in voxel.ROTATIONS:
        assert np.isclose(np.linalg.det(m), 1.0), "a reflection is not buildable"


def test_occupancy_add_collide_and_remove(parts):
    occ = voxel.Occupancy.around(120.0)
    cells = voxel.voxels("fixcube", 0)

    assert not occ.collides(cells, [0, 0, 0])
    occ.add(cells, [0, 0, 0])
    assert occ.filled > 0
    assert occ.collides(cells, [0, 0, 0])

    # Far enough away to clear it.
    assert not occ.collides(cells, [100, 0, 0])

    occ.remove(cells, [0, 0, 0])
    assert occ.filled == 0
    assert not occ.collides(cells, [0, 0, 0])


def test_flat_indices_round_trip(parts):
    """The search tests the same placement many times, so it precomputes flat
    indices once and reuses them."""
    occ = voxel.Occupancy.around(120.0)
    cells = voxel.voxels("fixcube", 0)
    flat = occ.flat(cells, [0, 0, 0])
    assert len(flat) > 0

    assert not occ.collides_flat(flat)
    occ.add_flat(flat)
    assert occ.collides_flat(flat)
    occ.remove_flat(flat)
    assert not occ.collides_flat(flat)


def test_an_axle_passes_through_the_bore_without_colliding(parts):
    """
    The point of all of it. A thin part laid through the tube's bore must not
    register as a collision, or no bearing is ever found.
    """
    occ = voxel.Occupancy.around(200.0)
    occ.add(voxel.voxels("fixtube", 0), [0, 0, 0])

    # A cube is 40 LDU wide and cannot fit through a 12 LDU bore.
    assert occ.collides(voxel.voxels("fixcube", 0), [0, 0, 0])

    # But the empty bore cells themselves are free.
    bore = np.array([[x, 0, 0] for x in range(-8, 9)], dtype=np.int32)
    assert not occ.collides(bore, [0, 0, 0])


def test_lattice_positions_are_on_the_stud_grid(parts):
    pts = voxel.lattice_positions(1)
    assert len(pts) == 27                       # 3 x 3 x 3
    for p in pts:
        assert all(abs(v) in (0.0, voxel.STUD) for v in p)
