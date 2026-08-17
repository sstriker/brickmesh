# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Bevel-mesh solver.

Parallel spur meshing follows the (t1+t2)/16 rule and needs no search. Bevel
engagement (a 12t double bevel driving the 28t ring inside a differential) does
not: the axes are perpendicular and the pitch cones meet at an apex that is not
derivable from bounding boxes.

So solve it from the actual tooth geometry. Sweep the driver over a grid of
candidate positions and score each on the gap between the two tooth surfaces.
The wanted position is where the surfaces just touch: a small positive minimum
gap, with no deep interpenetration.
"""
from __future__ import annotations

import numpy as np
from scipy.spatial import cKDTree

from . import ldraw
from .core import rot


def _subsample(v: np.ndarray, n: int, seed: int = 0) -> np.ndarray:
    if len(v) <= n:
        return v
    rng = np.random.default_rng(seed)
    return v[rng.choice(len(v), n, replace=False)]


def ring_teeth(diff_part: str = "62821.dat", axis: int = 2,
               min_radius: float = 30.0, n: int = 4000) -> np.ndarray:
    """Isolate the ring gear tooth surface from the differential housing."""
    g = ldraw.geometry(diff_part)
    v = g.verts
    other = [i for i in range(3) if i != axis]
    r = np.hypot(v[:, other[0]], v[:, other[1]])
    teeth = v[r > min_radius]
    return _subsample(teeth, n)


def driver_teeth(gear_part: str = "32270.dat", n: int = 3000) -> np.ndarray:
    return _subsample(ldraw.geometry(gear_part).verts, n)


def solve(diff_part: str = "62821.dat", gear_part: str = "32270.dat",
          radial_range=(30.0, 70.0), axial_range=(0.0, 45.0), step: float = 1.25):
    """
    Returns (best_radial, best_axial, diagnostics).

    The driver is rotated so its axis lies along X; it is then swept in X
    (radial, away from the diff axis) and Z (axial, along the diff axis).
    """
    ring = ring_teeth(diff_part)
    tree = cKDTree(ring)

    gear = driver_teeth(gear_part)
    base = gear @ rot("y", 90).T              # axis Z -> axis X

    # Tooth phase matters: at the wrong rotational phase the gears meet
    # tip-to-tip and the solver reports a center distance that is too large.
    # A 12t gear repeats every 30 deg, so sweep one tooth pitch.
    phases = [base @ rot("x", p).T for p in np.arange(0.0, 30.0, 2.5)]

    radials = np.arange(*radial_range, step)
    axials = np.arange(*axial_range, step)

    results = []
    for dx in radials:
        for dz in axials:
            best_gap, best_pen = 1e9, 0
            for pts0 in phases:
                pts = pts0 + np.array([dx, 0.0, dz])
                d, _ = tree.query(pts, k=1)
                gap = float(d.min())
                if gap < best_gap:
                    best_gap = gap
                    best_pen = int((d < 0.6).sum())
            results.append((dx, dz, best_gap, best_pen))

    arr = np.array(results)
    # Wanted: gap close to zero (touching) with the largest tooth overlap zone,
    # i.e. many points near the surface but not a huge count buried inside.
    touching = arr[(arr[:, 2] < 1.2)]
    if len(touching) == 0:
        return None, None, {"error": "no contact position found in the search range",
                            "min_gap": float(arr[:, 2].min())}

    # among touching candidates, prefer maximum engagement depth
    best = touching[np.argmax(touching[:, 3])]
    return float(best[0]), float(best[1]), {
        "min_gap_LDU": float(best[2]),
        "contact_points": int(best[3]),
        "candidates_touching": len(touching),
        "radial_stud": float(best[0]) / 20.0,
        "axial_stud": float(best[1]) / 20.0,
    }


if __name__ == "__main__":
    r, a, info = solve()
    print("Bevel engagement 12t -> 28t differential ring gear\n")
    if r is None:
        print("  FAILED:", info)
    else:
        print(f"  radial (X): {r:7.2f} LDU  = {r/20:.3f} stud")
        print(f"  axial  (Z): {a:7.2f} LDU  = {a/20:.3f} stud")
        print()
        for k, v in info.items():
            print(f"  {k:22s} {v}")
