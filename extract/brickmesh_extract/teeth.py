# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Tooth positions and mesh phasing.

Where the teeth sit is derivable from the part geometry: sweep the angle around
the rotation axis, keep the points near the tip radius, and the teeth show up as
peaks. From that follows the phase two gears need in order to interleave rather
than butt heads - and that phase is what fixes the rotation of the axle running
through them.
"""
from __future__ import annotations

import numpy as np

from . import ldraw


def _axis_frame(axis: np.ndarray):
    axis = axis / np.linalg.norm(axis)
    tmp = np.array([1.0, 0, 0]) if abs(axis[0]) < 0.9 else np.array([0, 1.0, 0])
    u = np.cross(axis, tmp); u /= np.linalg.norm(u)
    v = np.cross(axis, u)
    return axis, u, v


def tooth_angles(part: str, axis: np.ndarray, teeth: int,
                 tip_frac: float = 0.93) -> np.ndarray:
    """
    Angular position (degrees) of each tooth in the part's own frame.
    Returns `teeth` angles, ascending.
    """
    g = ldraw.geometry(part)
    n, u, v = _axis_frame(axis)
    p = g.verts
    r = np.hypot(p @ u, p @ v)
    tip = r.max()
    sel = r > tip_frac * tip
    if sel.sum() < teeth * 4:
        sel = r > 0.85 * tip
    ang = np.degrees(np.arctan2(p[sel] @ v, p[sel] @ u)) % 360.0

    pitch = 360.0 / teeth
    # Fold every tooth onto one pitch window and take the circular mean.
    # That is far more robust than peak picking on a sparse vertex cloud.
    phase = (ang % pitch) / pitch * 2 * np.pi
    mean = np.arctan2(np.sin(phase).mean(), np.cos(phase).mean())
    offset = (mean / (2 * np.pi)) * pitch % pitch
    return (offset + np.arange(teeth) * pitch) % 360.0


def tooth_sharpness(part: str, axis: np.ndarray, teeth: int) -> float:
    """
    How cleanly the teeth cluster (0..1). Low values mean the extraction is
    unreliable and the resulting phase should not be trusted.
    """
    g = ldraw.geometry(part)
    n, u, v = _axis_frame(axis)
    p = g.verts
    r = np.hypot(p @ u, p @ v)
    sel = r > 0.93 * r.max()
    ang = np.degrees(np.arctan2(p[sel] @ v, p[sel] @ u)) % 360.0
    pitch = 360.0 / teeth
    phase = (ang % pitch) / pitch * 2 * np.pi
    return float(np.hypot(np.sin(phase).mean(), np.cos(phase).mean()))


def axle_symmetry_ok(teeth: int) -> bool:
    """
    A cross axle has 4-fold symmetry, so a gear can be seated on it in four
    ways. If the tooth count is a multiple of 4 those four seatings map teeth
    onto teeth and the seating does not affect phase at all.
    """
    return teeth % 4 == 0


def mesh_phase(part_a: str, teeth_a: int, axis_a: np.ndarray,
               part_b: str, teeth_b: int, axis_b: np.ndarray,
               direction_ab: np.ndarray) -> dict:
    """
    Rotation (deg, about each gear's own axis) so that a tooth of A points at B
    while a gap of B points back at A.
    """
    _, ua, va = _axis_frame(axis_a)
    _, ub, vb = _axis_frame(axis_b)

    d = direction_ab / np.linalg.norm(direction_ab)
    beta_a = np.degrees(np.arctan2(d @ va, d @ ua)) % 360.0
    beta_b = np.degrees(np.arctan2(-d @ vb, -d @ ub)) % 360.0

    ta = tooth_angles(part_a, axis_a, teeth_a)
    tb = tooth_angles(part_b, axis_b, teeth_b)
    pitch_a, pitch_b = 360.0 / teeth_a, 360.0 / teeth_b

    # A: bring a tooth onto the line of centers.
    rot_a = (beta_a - ta[0]) % pitch_a
    # B: bring a GAP onto the line of centers, i.e. a tooth half a pitch away.
    rot_b = (beta_b + pitch_b / 2 - tb[0]) % pitch_b

    return {
        "rot_a_deg": rot_a,
        "rot_b_deg": rot_b,
        "pitch_a_deg": pitch_a,
        "pitch_b_deg": pitch_b,
        "sharpness_a": tooth_sharpness(part_a, axis_a, teeth_a),
        "sharpness_b": tooth_sharpness(part_b, axis_b, teeth_b),
        "axle_seating_free_a": axle_symmetry_ok(teeth_a),
        "axle_seating_free_b": axle_symmetry_ok(teeth_b),
    }


if __name__ == "__main__":
    Z = np.array([0.0, 0, 1])
    print("Tooth positions derived from the geometry\n")
    print(f"  {'part':10s}{'teeth':>7}{'pitch':>8}{'1st tooth':>10}{'sharpness':>10}")
    for p, t in [("3647.dat", 8), ("32270.dat", 12), ("4019.dat", 16),
                 ("32269.dat", 20), ("3648b.dat", 24), ("32498.dat", 36)]:
        ang = tooth_angles(p, Z, t)
        sh = tooth_sharpness(p, Z, t)
        print(f"  {p:10s}{t:7d}{360/t:8.2f}{ang[0]:10.2f}{sh:10.3f}"
              f"   {'axle seating free' if axle_symmetry_ok(t) else 'NOTE: seating fixes phase'}")
