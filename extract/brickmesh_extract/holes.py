# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Gaten zoeken door er een as doorheen te steken.

De rasternotatie in de shadow library is niet altijd eenduidig te lezen, en
niet elk onderdeel heeft shadow-data. Dit doet het empirisch: schuif een as
door het onderdeel en kijk waar hij past zonder de mesh te raken. Waar hij
past, zit een gat. Dat is meteen de exacte maat die je nodig hebt om er
iets op uit te lijnen.
"""
from __future__ import annotations

import numpy as np
import trimesh

from . import ldraw
from .core import rot

# Een echte LEGO-as vult het gat exact, dus die raakt de wand altijd. De sonde
# moet dunner zijn. En om een gat van open lucht te onderscheiden: een DIKKE
# sonde moet er juist NIET door passen. Zit er materiaal omheen, dan is het een
# gat; past de dikke sonde ook, dan sta je gewoon naast het onderdeel.
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
    Alle posities waar een as door `part` past, langs de gevraagde as.
    Geeft de coordinaten in het vlak loodrecht daarop, in LDU.
    """
    host = ldraw.geometry(part).mesh().copy()
    if orient is not None:
        host.apply_transform(_T(np.asarray(orient), [0, 0, 0]))
    mgr = trimesh.collision.CollisionManager()
    mgr.add_object("host", host)

    thin, thick = _cylinder(R_THIN), _cylinder(R_THICK)
    # trimesh-cilinders staan van nature langs Z
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
                continue                      # dunne sonde past niet: massief
            mk = thick.copy(); mk.apply_transform(_T(probe_ori, pos))
            if not mgr.in_collision_single(mk):
                continue                      # dikke sonde past ook: open lucht
            hits.append((float(a), float(b)))
    return hits


def cluster(hits, tol: float = 6.0) -> list[tuple[float, float]]:
    """Losse trefposities samenvoegen tot gatcentra."""
    remaining = list(hits)
    centres = []
    while remaining:
        seed = remaining.pop()
        group = [seed]
        changed = True
        while changed:
            changed = False
            for p in list(remaining):
                if any(abs(p[0] - q[0]) <= tol and abs(p[1] - q[1]) <= tol for q in group):
                    group.append(p); remaining.remove(p); changed = True
        centres.append((float(np.mean([g[0] for g in group])),
                        float(np.mean([g[1] for g in group]))))
    return sorted(centres)


if __name__ == "__main__":
    part = "64179.dat"
    print(f"Gaten in {part} gevonden door er een as doorheen te steken\n")
    for axis in ("x", "z"):
        hits = find_holes(part, axis=axis, step=2.0)
        cs = cluster(hits)
        other = [c for c in "xyz" if c != axis]
        print(f"  as langs {axis.upper()}: {len(cs)} gaten "
              f"(coordinaten in {other[0].upper()},{other[1].upper()})")
        for c in cs:
            print(f"      {other[0].upper()}={c[0]:+7.1f}  {other[1].upper()}={c[1]:+7.1f}"
                  f"   = {c[0]/20:+.2f}, {c[1]/20:+.2f} stud")
        print()
