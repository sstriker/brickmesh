# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Reading the shadow library, and turning it into catalog entries.

This is the layer that says where the holes and pins are, so it decides what
the search is allowed to build with. The fixture beam carries one grid of five
hole positions, one axle hole, one pin, and one grouped snap that must not be
mistaken for a port.
"""
import numpy as np
from brickmesh_extract import catalog, snap


def test_snap_lines_are_parsed(shadow):
    snaps = snap.snaps("fixbeam")
    assert len(snaps) == 4
    kinds = {s.kind for s in snaps}
    assert kinds == {"SNAP_CYL"}


def test_gender_and_section_are_read(shadow):
    by_secs = {s.secs.split()[0]: s for s in snap.snaps("fixbeam")}
    assert by_secs["A"].is_axle, "an A section is an axle hole"
    assert not by_secs["R"].is_axle
    assert {s.gender for s in snap.snaps("fixbeam")} == {"F", "M"}


def test_axis_comes_from_the_orientation_matrix(shadow):
    """Snap cylinders point along +Y locally; the ori matrix turns them."""
    grid_snap = next(s for s in snap.snaps("fixbeam") if s.grid)
    assert np.allclose(grid_snap.axis, [0, 0, 1])


def test_grouped_snaps_are_not_generic(shadow):
    grouped = [s for s in snap.snaps("fixbeam") if s.group]
    assert len(grouped) == 1
    assert grouped[0].group == "craneArm"
    assert not grouped[0].is_generic


def test_rotation_axis_prefers_the_axle_hole(shadow):
    axis, source = snap.rotation_axis("fixbeam")
    assert np.allclose(axis, [0, 0, 1])
    assert "axle hole" in source


def test_rotation_axis_falls_back_to_an_include(shadow):
    """Many parts define their hole indirectly through an include."""
    axis, source = snap.rotation_axis("fixincl")
    assert np.allclose(axis, [0, 0, 1])
    assert "include" in source


def test_a_part_without_shadow_data_has_no_axis(shadow):
    assert snap.rotation_axis("nosuchpart") is None
    assert snap.snaps("nosuchpart") == []


def test_mounting_points_are_the_female_snaps(shadow):
    # Three of the four snaps are female; the pin is not a mounting point.
    assert len(snap.mounting_points("fixbeam")) == 3


def test_ensure_library_accepts_an_explicit_directory(shadow):
    """The default is resolved at call time, so pointing the extractor at
    another directory actually works."""
    assert snap.ensure_library() == str(shadow)


# --------------------------------------------------------------------------
# catalog entries built from the shadow data
# --------------------------------------------------------------------------

def test_entry_expands_the_grid_into_individual_holes(shadow):
    entry = catalog.entry("fixbeam")
    xs = sorted(round(h[0], 1) for h in entry.holes if h[6] == 0)
    assert xs == [-40.0, -20.0, 0.0, 20.0, 40.0], "the 5-hole grid did not expand"


def test_entry_marks_the_axle_hole_as_cross(shadow):
    entry = catalog.entry("fixbeam")
    cross = [h for h in entry.holes if h[6] == 1]
    assert len(cross) == 1
    assert entry.axle_holes == 1
    assert np.allclose(cross[0][:3], [0, 0, 40])


def test_entry_keeps_pins_separate_from_holes(shadow):
    entry = catalog.entry("fixbeam")
    assert len(entry.pins) == 1
    assert np.allclose(entry.pins[0][:3], [50, 0, 0])


def test_grouped_snaps_never_reach_the_catalog(shadow):
    """
    A [group=...] snap is a crane-arm slot or a hinge. Counting it as a port
    gives a liftarm a phantom male port along its whole length, and the search
    then builds things that cannot exist.
    """
    entry = catalog.entry("fixbeam")
    # Five grid holes plus one axle hole. The craneArm snap would make seven.
    assert len(entry.holes) == 6
    for hole in entry.holes:
        assert not np.allclose(hole[:3], [0, 10, 0]), "the grouped snap leaked in"


def test_entry_is_none_without_shadow_data(shadow):
    assert catalog.entry("nosuchpart") is None


def test_all_holes_share_one_direction_on_a_straight_beam(shadow):
    assert catalog.entry("fixbeam").axes == 1


def test_usable_drops_subparts_and_obsolete_titles(tmp_path):
    """
    LDraw marks subparts with a ~ prefix: they exist only inside another file
    and cannot be ordered. Leaving them in makes the search invent structures
    out of parts that do not exist.
    """
    parts_dir = tmp_path / "parts"
    parts_dir.mkdir()
    for name, title in [
        ("real.dat", "0 Technic Beam 3"),
        ("sub.dat", "0 ~Technic Beam 3 Subpart"),
        ("moved.dat", "0 Moved to 12345"),
        ("old.dat", "0 Technic Beam 3 (Obsolete)"),
        ("alias.dat", "0 =Technic Beam 3 Alias"),
    ]:
        (parts_dir / name).write_text(title + "\n", encoding="utf-8")

    cat = {n[:-4]: {"part": n[:-4], "holes": [[0] * 7], "pins": [], "axle_holes": 0}
           for n in ("real.dat", "sub.dat", "moved.dat", "old.dat", "alias.dat")}
    kept = catalog.usable(cat, str(parts_dir))
    assert set(kept) == {"real"}
    assert kept["real"]["title"] == "Technic Beam 3"
