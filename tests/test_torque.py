# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""Torque propagation and tooth loads.

The propagation arithmetic is exact and worth pinning down. The failure limits
it is compared against are explicitly unverified community figures, so these
tests check that the assessment reacts to the table rather than asserting that
any particular limit is right.
"""
import pytest
from brickmesh_extract.torque import (
    EFFICIENCY,
    LIMITS,
    Stage,
    assess,
    pitch_radius_mm,
    propagate,
    tooth_force_N,
    unverified_notice,
)


def test_pitch_radius_follows_the_mesh_rule():
    # radius in mm = teeth / 2, so two meshing gears sit (t1+t2)/2 mm apart.
    assert pitch_radius_mm(8) == 4.0
    assert pitch_radius_mm(24) == 12.0
    assert pitch_radius_mm(8) + pitch_radius_mm(24) == 16.0


def test_stage_ratio_and_efficiency():
    s = Stage("reduction", driver_teeth=8, driven_teeth=24)
    assert s.ratio == 3.0
    assert s.eff == EFFICIENCY["spur"]
    # A worm is deliberately lossy; that is the price of self-locking.
    assert Stage("w", 1, 24, kind="worm").eff == EFFICIENCY["worm"]
    # An unknown kind falls back rather than raising.
    assert Stage("odd", 8, 8, kind="mystery").eff == 0.9


def test_torque_rises_with_reduction_and_loses_to_friction():
    rows = propagate(10.0, [Stage("s1", 8, 24)])
    assert len(rows) == 1
    row = rows[0]
    assert row["torque_in_Ncm"] == 10.0
    # 3x reduction at 94% efficiency.
    assert row["torque_out_Ncm"] == pytest.approx(10.0 * 3 * 0.94)
    assert row["torque_out_Ncm"] < 30.0


def test_torque_chains_through_stages():
    stages = [Stage("s1", 8, 24), Stage("s2", 8, 24)]
    rows = propagate(10.0, stages)
    assert rows[1]["torque_in_Ncm"] == rows[0]["torque_out_Ncm"]
    assert rows[1]["torque_out_Ncm"] == pytest.approx(10.0 * (3 * 0.94) ** 2)


def test_smaller_gear_sees_the_higher_tooth_force():
    # Same torque on a smaller radius means more force on the flank, which is
    # why the 8t is the classic first part to fail.
    assert tooth_force_N(10.0, 8) > tooth_force_N(10.0, 24)
    # 10 Ncm on a 8t: 0.1 Nm / 0.004 m = 25 N.
    assert tooth_force_N(10.0, 8) == pytest.approx(25.0)


def test_gentle_train_passes_assessment():
    assert assess(propagate(1.0, [Stage("s1", 24, 24)])) == [
        "No limits exceeded — but see the warning about the limit values."
    ]


def test_overloaded_small_gear_is_failed():
    # Far past the 8t limit in the table.
    out = assess(propagate(200.0, [Stage("s1", 8, 24)]))
    assert any(line.startswith("FAIL") for line in out)
    assert any("skip" in line for line in out)


def test_assessment_tracks_the_table_not_a_constant():
    """Lower the 8t limit and a previously fine train must start failing."""
    rows = propagate(8.0, [Stage("s1", 8, 24)])
    original = LIMITS["gear_tooth_force_N"]["gear_8t"]["value"]
    try:
        LIMITS["gear_tooth_force_N"]["gear_8t"]["value"] = 1.0
        assert any(line.startswith("FAIL") for line in assess(rows))
    finally:
        LIMITS["gear_tooth_force_N"]["gear_8t"]["value"] = original


def test_every_limit_is_still_marked_unverified():
    """If someone measures these for real, they should flip `verified` and this
    test should be updated deliberately rather than quietly passing."""
    notice = unverified_notice()
    total = sum(len(group) for group in LIMITS.values())
    assert len(notice) == total
    for group in LIMITS.values():
        for entry in group.values():
            assert entry["verified"] is False
            assert entry["source"]
