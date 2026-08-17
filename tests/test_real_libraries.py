# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The facts that only real parts can settle.

PLAN.md's first rule is not to trust a geometric measurement that has not been
calibrated: two 24-tooth gears mesh at exactly 60 LDU, and any collision or
engagement test has to reproduce that before its other answers mean anything.
A synthetic box cannot stand in for tooth geometry, so these run against the
actual LDraw and LDCad libraries.

They are opt-in because they download: set BRICKMESH_LIBRARIES=1. CI does that
in a job that caches both libraries between runs.
"""
import numpy as np
import pytest

pytest.importorskip("trimesh", reason="needs the geometry extra")
pytest.importorskip("fcl", reason="needs the geometry extra")

from brickmesh_extract import build, catalog, holes, interfere, ldraw, snap, teeth

pytestmark = pytest.mark.libraries

GEAR_24 = "3648b.dat"       # Technic Gear 24 Tooth
FRAME_5x7 = "64179"         # Technic Beam 7 x 5 with Open Center 5 x 3
Z_AXIS = np.array([0.0, 0.0, 1.0])


# --------------------------------------------------------------------------
# the calibration
# --------------------------------------------------------------------------

def test_two_24t_gears_mesh_at_60_ldu():
    """
    The reference measurement. 24 teeth is a pitch radius of 30 LDU, so a pair
    sits 60 LDU apart, and turning one of them through a full revolution has to
    leave exactly 24 free windows — one per tooth.
    """
    result = interfere.mesh_lock(GEAR_24, np.eye(3), [0, 0, 0],
                                 GEAR_24, np.eye(3), [60.0, 0, 0],
                                 teeth_b=24, steps=360)
    assert result["verdict"] == "MESHES"
    assert result["windows"] == 24
    assert result["window_spacing_deg"] == pytest.approx(15.0, abs=0.5)


def test_the_same_pair_jams_at_58_ldu():
    """Two LDU too close and the teeth cannot be assembled at any phase."""
    result = interfere.mesh_lock(GEAR_24, np.eye(3), [0, 0, 0],
                                 GEAR_24, np.eye(3), [58.0, 0, 0],
                                 teeth_b=24, steps=360)
    assert result["windows"] == 0
    assert "TOO DEEP" in result["verdict"]


def test_pulling_them_apart_widens_the_backlash():
    at_60, at_62 = (
        interfere.mesh_lock(GEAR_24, np.eye(3), [0, 0, 0],
                            GEAR_24, np.eye(3), [d, 0, 0], teeth_b=24, steps=360)
        for d in (60.0, 62.0)
    )
    assert at_62["free_fraction"] > at_60["free_fraction"]


def test_the_verdict_needs_enough_angular_resolution():
    """
    A caveat worth pinning. The free window either side of a meshed tooth is a
    couple of degrees wide, so sampling every 5 degrees steps straight over it
    and mesh_lock reports a confident "cannot be assembled" for a pair that
    meshes perfectly. The default of 144 steps is fine; coarser is not.
    """
    coarse = interfere.mesh_lock(GEAR_24, np.eye(3), [0, 0, 0],
                                 GEAR_24, np.eye(3), [60.0, 0, 0],
                                 teeth_b=24, steps=72)
    assert coarse["windows"] == 0, "if this now passes, the caveat can go"

    default = interfere.mesh_lock(GEAR_24, np.eye(3), [0, 0, 0],
                                  GEAR_24, np.eye(3), [60.0, 0, 0],
                                  teeth_b=24)
    assert default["windows"] == 24


def test_a_24t_needs_half_a_pitch_of_phase():
    """One gear puts a tooth on the line of centers, the other a gap: half of
    the 15 degree pitch."""
    phase = teeth.mesh_phase(GEAR_24, 24, Z_AXIS, GEAR_24, 24, Z_AXIS,
                             np.array([60.0, 0.0, 0.0]))
    assert phase["pitch_a_deg"] == pytest.approx(15.0)
    assert (phase["rot_b_deg"] - phase["rot_a_deg"]) % 15.0 == pytest.approx(7.5)
    # A 24t is a multiple of 4, so seating it on a cross axle does not move it.
    assert phase["axle_seating_free_a"]


# --------------------------------------------------------------------------
# the hole probe against a part whose holes are known
# --------------------------------------------------------------------------

def probe(axis: str) -> list[tuple[float, float]]:
    return holes.cluster(holes.find_holes(FRAME_5x7, axis=axis, step=2.0))


def inside_the_part(found, half_thickness: float = 10.0):
    """
    Drop the hits that sit outside the part. Just past a corner the thin probe
    misses the beam while the thick one still clips it, which looks exactly
    like a hole; every such hit is outside the 20 LDU thickness, so position
    alone separates them. find_holes does not do this itself.
    """
    return sorted(c for c in found if abs(c[0]) <= half_thickness or
                  abs(c[1]) <= half_thickness)


def test_the_frame_has_three_holes_along_x():
    found = [c for c in probe("x") if abs(c[0]) <= 10.0]     # coords are (Y, Z)
    assert [round(z, 1) for _, z in sorted(found, key=lambda c: c[1])] == \
        [-40.0, 0.0, 40.0]
    assert all(y == pytest.approx(0.0, abs=1e-6) for y, _ in found)


def test_the_frame_has_three_holes_along_z():
    found = [c for c in probe("z") if abs(c[1]) <= 10.0]     # coords are (X, Y)
    assert [round(x, 1) for x, _ in sorted(found)] == [-20.0, 0.0, 20.0]


def test_the_frame_geometry_resolves():
    g = ldraw.geometry(FRAME_5x7)
    assert "Open Center" in g.title
    # 7 x 5 studs at 20 LDU, one brick-width thick.
    assert np.allclose(g.size, [100.0, 20.0, 140.0])


# --------------------------------------------------------------------------
# the extractor against the real shadow library
# --------------------------------------------------------------------------

@pytest.fixture(scope="module")
def real_records():
    """A slice of the real catalog. The limit keeps the run short; the point is
    the shape of the output, not its size."""
    return build.build(max_tier=3, limit=400)


def test_the_extractor_produces_the_engine_schema(real_records):
    assert real_records
    for record in real_records:
        assert set(record) == {"id", "title", "tier", "holes", "pins"}
        assert record["tier"] in (1, 2, 3)
        assert record["holes"] or record["pins"]
        for port in record["holes"] + record["pins"]:
            assert len(port) == 7


def test_titles_are_real_part_names_not_shadow_headers(real_records):
    """
    The failure this guards against is quiet. Reading the shadow directory as
    if it were LDraw gives every part the title 'LDCad shadow info for "..."',
    which matches no tier pattern and no subpart prefix, so nothing is filtered
    and everything falls into tier 3.
    """
    for record in real_records:
        assert "shadow info" not in record["title"]
    assert any(r["title"].startswith("Technic ") for r in real_records)


def test_no_subparts_or_obsolete_entries_survive(real_records):
    for record in real_records:
        title = record["title"]
        assert not title.startswith("~"), f"{record['id']} is a subpart"
        assert not title.startswith("="), f"{record['id']} is an alias"
        assert not title.lower().startswith("moved")
        assert "obsolete" not in title.lower()


def test_the_tiers_actually_separate(real_records):
    tiers = {r["tier"] for r in real_records}
    assert tiers == {1, 2, 3}, f"expected all three tiers, got {tiers}"
    for record in real_records:
        if record["tier"] == 1:
            assert "Technic" in record["title"]


def test_grouped_snaps_never_become_ports():
    """
    A [group=...] snap is a specific connection — a crane arm, a hinge, a plug.
    Counted as a port it gives a liftarm a phantom male port along its whole
    length. Checked here against real shadow data rather than a fixture.
    """
    root = snap.ensure_library()
    grouped = []
    for part in ("64179", "32523", "32316", "3648b"):
        grouped += [s for s in snap.snaps(part, root) if s.group]
        entry = catalog.entry(part)
        if entry is None:
            continue
        for s in snap.snaps(part, root):
            if not s.group:
                continue
            for hole in entry.holes + entry.pins:
                assert not np.allclose(hole[:3], s.pos), \
                    f"{part}: the {s.group} snap reached the catalog"
    # The assertion above is only meaningful if such snaps exist at all.
    assert grouped, "no grouped snaps in the sample; pick different parts"
