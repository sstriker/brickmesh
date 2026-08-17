# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Finding holes by pushing an axle through them.

The grid notation in the shadow library cannot always be read unambiguously,
and not every part has shadow data. This does it empirically: slide an axle
through the part and see where it fits without touching the mesh. Where it
fits, there is a hole. That is also exactly the measurement you need to align
something to it.
"""
from __future__ import annotations

import numpy as np
import trimesh

from . import ldraw
from .core import rot

# A real LEGO axle fills the hole exactly, so it always touches the wall. The
# probe has to be thinner. And to tell a hole from open air: a THICK probe must
# NOT fit through. If there is material around it, it is a hole; if the thick
# probe fits as well, you are simply beside the part.
R_THIN = 4.5
R_THICK = 9.0


def _T(o, p):
    M = np.eye(4); M[:3, :3] = o; M[:3, 3] = p
    return M


def _cylinder(radius: float, length: float = 400.0) -> trimesh.Trimesh:
    return trimesh.creation.cylinder(radius=radius, height=length, sections=16)


def find_holes(part: str, axis: str = "x", step: float = 2.0,
               orient=None) -> list[tuple[float, float]]:
    """
    All positions where an axle fits through `part`, along the requested axis.
    Returns the coordinates in the plane perpendicular to it, in LDU.
    """
    host = ldraw.geometry(part).mesh().copy()
    if orient is not None:
        host.apply_transform(_T(np.asarray(orient), [0, 0, 0]))
    mgr = trimesh.collision.CollisionManager()
    mgr.add_object("host", host)

    thin, thick = _cylinder(R_THIN), _cylinder(R_THICK)
    # trimesh cylinders naturally lie along Z
    probe_ori = {"x": rot("y", 90), "y": rot("x", 90), "z": np.eye(3)}[axis]
    ai = "xyz".index(axis)
    other = [i for i in range(3) if i != ai]

    lo, hi = host.bounds
    rng = [np.arange(lo[i] - 4, hi[i] + 4 + step, step) for i in other]

    hits = []
    for a in rng[0]:
        for b in rng[1]:
            pos = np.zeros(3)
            pos[other[0]] = a
            pos[other[1]] = b
            mt = thin.copy(); mt.apply_transform(_T(probe_ori, pos))
            if mgr.in_collision_single(mt):
                continue                      # thin probe does not fit: solid
            mk = thick.copy(); mk.apply_transform(_T(probe_ori, pos))
            if not mgr.in_collision_single(mk):
                continue                      # thick probe fits too: open air
            hits.append((float(a), float(b)))
    return hits


def cluster(hits, tol: float = 6.0) -> list[tuple[float, float]]:
    """Merge individual hit positions into hole centers."""
    remaining = list(hits)
    centers = []
    while remaining:
        seed = remaining.pop()
        group = [seed]
        changed = True
        while changed:
            changed = False
            for p in list(remaining):
                if any(abs(p[0] - q[0]) <= tol and abs(p[1] - q[1]) <= tol for q in group):
                    group.append(p); remaining.remove(p); changed = True
        centers.append((float(np.mean([g[0] for g in group])),
                        float(np.mean([g[1] for g in group]))))
    return sorted(centers)


if __name__ == "__main__":
    part = "64179.dat"
    print(f"Holes in {part} found by pushing an axle through\n")
    for axis in ("x", "z"):
        hits = find_holes(part, axis=axis, step=2.0)
        cs = cluster(hits)
        other = [c for c in "xyz" if c != axis]
        print(f"  axle along {axis.upper()}: {len(cs)} holes "
              f"(coordinates in {other[0].upper()},{other[1].upper()})")
        for c in cs:
            print(f"      {other[0].upper()}={c[0]:+7.1f}  {other[1].upper()}={c[1]:+7.1f}"
                  f"   = {c[0]/20:+.2f}, {c[1]/20:+.2f} stud")
        print()
