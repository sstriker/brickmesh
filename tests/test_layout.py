# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""The geometric layer: shaft lines, distances and lattice arithmetic."""
import numpy as np
import pytest
from brickmesh_extract.layout import (
    Placement,
    Station,
    axes_intersect,
    effective_radius,
    free_intervals,
    line_distance,
    parallel_distance,
    perpendicular,
    sum_of_two_squares,
)

X = np.array([1.0, 0, 0])
Y = np.array([0, 1.0, 0])
Z = np.array([0, 0, 1.0])


def test_placement_normalizes_its_direction():
    p = Placement([0, 0, 0], [0, 0, 5])
    assert np.allclose(p.direction, Z)
    assert np.isclose(np.linalg.norm(p.direction), 1.0)


def test_placement_key_is_hashable_and_stable():
    a = Placement([1, 2, 3], Z).key()
    b = Placement([1, 2, 3], [0, 0, 2]).key()
    assert a == b
    assert len({a, b}) == 1


def test_parallel_distance_measures_perpendicular_offset():
    a = Placement([0, 0, 0], Z)
    b = Placement([4, 0, 100], Z)   # offset sideways, and along the shaft
    # Sliding along the shared direction must not change the distance.
    assert parallel_distance(a, b) == pytest.approx(4.0)


def test_parallel_distance_is_none_for_skew_shafts():
    assert parallel_distance(Placement([0, 0, 0], Z), Placement([0, 0, 0], X)) is None


def test_perpendicular_and_intersecting():
    a = Placement([0, 0, 0], Z)
    b = Placement([0, 0, 0], X)
    assert perpendicular(a, b)
    assert axes_intersect(a, b)
    # Same directions, lifted apart in Y: still perpendicular, no longer
    # intersecting. A bevel pair needs both.
    c = Placement([0, 30, 0], X)
    assert perpendicular(a, c)
    assert not axes_intersect(a, c)


def test_axes_intersect_is_false_for_parallel_lines():
    # Parallel lines never "intersect", even when they coincide in projection.
    assert not axes_intersect(Placement([0, 0, 0], Z), Placement([10, 0, 0], Z))


def test_line_distance_handles_parallel_and_skew():
    # Parallel: falls back to the perpendicular offset.
    assert line_distance(Placement([0, 0, 0], Z), Placement([3, 4, 50], Z)) == pytest.approx(5.0)
    # Skew: X-axis line and a Y-direction line lifted 7 along Z.
    assert line_distance(Placement([0, 0, 0], X), Placement([0, 0, 7], Y)) == pytest.approx(7.0)
    # Intersecting lines are zero apart.
    assert line_distance(Placement([0, 0, 0], X), Placement([0, 0, 0], Y)) == pytest.approx(0.0)


def test_sum_of_two_squares_finds_lattice_offsets():
    # 25 = 3^2+4^2 = 5^2+0^2, the classic buildable offsets.
    got = set(sum_of_two_squares(25))
    for pair in ((3, 4), (4, 3), (5, 0), (0, 5), (3, -4), (-5, 0)):
        assert pair in got
    for a, b in got:
        assert a * a + b * b == 25


def test_sum_of_two_squares_is_empty_when_impossible():
    # 6 is not a sum of two squares, so a gear pair needing sqrt(6) half studs
    # has nowhere on the lattice to go.
    assert sum_of_two_squares(6) == []


def test_effective_radius_pairs_sum_to_the_center_distance():
    # Same rule as the Go side: (t1+t2)/8 half studs.
    assert effective_radius(8) + effective_radius(24) == pytest.approx(4.0)
    assert effective_radius(12) + effective_radius(20) == pytest.approx(4.0)


def station(shaft: str, teeth: int, axial: float, thickness: float = 2.0) -> Station:
    return Station(shaft=shaft, teeth=teeth, axial=axial, thickness=thickness)


def test_station_span_is_centered_on_its_axial_position():
    lo, hi = station("a", 24, 4.0, thickness=2.0).span
    assert (lo, hi) == (3.0, 5.0)


def test_free_intervals_leaves_room_beside_a_gear():
    # One gear at the origin, 2 half studs thick, on a shaft reaching +/-12.
    free = free_intervals([station("a", 24, 0.0, 2.0)], "a", reach=12.0)
    assert free == [(-12.0, -1.0), (1.0, 12.0)]


def test_free_intervals_ignores_other_shafts():
    stations = [station("a", 24, 0.0), station("b", 24, 5.0)]
    assert free_intervals(stations, "b", reach=8.0) == [(-8.0, 4.0), (6.0, 8.0)]


def test_free_intervals_merges_overlapping_gears():
    # Two gears side by side leave one gap on each side, not three.
    stations = [station("a", 24, 0.0, 2.0), station("a", 24, 1.0, 2.0)]
    assert free_intervals(stations, "a", reach=10.0) == [(-10.0, -1.0), (2.0, 10.0)]


def test_a_crowded_shaft_has_nowhere_for_a_bearing():
    # A gear wider than the reach leaves no free stretch at all, which is what
    # the bearing search keys off.
    assert free_intervals([station("a", 40, 0.0, 24.0)], "a", reach=8.0) == []


def two_shaft_mech(ta: int, tb: int):
    from brickmesh_extract.mech import Mechanism
    m = Mechanism(f"{ta}t/{tb}t")
    m.shaft("a", bearings=2)
    m.shaft("b", bearings=2)
    m.mesh("a", "b", ta, tb)
    return m


def test_realize_places_a_pair_that_fits_the_lattice():
    from brickmesh_extract.layout import realize
    # 8+24 = 32 -> 4 half studs, and 16 = 0^2+4^2, so positions exist.
    sols = realize(two_shaft_mech(8, 24), max_solutions=3, span=1)
    assert sols
    lay = sols[0]
    assert parallel_distance(lay.place["a"], lay.place["b"]) == pytest.approx(4.0)


def test_realize_refuses_an_off_lattice_pair_rather_than_misplacing_it():
    """8t+12t needs 2.5 half studs. Squaring and rounding that to 6 used to
    invent candidates at sqrt(6); there is genuinely nowhere to put it."""
    from brickmesh_extract.layout import realize
    assert realize(two_shaft_mech(8, 12), max_solutions=3, span=1) == []


def test_realize_does_not_place_the_9_5_pair_a_fraction_out():
    """36t+40t needs 9.5 half studs. Rounding 90.25 to 90 would place the gear
    at sqrt(90) = 9.487 — close enough to look right, and wrong."""
    from brickmesh_extract.layout import realize
    for lay in realize(two_shaft_mech(36, 40), max_solutions=3, span=1):
        d = parallel_distance(lay.place["a"], lay.place["b"])
        assert d == pytest.approx(9.5), f"placed at {d}, which does not mesh"
