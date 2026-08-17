# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""The functional layer: degrees of freedom, speeds, loop closure.

Pure linear algebra over the mechanism graph, so none of this needs the parts
libraries.
"""
import pytest
from brickmesh_extract.mech import Mechanism


def simple_train() -> Mechanism:
    """8t driving 24t: one input, one output, ratio 3."""
    m = Mechanism("simple")
    m.shaft("in", bearings=2)
    m.shaft("out", bearings=2)
    m.mesh("in", "out", 8, 24)
    m.drive("in", 1.0)
    m.output("out")
    return m


def subtractor() -> Mechanism:
    """A differential fed from two sides: the case averages both outputs."""
    m = Mechanism("subtractor")
    for s in ("drive", "steer", "case"):
        m.shaft(s, bearings=2)
    m.differential("case", "drive", "steer")
    return m


def test_dof_of_a_plain_train_is_one():
    assert simple_train().dof() == 1


def test_subtractor_has_two_degrees_of_freedom():
    # The whole reason a subtractor works: three shafts, one equation.
    assert subtractor().dof() == 2


def test_speeds_follow_the_gear_ratio():
    sol = simple_train().solve()
    assert sol is not None
    assert sol["in"] == pytest.approx(1.0)
    # 8t driving 24t turns the output three times slower, and a spur pair
    # reverses direction.
    assert sol["out"] == pytest.approx(-1.0 / 3.0)


def test_differential_case_is_the_average_of_its_outputs():
    m = subtractor()
    m.drive("drive", 4.0)
    m.drive("steer", 2.0)
    sol = m.solve()
    assert sol is not None
    assert sol["case"] == pytest.approx(3.0)


def test_underdriven_mechanism_is_unsolvable():
    # Two degrees of freedom but only one input: the rest stays undetermined.
    m = subtractor()
    m.drive("drive", 1.0)
    assert m.solve() is None


def test_locked_train_is_reported():
    """Three shafts in a ring is one equation too many: nothing can turn."""
    m = Mechanism("locked")
    for s in ("a", "b", "c"):
        m.shaft(s, bearings=2)
    m.mesh("a", "b", 8, 8)
    m.mesh("b", "c", 8, 8)
    m.mesh("a", "c", 8, 8)
    assert m.dof() == 0
    levels = {(f.level, f.check) for f in m.check_dof()}
    assert ("FAIL", "dof") in levels


def test_too_many_drives_is_overdetermined():
    m = simple_train()
    m.drive("out", 1.0)
    assert any(f.level == "FAIL" and f.check == "dof" for f in m.check_dof())


def test_single_bearing_shaft_fails_the_bearing_check():
    m = Mechanism("wobbly")
    m.shaft("lonely", bearings=1)
    findings = m.check_bearings()
    assert [f.level for f in findings] == ["FAIL"]
    assert "lonely" in findings[0].detail


def test_mixed_grid_domains_are_flagged():
    """Technic bricks sit at 24 LDU vertically, liftarms at 20: holes do not
    line up, so a gear pair spanning both cannot be built."""
    m = Mechanism("mixed")
    m.shaft("a", bearings=2, domain="technic-studless")
    m.shaft("b", bearings=2, domain="technic-brick")
    m.mesh("a", "b", 8, 24)
    assert any(f.level == "FAIL" and f.check == "grid" for f in m.check_domains())


def test_loop_that_closes_on_the_lattice():
    # 8t/24t, 24t/8t and 8t/8t give distances 2, 2 and 1 studs: a valid
    # triangle whose third point lands on whole half studs.
    m = Mechanism("closing")
    for s in ("a", "b", "c"):
        m.shaft(s, bearings=2)
    m.mesh("a", "b", 24, 8)
    m.mesh("b", "c", 8, 24)
    m.mesh("a", "c", 24, 24)
    closure = [f for f in m.check_closure() if f.check == "loop closure"]
    assert closure, "expected the triangle to be examined"


def test_degenerate_triangle_is_rejected():
    """Distances that cannot form a triangle at all."""
    m = Mechanism("degenerate")
    for s in ("a", "b", "c"):
        m.shaft(s, bearings=2)
    m.mesh("a", "b", 8, 8)     # 2 half studs
    m.mesh("b", "c", 8, 8)     # 2 half studs
    m.mesh("a", "c", 40, 40)   # 10 half studs — longer than the other two
    fails = [f for f in m.check_closure() if f.level == "FAIL"]
    assert fails and "triangle" in fails[0].detail


def test_no_loops_reports_ok():
    findings = simple_train().check_closure()
    assert [f.level for f in findings] == ["OK"]


def test_center_distance_only_applies_to_spur_pairs():
    m = Mechanism("bevel")
    m.shaft("a", bearings=2)
    m.shaft("b", bearings=2)
    m.mesh("a", "b", 12, 20, kind="bevel")
    link = m.links[0]
    assert link.center_distance_halfstuds is None
    # A bevel pair still reverses; a worm does not.
    assert link.reverses


def test_backlash_accumulates_along_a_path():
    m = Mechanism("backlash")
    for s in ("a", "b", "c"):
        m.shaft(s, bearings=2)
    m.mesh("a", "b", 8, 24, backlash_deg=4.0)
    m.mesh("b", "c", 8, 24, backlash_deg=4.0)
    # The first stage's play is seen through the second stage's reduction, so
    # the total is less than the naive sum of 8.
    total = m.backlash(["a", "b", "c"])
    assert 4.0 < total < 8.0


def spur_pair(ta: int, tb: int) -> Mechanism:
    m = Mechanism(f"{ta}t/{tb}t")
    m.shaft("a", bearings=2)
    m.shaft("b", bearings=2)
    m.mesh("a", "b", ta, tb)
    return m


def test_center_distance_on_the_lattice_is_accepted():
    # 8+24 = 32, a multiple of 8, so the pair sits 4 whole half studs apart.
    findings = spur_pair(8, 24).check_center_distances()
    assert [f.level for f in findings] == ["OK"]


def test_center_distance_off_the_lattice_is_failed():
    """Both counts are multiples of 4, but they sum to 20, not a multiple of 8,
    so the pair lands on 2.5 half studs and cannot be built."""
    findings = spur_pair(8, 12).check_center_distances()
    assert [f.level for f in findings] == ["FAIL"]
    assert "2.5 half studs" in findings[0].detail
    assert "8+12 = 20" in findings[0].detail


def test_the_other_off_lattice_pair_is_failed_too():
    findings = spur_pair(36, 40).check_center_distances()
    assert [f.level for f in findings] == ["FAIL"]
    assert "9.5 half studs" in findings[0].detail


def test_bevel_pairs_are_not_center_distance_checked():
    # A bevel pair has no center distance to speak of; the shafts intersect.
    m = Mechanism("bevel")
    m.shaft("a", bearings=2)
    m.shaft("b", bearings=2)
    m.mesh("a", "b", 8, 12, kind="bevel")
    assert [f.level for f in m.check_center_distances()] == ["OK"]


def test_center_distance_check_runs_as_part_of_the_suite():
    checks = {f.check for f in spur_pair(8, 12).run_checks()}
    assert "center dist" in checks


# --------------------------------------------------------------------------
# facts recorded in PLAN.md and docs/findings.md, pinned as regressions
# --------------------------------------------------------------------------

STANDARD_TEETH = [8, 12, 16, 20, 24, 28, 36, 40]


def triple_closes(t1: int, t2: int, t3: int) -> bool:
    m = Mechanism("triple")
    for s in ("a", "b", "c"):
        m.shaft(s, bearings=2)
    m.mesh("a", "b", t1, t2)
    m.mesh("b", "c", t2, t3)
    m.mesh("a", "c", t1, t3)
    closure = [f for f in m.check_closure() if f.check == "loop closure"]
    return bool(closure) and all(f.level == "OK" for f in closure)


def test_exactly_seven_of_512_gear_triples_close():
    """Documented in PLAN.md, and cheap to keep honest."""
    import itertools
    closing = [t for t in itertools.product(STANDARD_TEETH, repeat=3)
               if triple_closes(*t)]
    assert len(closing) == 7
    assert {tuple(sorted(t)) for t in closing} == {
        (8, 12, 40), (8, 16, 24), (12, 24, 36), (16, 24, 24)}


def test_only_two_of_those_four_are_actually_buildable():
    """
    check_closure places the first shaft at the origin and the second at
    (d, 0), then asks whether the THIRD lands on the lattice — it never checks
    that the second one did. (8,12,40) and (12,24,36) each contain a pair whose
    tooth counts do not sum to a multiple of 8, so a shaft sits on a quarter
    stud and the triple cannot be built after all. check_center_distances is
    what catches that.
    """
    buildable = []
    for triple in ((8, 12, 40), (8, 16, 24), (12, 24, 36), (16, 24, 24)):
        m = Mechanism("t")
        for s in ("a", "b", "c"):
            m.shaft(s, bearings=2)
        a, b, c = triple
        m.mesh("a", "b", a, b)
        m.mesh("b", "c", b, c)
        m.mesh("a", "c", a, c)
        if all(f.level == "OK" for f in m.check_center_distances()):
            buildable.append(triple)
    assert buildable == [(8, 16, 24), (16, 24, 24)]


def test_driving_both_tracks_together_turns_the_case_with_them():
    """A subtractor rolling straight: both tracks the same way, and the
    differential case follows at the same speed."""
    m = subtractor()
    m.drive("drive", 1.0)
    m.drive("steer", 1.0)
    sol = m.solve()
    assert sol is not None
    assert sol["case"] == pytest.approx(1.0)


def test_driving_the_tracks_opposite_pivots_on_the_spot():
    """Equal and opposite tracks: the machine spins in place, so the case —
    the forward-motion port — stands still."""
    m = subtractor()
    m.drive("drive", 1.0)
    m.drive("steer", -1.0)
    sol = m.solve()
    assert sol is not None
    assert sol["case"] == pytest.approx(0.0)
