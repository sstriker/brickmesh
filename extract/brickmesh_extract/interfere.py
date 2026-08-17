# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
A real intersection test at triangle level.

The earlier approach measured distances between individual triangle vertices.
That cloud is too sparse: two gears grazing past each other and two meshing
properly both came out at "distance ~0". This is the replacement: FCL does a
real triangle-against-triangle test, exactly the class of test Stud.io does.

The rule for a correct gear position:
  - at AT LEAST ONE tooth phase there must be no intersection (that is the
    backlash)
  - rotated by half a pitch there MUST be an intersection (the teeth then
    really engage instead of running past each other)
If a position satisfies the first but not the second, it is only grazing past.
"""
from __future__ import annotations

import numpy as np
import trimesh

from . import ldraw
from .core import rot


def _mesh(part: str) -> trimesh.Trimesh:
    m = ldraw.geometry(part).mesh()
    if m is None:
        raise ValueError(f"{part}: no mesh")
    return m


def _transform(orient: np.ndarray, pos: np.ndarray) -> np.ndarray:
    T = np.eye(4)
    T[:3, :3] = orient
    T[:3, 3] = pos
    return T


def engagement(part_a: str, orient_a, pos_a,
               part_b: str, orient_b, pos_b,
               phase_axis: str = "x", phase_steps: int = 24,
               pitch_deg: float = 30.0) -> dict:
    """
    Rotate part B through one full tooth pitch and check at every step whether
    the meshes intersect.
    """
    ma, mb = _mesh(part_a), _mesh(part_b)
    ma = ma.copy(); ma.apply_transform(_transform(np.asarray(orient_a), np.asarray(pos_a)))

    mgr = trimesh.collision.CollisionManager()
    mgr.add_object("a", ma)

    hits, dists = [], []
    for k in range(phase_steps):
        ang = k * pitch_deg / phase_steps
        o = np.asarray(orient_b) @ rot(phase_axis, ang)
        m = mb.copy(); m.apply_transform(_transform(o, np.asarray(pos_b)))
        hits.append(bool(mgr.in_collision_single(m)))
        dists.append(float(mgr.min_distance_single(m)))

    n_free = sum(1 for h in hits if not h)
    return {
        "phases": phase_steps,
        "free_phases": n_free,
        "colliding_phases": phase_steps - n_free,
        "min_distance": min(dists),
        "min_distance_when_free": min([d for d, h in zip(dists, hits) if not h], default=None),
        "verdict": _verdict(n_free, phase_steps, min(dists)),
    }


def _verdict(n_free: int, total: int, min_dist: float) -> str:
    if n_free == 0:
        return "JAMS - intersection at every tooth phase"
    if n_free == total:
        if min_dist > 2.0:
            return "NO CONTACT - the gears do not touch"
        return "GRAZES - free at every phase, so the teeth do not engage"
    return "MESHES - free at some phases, jams at others"


def mesh_lock_robust(part_a: str, orient_a, pos_a,
                     part_b: str, orient_b, pos_b,
                     teeth_b: int, teeth_a: int,
                     spin_axis: str = "z", steps: int = 72) -> dict:
    """
    mesh_lock, but resistant to the open-shell error.

    LDraw parts are not closed volumes (the differential has 835 boundary
    loops), and on such a shell FCL occasionally returns a wrong answer
    depending on the orientation. A ring gear is symmetric under rotation by a
    whole number of teeth, though, so the same measurement at four orientations
    of A has to come out the same. If one of them deviates, that one is the
    artifact.
    """
    from collections import Counter
    results = []
    for k in range(4):
        ang = k * 360.0 / 4
        if (teeth_a * k) % 4 != 0 and k not in (0, 2):
            pass                       # the rotation need not be a whole number of teeth
        o = np.asarray(orient_a) @ rot("z", ang)
        r = mesh_lock(part_a, o, pos_a, part_b, orient_b, pos_b,
                      teeth_b, spin_axis=spin_axis, steps=steps)
        results.append(r)
    counts = Counter(r["windows"] for r in results)
    modal, n = counts.most_common(1)[0]
    winner = next(r for r in results if r["windows"] == modal)
    winner = dict(winner)
    winner["agreement"] = f"{n}/4"
    winner["outliers"] = [r["windows"] for r in results if r["windows"] != modal]
    if n < 3:
        winner["verdict"] = f"UNRELIABLE - measurements disagree {sorted(counts)}"
    return winner


def mesh_lock(part_a: str, orient_a, pos_a,
              part_b: str, orient_b, pos_b,
              teeth_b: int, spin_axis: str = "z", steps: int = 144) -> dict:
    """
    The decisive engagement test: rotate B one full turn while A stands still.

    If the teeth really engage, B is blocked for most of the turn and exactly
    `teeth_b` narrow free windows remain, one per tooth, spaced a tooth pitch
    apart. That window width IS the backlash.

    If B turns freely, the gears either do not touch at all or merely graze
    along the tips - however close together they stand.
    """
    ma = _mesh(part_a).copy()
    ma.apply_transform(_transform(np.asarray(orient_a), np.asarray(pos_a)))
    mgr = trimesh.collision.CollisionManager()
    mgr.add_object("a", ma)
    mb = _mesh(part_b)

    free = []
    for k in range(steps):
        ang = k * 360.0 / steps
        o = np.asarray(orient_b) @ rot(spin_axis, ang)
        m = mb.copy(); m.apply_transform(_transform(o, np.asarray(pos_b)))
        if not mgr.in_collision_single(m):
            free.append(ang)

    if not free:
        return {"verdict": "TOO DEEP - no phase free at all, cannot be assembled",
                "windows": 0, "expected_windows": teeth_b, "window_spacing_deg": 0.0,
                "expected_spacing_deg": 360.0 / teeth_b,
                "backlash_deg": 0.0, "free_fraction": 0.0}
    if len(free) == steps:
        return {"verdict": "NO ENGAGEMENT - turns freely",
                "windows": 0, "expected_windows": teeth_b, "window_spacing_deg": 0.0,
                "expected_spacing_deg": 360.0 / teeth_b,
                "backlash_deg": 360.0, "free_fraction": 1.0}

    # merge consecutive phases into windows
    step = 360.0 / steps
    windows, cur = [], [free[0]]
    for a in free[1:]:
        if a - cur[-1] <= step * 1.5:
            cur.append(a)
        else:
            windows.append(cur); cur = [a]
    windows.append(cur)
    if len(windows) > 1 and (free[0] + 360.0) - windows[-1][-1] <= step * 1.5:
        windows[0] = windows[-1] + windows[0]; windows.pop()

    starts = [w[0] for w in windows]
    spacing = np.diff(starts) if len(starts) > 1 else np.array([360.0])
    expected = 360.0 / teeth_b
    ok = len(windows) == teeth_b and abs(np.median(spacing) - expected) < expected * 0.15

    return {
        "verdict": ("MESHES" if ok else
                    f"DOUBTFUL - {len(windows)} windows, expected {teeth_b}"),
        "windows": len(windows),
        "expected_windows": teeth_b,
        "window_spacing_deg": float(np.median(spacing)),
        "expected_spacing_deg": expected,
        "backlash_deg": float(np.median([len(w) for w in windows]) * step),
        "free_fraction": len(free) / steps,
    }


if __name__ == "__main__":
    Z = np.eye(3)
    RY = rot("y", 90)
    RING = 23.5

    print("12t against the 28t ring gear of the differential")
    print("Diff sits at the origin, axis along Z, ring gear at Z=+20..+27\n")
    print(f"  {'radial':>8}{'axial':>8}  {'free':>6}{'jam':>6}{'mindist':>9}   verdict")

    for off in (30.0, 35.0, 40.0):
        for z in (5.0, 8.0, 12.0, 38.0, 42.0, 46.0):
            try:
                r = engagement("62821.dat", Z, [0, 0, 0],
                               "32270.dat", RY, [off, 0, z],
                               phase_axis="x", pitch_deg=30.0, phase_steps=18)
            except Exception as exc:
                print(f"  {off:8.1f}{z:8.1f}  error: {exc}")
                continue
            print(f"  {off:8.1f}{z:8.1f}  {r['free_phases']:6d}"
                  f"{r['colliding_phases']:6d}{r['min_distance']:9.2f}   {r['verdict']}")
